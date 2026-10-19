// Package store persists clipboard entries on disk and expires them after the TTL.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"clipboard/internal/model"
)

var allowedMime = map[string]bool{
	"image/png": true, "image/jpeg": true, "image/webp": true, "image/gif": true,
}

type Store struct {
	mu      sync.Mutex
	dir     string
	now     func() time.Time
	ttl     time.Duration
	entries []model.Entry // oldest first
}

// Open loads (or creates) a store in dir. ttl defaults to model.TTL.
func Open(dir string, clock func() time.Time) (*Store, error) {
	return OpenTTL(dir, clock, model.TTL)
}

func OpenTTL(dir string, clock func() time.Time, ttl time.Duration) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "images"), 0o755); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, now: clock, ttl: ttl}
	b, err := os.ReadFile(s.indexPath())
	if err == nil {
		if err := json.Unmarshal(b, &s.entries); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return s, nil
}

func (s *Store) indexPath() string { return filepath.Join(s.dir, "entries.json") }

func (s *Store) imagePath(id string) string { return filepath.Join(s.dir, "images", id) }

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Store) saveLocked() error {
	b, err := json.Marshal(s.entries)
	if err != nil {
		return err
	}
	tmp := s.indexPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.indexPath())
}

func (s *Store) expired(e model.Entry) bool { return s.now().Sub(e.CreatedAt) > s.ttl }

func (s *Store) AddText(device, text string) (model.Entry, error) {
	if strings.TrimSpace(text) == "" {
		return model.Entry{}, model.ErrEmpty
	}
	if len(text) > model.MaxText {
		return model.Entry{}, model.ErrTooLarge
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e := model.Entry{ID: newID(), Kind: "text", Text: text, Size: len(text), Device: device, CreatedAt: s.now()}
	s.entries = append(s.entries, e)
	return e, s.saveLocked()
}

func (s *Store) AddImage(device, mime string, r io.Reader) (model.Entry, error) {
	if !allowedMime[mime] {
		return model.Entry{}, model.ErrBadMime
	}
	data, err := io.ReadAll(io.LimitReader(r, model.MaxImage+1))
	if err != nil {
		return model.Entry{}, err
	}
	if len(data) > model.MaxImage {
		return model.Entry{}, model.ErrTooLarge
	}
	if len(data) == 0 {
		return model.Entry{}, model.ErrEmpty
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e := model.Entry{ID: newID(), Kind: "image", Mime: mime, Size: len(data), Device: device, CreatedAt: s.now()}
	if err := os.WriteFile(s.imagePath(e.ID), data, 0o644); err != nil {
		return model.Entry{}, err
	}
	s.entries = append(s.entries, e)
	return e, s.saveLocked()
}

// List returns non-expired entries, oldest first.
func (s *Store) List() []model.Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []model.Entry{}
	for _, e := range s.entries {
		if !s.expired(e) {
			out = append(out, e)
		}
	}
	return out
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, e := range s.entries {
		if e.ID == id {
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
			if e.Kind == "image" {
				_ = os.Remove(s.imagePath(id))
			}
			return s.saveLocked()
		}
	}
	return model.ErrNotFound
}

// Image opens an image entry's bytes and returns its mime type.
func (s *Store) Image(id string) (io.ReadCloser, string, error) {
	s.mu.Lock()
	var found *model.Entry
	for i := range s.entries {
		if s.entries[i].ID == id && s.entries[i].Kind == "image" && !s.expired(s.entries[i]) {
			found = &s.entries[i]
			break
		}
	}
	s.mu.Unlock()
	if found == nil {
		return nil, "", model.ErrNotFound
	}
	f, err := os.Open(s.imagePath(id))
	if err != nil {
		return nil, "", model.ErrNotFound
	}
	return f, found.Mime, nil
}

// Sweep removes expired entries and returns their IDs.
func (s *Store) Sweep() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ids []string
	kept := s.entries[:0:0]
	for _, e := range s.entries {
		if s.expired(e) {
			ids = append(ids, e.ID)
			if e.Kind == "image" {
				_ = os.Remove(s.imagePath(e.ID))
			}
		} else {
			kept = append(kept, e)
		}
	}
	if len(ids) > 0 {
		s.entries = kept
		_ = s.saveLocked()
	}
	return ids
}
