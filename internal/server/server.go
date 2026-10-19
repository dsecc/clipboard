// Package server exposes the store over REST and live events over SSE.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"clipboard/internal/model"
	"clipboard/internal/store"
)

type Server struct {
	st   *store.Store
	mux  *http.ServeMux
	mu   sync.Mutex
	subs map[chan event]struct{}
}

type event struct{ name, data string }

func New(st *store.Store, web fs.FS) *Server {
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
	s := &Server{st: st, mux: http.NewServeMux(), subs: map[chan event]struct{}{}}
	s.mux.HandleFunc("GET /api/entries", s.list)
	s.mux.HandleFunc("POST /api/entries", s.postText)
	s.mux.HandleFunc("POST /api/images", s.postImage)
	s.mux.HandleFunc("DELETE /api/entries/{id}", s.del)
	s.mux.HandleFunc("GET /api/images/{id}", s.image)
	s.mux.HandleFunc("GET /api/events", s.events)
	s.mux.Handle("/", http.FileServerFS(web))
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead && crossSite(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "pedido de otro sitio rechazado"})
		return
	}
	s.mux.ServeHTTP(w, r)
}

// crossSite reports whether a write request was started by a page from another origin.
func crossSite(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return true
	}
	if o := r.Header.Get("Origin"); o != "" {
		u, err := url.Parse(o)
		return err != nil || u.Host != r.Host
	}
	return false
}

// SweepNow expires old entries and tells connected clients.
func (s *Server) SweepNow() {
	for _, id := range s.st.Sweep() {
		s.publish("deleted", map[string]string{"id": id})
	}
}

// RunSweeper calls SweepNow every interval until ctx ends.
func (s *Server) RunSweeper(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.SweepNow()
		}
	}
}

func (s *Server) publish(name string, v any) {
	b, _ := json.Marshal(v)
	ev := event{name, string(b)}
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.subs {
		select {
		case ch <- ev:
		default: // slow client: drop; it resyncs on reconnect
		}
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	switch {
	case errors.Is(err, model.ErrEmpty):
		code = http.StatusBadRequest
	case errors.Is(err, model.ErrTooLarge):
		code = http.StatusRequestEntityTooLarge
	case errors.Is(err, model.ErrBadMime):
		code = http.StatusUnsupportedMediaType
	case errors.Is(err, model.ErrNotFound):
		code = http.StatusNotFound
	}
	msg := err.Error()
	if code == http.StatusInternalServerError {
		msg = "error interno del servidor"
	}
	writeJSON(w, code, map[string]string{"error": msg})
}

func device(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "desconocido"
	}
	if r := []rune(s); len(r) > 40 {
		s = string(r[:40])
	}
	return s
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.st.List())
}

func (s *Server) postText(w http.ResponseWriter, r *http.Request) {
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "se esperaba application/json"})
		return
	}
	var in struct{ Text, Device string }
	body := http.MaxBytesReader(w, r.Body, 8*model.MaxText) // JSON escaping can inflate the text
	if err := json.NewDecoder(body).Decode(&in); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeErr(w, model.ErrTooLarge)
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "pedido inválido"})
		return
	}
	e, err := s.st.AddText(device(in.Device), in.Text)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.publish("added", e)
	writeJSON(w, http.StatusCreated, e)
}

func (s *Server) postImage(w http.ResponseWriter, r *http.Request) {
	mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	body := http.MaxBytesReader(w, r.Body, model.MaxImage+1)
	e, err := s.st.AddImage(device(r.Header.Get("X-Device")), mt, body)
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			err = model.ErrTooLarge
		}
		writeErr(w, err)
		return
	}
	s.publish("added", e)
	writeJSON(w, http.StatusCreated, e)
}

func (s *Server) del(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.st.Delete(id); err != nil {
		writeErr(w, err)
		return
	}
	s.publish("deleted", map[string]string{"id": id})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) image(w http.ResponseWriter, r *http.Request) {
	rc, mt, err := s.st.Image(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", mt)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, rc)
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming no soportado", http.StatusInternalServerError)
		return
	}
	ch := make(chan event, 32)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	fmt.Fprint(w, ": conectado\n\n")
	fl.Flush()
	beat := time.NewTicker(20 * time.Second)
	defer beat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-ch:
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.name, ev.data)
			fl.Flush()
		case <-beat.C:
			fmt.Fprint(w, ": latido\n\n")
			fl.Flush()
		}
	}
}
