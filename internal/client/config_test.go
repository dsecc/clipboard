package client

import (
	"path/filepath"
	"testing"
)

func TestConfigPrecedence(t *testing.T) {
	file := Config{Server: "http://file:1"}
	if s, _ := Resolve("http://flag:1", "http://env:1", file); s != "http://flag:1" {
		t.Errorf("flag should win, got %s", s)
	}
	if s, _ := Resolve("", "http://env:1", file); s != "http://env:1" {
		t.Errorf("env should beat file, got %s", s)
	}
	if s, ok := Resolve("", "", file); !ok || s != "http://file:1" {
		t.Errorf("file fallback, got %s %v", s, ok)
	}
	if _, ok := Resolve("", "", Config{}); ok {
		t.Error("nothing configured must report ok=false")
	}
}

func TestNormalizeServer(t *testing.T) {
	cases := map[string]string{
		"100.64.0.1:8787":         "http://100.64.0.1:8787",
		"http://x:1/":             "http://x:1",
		"https://srv.ts.net":      "https://srv.ts.net",
		"  srv.tail1234.ts.net  ": "https://srv.tail1234.ts.net",
		"srv.tail1234.ts.net/":    "https://srv.tail1234.ts.net",
	}
	for in, want := range cases {
		if got := NormalizeServer(in); got != want {
			t.Errorf("NormalizeServer(%q) = %q, want %q", in, got, want)
		}
	}
}

// %APPDATA% can be redirected for packaged apps, so the file lives under the home folder everywhere.
func TestDefaultConfigPathIsUnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	got, err := DefaultConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".config", "clip", "config.json"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestConfigSaveLoadRoundTrip(t *testing.T) {
	path := t.TempDir() + "/clip/config.json"
	if err := SaveConfig(path, Config{Server: "http://a:1", Device: "pc"}); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig(path)
	if err != nil || got.Server != "http://a:1" || got.Device != "pc" {
		t.Fatalf("%+v %v", got, err)
	}
	if got, err := LoadConfig(t.TempDir() + "/missing.json"); err != nil || got.Server != "" {
		t.Fatalf("missing file must be empty config, not error: %+v %v", got, err)
	}
}
