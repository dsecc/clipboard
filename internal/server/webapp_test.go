package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"clipboard/internal/store"
	"clipboard/web"
)

// The page must be installable on a phone: manifest, icons and favicon are served with the right types.
func TestInstallableWebAppFiles(t *testing.T) {
	st, err := store.Open(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(New(st, web.FS))
	defer ts.Close()

	get := func(path string) (*http.Response, []byte) {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, b
	}

	resp, body := get("/manifest.webmanifest")
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Type"), "json") {
		t.Fatalf("manifest: %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	var m struct {
		Name, ShortName, StartURL, Display, BackgroundColor, ThemeColor string
		Icons                                                           []struct{ Src, Sizes, Type string }
	}
	raw := map[string]any{}
	_ = json.Unmarshal(body, &raw)
	b, _ := json.Marshal(raw)
	// keys are snake_case in the file; decode them by hand
	m.Name, _ = raw["name"].(string)
	m.ShortName, _ = raw["short_name"].(string)
	m.StartURL, _ = raw["start_url"].(string)
	m.Display, _ = raw["display"].(string)
	if m.Name == "" || m.ShortName == "" || m.StartURL != "/" || m.Display != "standalone" {
		t.Fatalf("manifest incomplete: %s", b)
	}
	icons, _ := raw["icons"].([]any)
	sizes := map[string]bool{}
	for _, i := range icons {
		ic := i.(map[string]any)
		sizes[ic["sizes"].(string)] = true
		if r, _ := get("/" + strings.TrimPrefix(ic["src"].(string), "/")); r.StatusCode != 200 || r.Header.Get("Content-Type") != "image/png" {
			t.Errorf("manifest icon %v: %d %q", ic["src"], r.StatusCode, r.Header.Get("Content-Type"))
		}
	}
	if !sizes["192x192"] || !sizes["512x512"] {
		t.Errorf("manifest needs 192 and 512 icons, has %v", sizes)
	}

	for path, ctype := range map[string]string{
		"/apple-touch-icon.png": "image/png",
		"/favicon-32.png":       "image/png",
		"/icon.svg":             "image/svg+xml",
	} {
		r, _ := get(path)
		if r.StatusCode != 200 || !strings.HasPrefix(r.Header.Get("Content-Type"), ctype) {
			t.Errorf("%s: %d %q, want %s", path, r.StatusCode, r.Header.Get("Content-Type"), ctype)
		}
	}

	_, page := get("/")
	for _, want := range []string{`rel="manifest"`, `rel="apple-touch-icon"`, `rel="icon"`} {
		if !strings.Contains(string(page), want) {
			t.Errorf("index.html lacks %s", want)
		}
	}
}
