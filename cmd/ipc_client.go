package cmd

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
)

// IPCClient constructs an HTTP client that communicates over the local Unix socket
func IPCClient() *http.Client {
	home, _ := os.UserHomeDir()
	sockPath := filepath.Join(home, ".config", "chaind", "chaind.sock")

	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", sockPath)
			},
		},
	}
}
