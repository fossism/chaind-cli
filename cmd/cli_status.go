package cmd

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the health and status of the chaind daemon",
	Run: func(cmd *cobra.Command, args []string) {
		home, _ := os.UserHomeDir()
		sockPath := filepath.Join(home, ".config", "chaind", "chaind.sock")

		fmt.Println("=== chaind system status ===")
		
		// 1. Check socket file
		if _, err := os.Stat(sockPath); os.IsNotExist(err) {
			fmt.Println("🔴 Daemon: NOT RUNNING (Socket missing)")
			fmt.Println("   FIX: Run './chaind daemon start' to boot the system.")
			os.Exit(1)
		}

		// 2. Try to ping daemon via IPC
		req, _ := http.NewRequest("GET", "http://unix/api/v1/adapters/status", nil)
		tok := os.Getenv("CHAIND_TOKEN")
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}

		resp, err := IPCClient().Do(req)
		if err != nil {
			fmt.Println("🟡 Daemon: ZOMBIE (Socket exists but process not responding)")
			fmt.Println("   FIX: Run 'pkill chaind' then './chaind daemon start' to recover.")
			os.Exit(1)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			fmt.Println("🟢 Daemon: HEALTHY & LISTENING")
		} else if resp.StatusCode == http.StatusUnauthorized {
			fmt.Println("🟠 Daemon: ALIVE (But local CHAIND_TOKEN is missing/invalid)")
		} else {
			fmt.Printf("🔴 Daemon: ERROR (HTTP %d)\n", resp.StatusCode)
		}
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
