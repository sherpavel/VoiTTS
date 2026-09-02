package main

import (
	"errors"
	"fmt"
	"syscall"
)

// OS-specific error check and message
func listenError(port int, err error) error {
	if errors.Is(err, syscall.EADDRINUSE) {
		return fmt.Errorf("port %d is already in use; find what holds it with: ss -ltnp 'sport = :%d'",
			port, port)
	}
	return fmt.Errorf("listen: %w", err)
}
