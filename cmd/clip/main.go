// Command clip is the terminal client for Clipboard.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"

	"clipboard/internal/client"
	"clipboard/internal/model"
)

const help = `Comandos:
  texto + Enter   manda el texto a todos tus equipos
  clic            un clic del mouse sobre un mensaje lo copia entero
  c               copia la última entrada a este equipo
  c N             copia la entrada número N
  d N             borra la entrada número N
  m               activa/desactiva el clic para copiar (con el clic activo,
                  seleccionar con el mouse necesita mantener Shift apretado)
  q               sale (también Ctrl+D)
  \c              manda la letra "c" (la barra invertida evita el comando)
Un pegado de varias líneas se manda como una sola entrada.`

// view receives live changes and prints them above the prompt.
type view struct {
	mu        sync.Mutex
	term      *client.Term
	num       *client.Numbering
	entries   map[string]model.Entry
	connected bool
	everUp    bool
}

func (v *view) Added(e model.Entry) {
	v.mu.Lock()
	n := v.num.Assign(e.ID)
	v.entries[e.ID] = e
	v.mu.Unlock()
	v.term.PrintEntry(e.ID, client.FormatEntry(n, e, time.Now()))
}

// copyByID copies an entry to this computer's clipboard and says so above the prompt.
func (v *view) copyByID(ctx context.Context, c *client.Client, id string) {
	v.mu.Lock()
	e, ok := v.entries[id]
	v.mu.Unlock()
	if !ok {
		v.term.Print("✗ esa entrada ya no existe (se borró o venció)")
		return
	}
	if err := copyEntry(ctx, c, e); err != nil {
		v.term.Print("✗ " + err.Error())
		return
	}
	v.term.Print("✓ copiado: " + shorten(e))
}

func (v *view) Deleted(id string) {
	v.mu.Lock()
	v.num.Remove(id)
	e, ok := v.entries[id]
	delete(v.entries, id)
	v.mu.Unlock()
	if ok {
		v.term.Print("· una entrada se borró (" + shorten(e) + ")")
	}
}

func (v *view) Status(up bool) {
	v.mu.Lock()
	changed, again := up != v.connected, v.everUp
	v.connected = up
	if up {
		v.everUp = true
	}
	v.mu.Unlock()
	switch {
	case changed && !up && again:
		v.term.Print("· sin conexión, reintentando…")
	case changed && up && again:
		v.term.Print("· reconectado")
	}
}

func shorten(e model.Entry) string {
	if e.Kind == "image" {
		return "imagen"
	}
	s := strings.Join(strings.Fields(e.Text), " ")
	if r := []rune(s); len(r) > 30 {
		s = string(r[:30]) + "…"
	}
	return s
}

func main() {
	serverFlag := flag.String("server", "", "dirección del servidor (por ejemplo 100.64.0.1:8787 o nombre.tailnet.ts.net)")
	nameFlag := flag.String("name", "", "nombre de este equipo (por defecto, el nombre de la máquina)")
	flag.Parse()

	cfgPath, err := client.DefaultConfigPath()
	if err != nil {
		fatal(err)
	}
	cfg, err := client.LoadConfig(cfgPath)
	if err != nil {
		fatal(fmt.Errorf("no pude leer %s: %w", cfgPath, err))
	}
	server, ok := client.Resolve(*serverFlag, os.Getenv("CLIP_SERVER"), cfg)
	if !ok {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			fatal(errors.New("falta la dirección del servidor: usá --server o la variable CLIP_SERVER"))
		}
		fmt.Print("Dirección del servidor (por ejemplo 100.64.0.1:8787): ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if strings.TrimSpace(line) == "" {
			fatal(errors.New("sin dirección no puedo conectar"))
		}
		server = client.NormalizeServer(line)
		cfg.Server = server
		if err := client.SaveConfig(cfgPath, cfg); err != nil {
			fmt.Fprintln(os.Stderr, "aviso: no pude guardar la configuración:", err)
		}
	}
	device := firstNonEmpty(*nameFlag, cfg.Device, hostname())
	c := &client.Client{Base: server, Device: device, HTTP: &http.Client{}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// piped input: `echo texto | clip` sends it and exits
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fatal(err)
		}
		bom := string(rune(0xFEFF)) // PowerShell pipes start with a BOM
		text := strings.TrimRight(strings.TrimPrefix(string(data), bom), "\r\n")
		if _, err := c.SendText(ctx, text); err != nil {
			fatal(err)
		}
		fmt.Println("enviado")
		return
	}

	t, err := client.NewTerm()
	if err != nil {
		fatal(err)
	}
	defer t.Close()
	v := &view{term: t, num: client.NewNumbering(), entries: map[string]model.Entry{}}
	t.Print(fmt.Sprintf("Clipboard · %s · como %q · escribí ? para ayuda", server, device))
	t.OnClick = func(id string) { v.copyByID(ctx, c, id) }
	go c.Watch(ctx, v)

	for {
		line, pasted, err := t.ReadLine()
		if err != nil {
			return // Ctrl+D or Ctrl+C
		}
		cmd := client.ParseInput(line, pasted)
		switch cmd.Kind {
		case client.Send:
			if _, err := c.SendText(ctx, cmd.Text); err != nil {
				t.Print("✗ " + err.Error())
			}
		case client.Copy, client.Delete:
			v.mu.Lock()
			var id string
			var found bool
			if cmd.Kind == client.Copy && cmd.N == 0 {
				id, found = v.num.Last()
			} else {
				id, found = v.num.ID(cmd.N)
			}
			e := v.entries[id]
			v.mu.Unlock()
			if !found {
				t.Print("✗ esa entrada no existe (se borró o venció)")
				continue
			}
			if cmd.Kind == client.Delete {
				if err := c.Delete(ctx, id); err != nil {
					t.Print("✗ " + err.Error())
				}
				continue
			}
			if err := copyEntry(ctx, c, e); err != nil {
				t.Print("✗ " + err.Error())
			} else {
				t.Print("✓ copiado: " + shorten(e))
			}
		case client.Help:
			t.Print(help)
		case client.Mouse:
			if t.ToggleMouse() {
				t.Print("· clic para copiar: activado (Shift + arrastrar para seleccionar texto)")
			} else {
				t.Print("· clic para copiar: desactivado (seleccionás con el mouse como siempre)")
			}
		case client.Invalid:
			t.Print("✗ " + cmd.Text)
		case client.Quit:
			return
		}
	}
}

func copyEntry(ctx context.Context, c *client.Client, e model.Entry) error {
	if e.Kind == "image" {
		data, mime, err := c.Image(ctx, e.ID)
		if err != nil {
			return err
		}
		return client.SetClipboardImage(mime, data)
	}
	return client.SetClipboardText(e.Text)
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "equipo"
	}
	return h
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "clip:", err)
	os.Exit(1)
}
