//go:build windows

package client

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32          = windows.NewLazySystemDLL("user32.dll")
	pOpenClipboard  = user32.NewProc("OpenClipboard")
	pCloseClipboard = user32.NewProc("CloseClipboard")
	pEmptyClipboard = user32.NewProc("EmptyClipboard")
	pSetClipboard   = user32.NewProc("SetClipboardData")
	pRegisterFormat = user32.NewProc("RegisterClipboardFormatW")

	kernel32      = windows.NewLazySystemDLL("kernel32.dll")
	pGlobalAlloc  = kernel32.NewProc("GlobalAlloc")
	pGlobalLock   = kernel32.NewProc("GlobalLock")
	pGlobalUnlock = kernel32.NewProc("GlobalUnlock")
	pGlobalFree   = kernel32.NewProc("GlobalFree")
)

const (
	cfDIB       = 8
	cfUnicode   = 13
	gmemMovable = 0x0002
)

func withClipboard(fn func() error) error {
	// Windows ties the open clipboard to the calling thread, so keep this goroutine on it
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var opened bool
	for i := 0; i < 10; i++ { // another program may hold it for a moment
		if r, _, _ := pOpenClipboard.Call(0); r != 0 {
			opened = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !opened {
		return errors.New("no pude abrir el portapapeles de Windows")
	}
	defer pCloseClipboard.Call()
	pEmptyClipboard.Call()
	return fn()
}

func setData(format uintptr, data []byte) error {
	h, _, _ := pGlobalAlloc.Call(gmemMovable, uintptr(len(data)))
	if h == 0 {
		return errors.New("sin memoria para el portapapeles")
	}
	addr, _, _ := pGlobalLock.Call(h)
	if addr == 0 {
		pGlobalFree.Call(h)
		return errors.New("no pude reservar memoria para el portapapeles")
	}
	// addr is a pointer into memory Windows owns; this form keeps go vet happy
	p := *(*unsafe.Pointer)(unsafe.Pointer(&addr))
	copy(unsafe.Slice((*byte)(p), len(data)), data)
	pGlobalUnlock.Call(h)
	if r, _, _ := pSetClipboard.Call(format, h); r == 0 {
		pGlobalFree.Call(h)
		return errors.New("Windows rechazó el contenido del portapapeles")
	}
	return nil // the system owns the memory now
}

// SetClipboardText puts text on the Windows clipboard (UTF-16, CRLF line ends).
func SetClipboardText(text string) error {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\n", "\r\n")
	u := utf16.Encode([]rune(text))
	u = append(u, 0)
	b := make([]byte, len(u)*2)
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[i*2:], c)
	}
	return withClipboard(func() error { return setData(cfUnicode, b) })
}

// SetClipboardImage puts an image on the clipboard both as PNG and as a classic bitmap.
func SetClipboardImage(mime string, data []byte) error {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return errors.New("no puedo copiar este formato de imagen desde la terminal; abrila en la web")
	}
	b := img.Bounds()
	rgba := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)

	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, rgba); err != nil {
		return err
	}
	w, h := rgba.Bounds().Dx(), rgba.Bounds().Dy()
	dib := make([]byte, 40+w*h*4)
	binary.LittleEndian.PutUint32(dib[0:], 40)
	binary.LittleEndian.PutUint32(dib[4:], uint32(w))
	binary.LittleEndian.PutUint32(dib[8:], uint32(h)) // positive height: rows bottom-up
	binary.LittleEndian.PutUint16(dib[12:], 1)
	binary.LittleEndian.PutUint16(dib[14:], 32)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := rgba.NRGBAAt(x, y)
			o := 40 + ((h-1-y)*w+x)*4
			dib[o], dib[o+1], dib[o+2], dib[o+3] = c.B, c.G, c.R, c.A
		}
	}
	name, _ := syscall.UTF16PtrFromString("PNG")
	pngFmt, _, _ := pRegisterFormat.Call(uintptr(unsafe.Pointer(name)))
	return withClipboard(func() error {
		if err := setData(cfDIB, dib); err != nil {
			return err
		}
		if pngFmt != 0 {
			_ = setData(pngFmt, pngBuf.Bytes())
		}
		return nil
	})
}
