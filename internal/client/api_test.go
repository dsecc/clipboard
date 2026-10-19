package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"clipboard/internal/model"
	"clipboard/internal/server"
	"clipboard/internal/store"
	"clipboard/web"
)

func newServer(t *testing.T) (*httptest.Server, *Client) {
	t.Helper()
	st, err := store.Open(t.TempDir(), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(server.New(st, web.FS))
	t.Cleanup(ts.Close)
	return ts, &Client{Base: ts.URL, Device: "test-pc", HTTP: &http.Client{}}
}

type recorder struct {
	mu      sync.Mutex
	added   []string
	deleted []string
}

func (r *recorder) Added(e model.Entry) { r.mu.Lock(); r.added = append(r.added, e.ID); r.mu.Unlock() }
func (r *recorder) Deleted(id string)   { r.mu.Lock(); r.deleted = append(r.deleted, id); r.mu.Unlock() }
func (r *recorder) Status(bool)         {}
func (r *recorder) snapshot() ([]string, []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.added...), append([]string(nil), r.deleted...)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func TestClientReportsServerErrors(t *testing.T) {
	_, c := newServer(t)
	_, err := c.SendText(context.Background(), strings.Repeat("a", model.MaxText+1))
	if err == nil || !strings.Contains(err.Error(), "demasiado grande") {
		t.Fatalf("want Spanish too-large message, got %v", err)
	}
	if _, err := c.SendText(context.Background(), "  "); err == nil || !strings.Contains(err.Error(), "vacío") {
		t.Fatalf("want empty message error, got %v", err)
	}
}

func TestSendListDeleteAndImage(t *testing.T) {
	_, c := newServer(t)
	ctx := context.Background()
	e, err := c.SendText(ctx, "hola")
	if err != nil || e.Device != "test-pc" {
		t.Fatalf("%+v %v", e, err)
	}
	list, _ := c.List(ctx)
	if len(list) != 1 || list[0].ID != e.ID {
		t.Fatalf("%+v", list)
	}
	if err := c.Delete(ctx, e.ID); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(ctx, e.ID); err == nil || !strings.Contains(err.Error(), "no existe") {
		t.Fatalf("second delete: %v", err)
	}
}

func TestEventStreamReconnectDedupes(t *testing.T) {
	ts, c := newServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rec := &recorder{}
	go c.Watch(ctx, rec)

	a, _ := c.SendText(ctx, "A")
	waitFor(t, "A added", func() bool { ad, _ := rec.snapshot(); return len(ad) == 1 })

	ts.CloseClientConnections() // cut the SSE stream
	if err := c.Delete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	b, _ := c.SendText(ctx, "B")
	waitFor(t, "B added and A deleted after reconnect", func() bool {
		ad, de := rec.snapshot()
		return len(ad) == 2 && len(de) == 1
	})
	time.Sleep(500 * time.Millisecond)
	ad, de := rec.snapshot()
	if len(ad) != 2 || ad[0] != a.ID || ad[1] != b.ID || len(de) != 1 || de[0] != a.ID {
		t.Fatalf("duplicates or misses: added=%v deleted=%v", ad, de)
	}
}
