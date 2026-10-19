//go:build !windows

package client

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type clipTool struct {
	name      string
	text      []string
	imageArgs func(mime string) []string
}

// pickTool finds an installed clipboard helper (Wayland first if the session is Wayland).
func pickTool() *clipTool {
	tools := []clipTool{
		{"xclip", []string{"-selection", "clipboard"}, func(m string) []string { return []string{"-selection", "clipboard", "-t", m} }},
		{"xsel", []string{"--clipboard", "--input"}, nil},
	}
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		tools = append([]clipTool{{"wl-copy", nil, func(m string) []string { return []string{"--type", m} }}}, tools...)
	}
	for i := range tools {
		if _, err := exec.LookPath(tools[i].name); err == nil {
			return &tools[i]
		}
	}
	return nil
}

func run(name string, args []string, input string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(input)
	return cmd.Run() // stdout/stderr stay nil so xclip's background process doesn't block us
}

// SetClipboardText puts text on the clipboard; without a helper program it asks the terminal (OSC 52).
func SetClipboardText(text string) error {
	if t := pickTool(); t != nil {
		return run(t.name, t.text, text)
	}
	fmt.Fprintf(os.Stdout, "\x1b]52;c;%s\x07", base64.StdEncoding.EncodeToString([]byte(text)))
	return nil
}

func SetClipboardImage(mime string, data []byte) error {
	t := pickTool()
	if t == nil || t.imageArgs == nil {
		return errors.New("para copiar imágenes instalá xclip (sudo apt install xclip) o wl-clipboard")
	}
	return run(t.name, t.imageArgs(mime), string(data))
}
