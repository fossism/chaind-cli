package cmd

import (
	"context"
	"fmt"
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
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				conn, err := net.Dial("unix", sockPath)
				if err != nil {
					if _, statErr := os.Stat(sockPath); os.IsNotExist(statErr) {
						return nil, fmt.Errorf("daemon is not running (socket missing at %s). \nFIX: Run './chaind daemon start' to boot the background engine.", sockPath)
					}
					return nil, fmt.Errorf("daemon connection refused (socket exists but no one is listening at %s). \nFIX: Run 'pkill chaind' and then './chaind daemon start' to reset the system.", sockPath)
				}
				return conn, nil
			},
		},
	}
}
