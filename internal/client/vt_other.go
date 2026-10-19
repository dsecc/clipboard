//go:build !windows

package client

func enableVT() func() { return func() {} }
