package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"clipboard/internal/model"
)

type Client struct {
	Base   string
	Device string
	HTTP   *http.Client
}

// Handler receives live changes. Added is called once per entry id and Deleted
// once per entry that was known, even across reconnects.
type Handler interface {
	Added(model.Entry)
	Deleted(id string)
	Status(connected bool)
}

func (c *Client) do(ctx context.Context, method, path string, body io.Reader, ctype string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.Base+path, body)
	if err != nil {
		return nil, err
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("no pude conectar con el servidor (%s)", c.Base)
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		var e struct{ Error string }
		if json.NewDecoder(resp.Body).Decode(&e) == nil && e.Error != "" {
			return nil, errors.New(e.Error)
		}
		return nil, fmt.Errorf("el servidor respondió %d", resp.StatusCode)
	}
	return resp, nil
}

func (c *Client) List(ctx context.Context) ([]model.Entry, error) {
	resp, err := c.do(ctx, "GET", "/api/entries", nil, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out []model.Entry
	return out, json.NewDecoder(resp.Body).Decode(&out)
}

func (c *Client) SendText(ctx context.Context, text string) (model.Entry, error) {
	b, _ := json.Marshal(map[string]string{"text": text, "device": c.Device})
	resp, err := c.do(ctx, "POST", "/api/entries", bytes.NewReader(b), "application/json")
	if err != nil {
		return model.Entry{}, err
	}
	defer resp.Body.Close()
	var e model.Entry
	return e, json.NewDecoder(resp.Body).Decode(&e)
}

func (c *Client) Delete(ctx context.Context, id string) error {
	resp, err := c.do(ctx, "DELETE", "/api/entries/"+id, nil, "")
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

func (c *Client) Image(ctx context.Context, id string) ([]byte, string, error) {
	resp, err := c.do(ctx, "GET", "/api/images/"+id, nil, "")
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return data, resp.Header.Get("Content-Type"), err
}

// Watch keeps a live connection until ctx ends, reconnecting with backoff and
// resyncing from the list each time, so nothing is shown twice or missed.
func (c *Client) Watch(ctx context.Context, h Handler) {
	known := map[string]bool{}
	wait := 300 * time.Millisecond
	for ctx.Err() == nil {
		_ = c.stream(ctx, h, known, func() { wait = 300 * time.Millisecond })
		h.Status(false)
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		if wait *= 2; wait > 5*time.Second {
			wait = 5 * time.Second
		}
	}
}

func (c *Client) stream(ctx context.Context, h Handler, known map[string]bool, connected func()) error {
	resp, err := c.do(ctx, "GET", "/api/events", nil, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	list, err := c.List(ctx)
	if err != nil {
		return err
	}
	connected()
	h.Status(true)
	present := map[string]bool{}
	for _, e := range list {
		present[e.ID] = true
		if !known[e.ID] {
			known[e.ID] = true
			h.Added(e)
		}
	}
	for id := range known {
		if !present[id] {
			delete(known, id)
			h.Deleted(id)
		}
	}
	r := bufio.NewReader(resp.Body)
	var ev string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return err
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(line, "event: "):
			ev = line[len("event: "):]
		case strings.HasPrefix(line, "data: "):
			data := line[len("data: "):]
			switch ev {
			case "added":
				var e model.Entry
				if json.Unmarshal([]byte(data), &e) == nil && !known[e.ID] {
					known[e.ID] = true
					h.Added(e)
				}
			case "deleted":
				var d struct{ ID string }
				if json.Unmarshal([]byte(data), &d) == nil && known[d.ID] {
					delete(known, d.ID)
					h.Deleted(d.ID)
				}
			}
		}
	}
}
