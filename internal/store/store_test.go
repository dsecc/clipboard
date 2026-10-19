package store

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clipboard/internal/model"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func newStore(t *testing.T) (*Store, *fakeClock, string) {
	t.Helper()
	dir := t.TempDir()
	clk := &fakeClock{t: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	s, err := Open(dir, clk.now)
	if err != nil {
		t.Fatal(err)
	}
	return s, clk, dir
}

func TestAddTextRejectsEmptyAndWhitespace(t *testing.T) {
	s, _, _ := newStore(t)
	for _, in := range []string{"", "   ", "\n\t "} {
		if _, err := s.AddText("pc", in); !errors.Is(err, model.ErrEmpty) {
			t.Fatalf("%q: got %v, want ErrEmpty", in, err)
		}
	}
	if len(s.List()) != 0 {
		t.Fatal("empty message created an entry")
	}
}

func TestAddTextRejectsOver64KB(t *testing.T) {
	s, _, _ := newStore(t)
	if _, err := s.AddText("pc", strings.Repeat("a", model.MaxText)); err != nil {
		t.Fatalf("exactly 64KB must pass: %v", err)
	}
	if _, err := s.AddText("pc", strings.Repeat("a", model.MaxText+1)); !errors.Is(err, model.ErrTooLarge) {
		t.Fatalf("got %v, want ErrTooLarge", err)
	}
}

func TestAddImageRejectsOver5MBAndBadMime(t *testing.T) {
	s, _, _ := newStore(t)
	for _, mime := range []string{"image/svg+xml", "text/html", ""} {
		if _, err := s.AddImage("pc", mime, bytes.NewReader([]byte("x"))); !errors.Is(err, model.ErrBadMime) {
			t.Fatalf("%q: got %v, want ErrBadMime", mime, err)
		}
	}
	big := bytes.NewReader(make([]byte, model.MaxImage+1))
	if _, err := s.AddImage("pc", "image/png", big); !errors.Is(err, model.ErrTooLarge) {
		t.Fatalf("got %v, want ErrTooLarge", err)
	}
	ok := bytes.NewReader(make([]byte, model.MaxImage))
	e, err := s.AddImage("pc", "image/png", ok)
	if err != nil || e.Size != model.MaxImage || e.Kind != "image" {
		t.Fatalf("exactly 5MB must pass: %v %+v", err, e)
	}
}

func TestListOrderOldestFirst(t *testing.T) {
	s, clk, _ := newStore(t)
	a, _ := s.AddText("pc", "uno")
	clk.t = clk.t.Add(time.Minute)
	b, _ := s.AddText("pc", "dos")
	got := s.List()
	if len(got) != 2 || got[0].ID != a.ID || got[1].ID != b.ID {
		t.Fatalf("order wrong: %+v", got)
	}
}

func TestSweepRemovesAfter24h(t *testing.T) {
	s, clk, dir := newStore(t)
	txt, _ := s.AddText("pc", "hola")
	img, _ := s.AddImage("pc", "image/png", bytes.NewReader([]byte("png")))
	clk.t = clk.t.Add(model.TTL - time.Second) // 23:59:59
	if ids := s.Sweep(); len(ids) != 0 || len(s.List()) != 2 {
		t.Fatalf("swept too early: %v", ids)
	}
	clk.t = clk.t.Add(2 * time.Second) // 24:00:01
	ids := s.Sweep()
	if len(ids) != 2 || len(s.List()) != 0 {
		t.Fatalf("not swept: %v", ids)
	}
	if _, err := os.Stat(filepath.Join(dir, "images", img.ID)); !os.IsNotExist(err) {
		t.Fatal("image file not removed")
	}
	_ = txt
}

func TestListHidesExpiredEvenBeforeSweep(t *testing.T) {
	s, clk, _ := newStore(t)
	s.AddText("pc", "hola")
	clk.t = clk.t.Add(model.TTL + time.Minute)
	if len(s.List()) != 0 {
		t.Fatal("expired entry still listed")
	}
}

func TestPersistsAcrossReopen(t *testing.T) {
	s, clk, dir := newStore(t)
	e, _ := s.AddText("pc", "persisto")
	i, _ := s.AddImage("pc", "image/png", bytes.NewReader([]byte("png")))
	s2, err := Open(dir, clk.now)
	if err != nil {
		t.Fatal(err)
	}
	got := s2.List()
	if len(got) != 2 || got[0].ID != e.ID || got[0].Text != "persisto" {
		t.Fatalf("not persisted: %+v", got)
	}
	r, mime, err := s2.Image(i.ID)
	if err != nil || mime != "image/png" {
		t.Fatalf("image lost: %v %q", err, mime)
	}
	b, _ := io.ReadAll(r)
	r.Close()
	if string(b) != "png" {
		t.Fatalf("image bytes %q", b)
	}
}

func TestDeleteUnknownIsErrNotFound(t *testing.T) {
	s, _, _ := newStore(t)
	if err := s.Delete("nope"); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
	e, _ := s.AddText("pc", "x")
	if err := s.Delete(e.ID); err != nil || len(s.List()) != 0 {
		t.Fatalf("delete failed: %v", err)
	}
}
