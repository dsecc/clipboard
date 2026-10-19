//go:build windows

package client

import "golang.org/x/sys/windows"

const (
	enableVTInput      = 0x0200 // ENABLE_VIRTUAL_TERMINAL_INPUT
	enableVTProcessing = 0x0004 // ENABLE_VIRTUAL_TERMINAL_PROCESSING
)

// enableVT makes the Windows console speak ANSI escape codes in and out; the
// returned func restores the previous modes.
func enableVT() func() {
	in, out := windows.Handle(windows.Stdin), windows.Handle(windows.Stdout)
	var inMode, outMode uint32
	_ = windows.GetConsoleMode(in, &inMode)
	_ = windows.GetConsoleMode(out, &outMode)
	_ = windows.SetConsoleMode(in, inMode|enableVTInput)
	_ = windows.SetConsoleMode(out, outMode|enableVTProcessing)
	return func() {
		_ = windows.SetConsoleMode(in, inMode)
		_ = windows.SetConsoleMode(out, outMode)
	}
}
