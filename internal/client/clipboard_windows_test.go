//go:build windows

package client

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os/exec"
	"strings"
	"testing"
)

func psOut(t *testing.T, script string) string {
	t.Helper()
	cmd := exec.Command("powershell", "-NoProfile", "-Command", "[Console]::OutputEncoding=[Text.Encoding]::UTF8; "+script)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("powershell: %v", err)
	}
	return strings.TrimRight(string(out), "\r\n")
}

// The test overwrites the clipboard, so it puts the previous text back afterwards.
func TestSetClipboardTextRoundTrip(t *testing.T) {
	prev := psOut(t, "Get-Clipboard -Raw")
	t.Cleanup(func() { _ = SetClipboardText(prev) })

	want := "línea 1 ñ 😀\nlínea 2 <b>x</b>"
	if err := SetClipboardText(want); err != nil {
		t.Fatal(err)
	}
	got := strings.ReplaceAll(psOut(t, "Get-Clipboard -Raw"), "\r\n", "\n")
	if got != want {
		t.Fatalf("clipboard has %q, want %q", got, want)
	}
}

func TestSetClipboardImageRoundTrip(t *testing.T) {
	prev := psOut(t, "Get-Clipboard -Raw")
	t.Cleanup(func() { _ = SetClipboardText(prev) })

	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	if err := SetClipboardImage("image/png", buf.Bytes()); err != nil {
		t.Fatal(err)
	}
	got := psOut(t, "Add-Type -AssemblyName System.Windows.Forms; $i=[Windows.Forms.Clipboard]::GetImage(); if ($i) { \"$($i.Width)x$($i.Height)\" } else { 'none' }")
	if got != "3x2" {
		t.Fatalf("clipboard image: %q, want 3x2", got)
	}
	if err := SetClipboardImage("image/webp", []byte("no es una imagen")); err == nil {
		t.Fatal("garbage must be rejected")
	}
}
