package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"clipboard/internal/model"
	"clipboard/internal/store"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func setup(t *testing.T) (*httptest.Server, *Server, *clock) {
	t.Helper()
	clk := &clock{t: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	st, err := store.Open(t.TempDir(), clk.now)
	if err != nil {
		t.Fatal(err)
	}
	web := fstest.MapFS{"index.html": {Data: []byte("<h1>web</h1>")}}
	srv := New(st, web)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return ts, srv, clk
}

func postText(t *testing.T, ts *httptest.Server, text string) *http.Response {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"text": text, "device": "pc"})
	resp, err := http.Post(ts.URL+"/api/entries", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

type sse struct {
	resp *http.Response
	ch   chan [2]string // event, data
}

func openSSE(t *testing.T, ts *httptest.Server) *sse {
	t.Helper()
	resp, err := http.Get(ts.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	s := &sse{resp: resp, ch: make(chan [2]string, 16)}
	go func() {
		sc := bufio.NewScanner(resp.Body)
		var ev string
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				ev = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				s.ch <- [2]string{ev, strings.TrimPrefix(line, "data: ")}
			}
		}
	}()
	t.Cleanup(func() { resp.Body.Close() })
	return s
}

func (s *sse) next(t *testing.T) [2]string {
	t.Helper()
	select {
	case e := <-s.ch:
		return e
	case <-time.After(2 * time.Second):
		t.Fatal("no SSE event received")
		return [2]string{}
	}
}

func TestPostTextThenListAndSSEAdded(t *testing.T) {
	ts, _, _ := setup(t)
	ev := openSSE(t, ts)
	time.Sleep(50 * time.Millisecond)
	resp := postText(t, ts, "hola")
	if resp.StatusCode != 201 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var e model.Entry
	json.NewDecoder(resp.Body).Decode(&e)
	got := ev.next(t)
	if got[0] != "added" || !strings.Contains(got[1], e.ID) {
		t.Fatalf("event %v", got)
	}
	lr, _ := http.Get(ts.URL + "/api/entries")
	var list []model.Entry
	json.NewDecoder(lr.Body).Decode(&list)
	if len(list) != 1 || list[0].Text != "hola" || list[0].Device != "pc" {
		t.Fatalf("list %+v", list)
	}
}

func TestSSEDeliversDeletedOnDelete(t *testing.T) {
	ts, _, _ := setup(t)
	var e model.Entry
	json.NewDecoder(postText(t, ts, "x").Body).Decode(&e)
	ev := openSSE(t, ts)
	time.Sleep(50 * time.Millisecond)
	req, _ := http.NewRequest("DELETE", ts.URL+"/api/entries/"+e.ID, nil)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != 204 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	got := ev.next(t)
	if got[0] != "deleted" || !strings.Contains(got[1], e.ID) {
		t.Fatalf("event %v", got)
	}
	req, _ = http.NewRequest("DELETE", ts.URL+"/api/entries/"+e.ID, nil)
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != 404 {
		t.Fatalf("second delete status %d, want 404", resp.StatusCode)
	}
}

func TestStatusCodes400413415(t *testing.T) {
	ts, _, _ := setup(t)
	check := func(resp *http.Response, want int) {
		t.Helper()
		defer resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("status %d, want %d", resp.StatusCode, want)
		}
		var body map[string]string
		json.NewDecoder(resp.Body).Decode(&body)
		if body["error"] == "" {
			t.Fatal("missing error message")
		}
	}
	check(postText(t, ts, "   "), 400)
	check(postText(t, ts, strings.Repeat("a", model.MaxText+1)), 413)

	img := func(mime string, n int) *http.Response {
		req, _ := http.NewRequest("POST", ts.URL+"/api/images", bytes.NewReader(make([]byte, n)))
		req.Header.Set("Content-Type", mime)
		req.Header.Set("X-Device", "iphone")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	check(img("image/png", model.MaxImage+1), 413)
	check(img("image/svg+xml", 10), 415)
	check(img("text/html", 10), 415)
}

func TestImageServedWithNosniff(t *testing.T) {
	ts, _, _ := setup(t)
	req, _ := http.NewRequest("POST", ts.URL+"/api/images", strings.NewReader("PNGDATA"))
	req.Header.Set("Content-Type", "image/png")
	req.Header.Set("X-Device", "iphone")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != 201 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var e model.Entry
	json.NewDecoder(resp.Body).Decode(&e)
	if e.Kind != "image" || e.Device != "iphone" {
		t.Fatalf("entry %+v", e)
	}
	gr, _ := http.Get(ts.URL + "/api/images/" + e.ID)
	b, _ := io.ReadAll(gr.Body)
	if string(b) != "PNGDATA" || gr.Header.Get("Content-Type") != "image/png" || gr.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("bad image response: %q %v", b, gr.Header)
	}
	nr, _ := http.Get(ts.URL + "/api/images/unknown")
	if nr.StatusCode != 404 {
		t.Fatalf("unknown image status %d", nr.StatusCode)
	}
}

func TestSweepEmitsDeletedEvent(t *testing.T) {
	ts, srv, clk := setup(t)
	var e model.Entry
	json.NewDecoder(postText(t, ts, "viejo").Body).Decode(&e)
	ev := openSSE(t, ts)
	time.Sleep(50 * time.Millisecond)
	clk.t = clk.t.Add(model.TTL + time.Minute)
	srv.SweepNow()
	got := ev.next(t)
	if got[0] != "deleted" || !strings.Contains(got[1], e.ID) {
		t.Fatalf("event %v", got)
	}
}

func TestMultipleSSEClientsAllReceive(t *testing.T) {
	ts, _, _ := setup(t)
	a, b := openSSE(t, ts), openSSE(t, ts)
	time.Sleep(50 * time.Millisecond)
	postText(t, ts, "para todos")
	if a.next(t)[0] != "added" || b.next(t)[0] != "added" {
		t.Fatal("not all clients got the event")
	}
}

func TestServesWebIndex(t *testing.T) {
	ts, _, _ := setup(t)
	resp, _ := http.Get(ts.URL + "/")
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !strings.Contains(string(b), "<h1>web</h1>") {
		t.Fatalf("web not served: %d %q", resp.StatusCode, b)
	}
}
