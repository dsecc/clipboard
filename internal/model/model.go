// Package model holds the types and limits shared by server and client.
package model

import (
	"errors"
	"time"
)

const (
	MaxText  = 64 << 10
	MaxImage = 5 << 20
	TTL      = 24 * time.Hour
)

var (
	ErrEmpty    = errors.New("el mensaje está vacío")
	ErrTooLarge = errors.New("el contenido es demasiado grande")
	ErrBadMime  = errors.New("tipo de imagen no permitido")
	ErrNotFound = errors.New("la entrada no existe")
)

// Entry is one item in the shared clipboard.
type Entry struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"` // "text" or "image"
	Text      string    `json:"text,omitempty"`
	Mime      string    `json:"mime,omitempty"`
	Size      int       `json:"size"`
	Device    string    `json:"device"`
	CreatedAt time.Time `json:"createdAt"`
}
