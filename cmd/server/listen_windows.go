package main

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

// OS-specific error check and message
func listenError(port int, err error) error {
	if errors.Is(err, windows.WSAEADDRINUSE) {
		return fmt.Errorf("port %d is already in use; find what holds it with: netstat -ano | findstr :%d",
			port, port)
	}
	return fmt.Errorf("listen: %w", err)
}
