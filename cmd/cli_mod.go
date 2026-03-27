package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

var (
	modAction    string
	modUser      string
	modReason    string
	modPlatform  string
	modRoom      string
	modDryRun    bool
)

var modCmd = &cobra.Command{
	Use:   "moderate",
	Short: "Cross-platform moderation tools",
	Run: func(cmd *cobra.Command, args []string) {
		payload := map[string]string{
			"platform": modPlatform,
			"room":     modRoom,
			"user":     modUser,
			"reason":   modReason,
		}
		
		body, _ := json.Marshal(payload)
		req, err := http.NewRequest("POST", "http://unix/api/v1/moderate", bytes.NewReader(body))
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		tok := os.Getenv("CHAIND_TOKEN")
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}

		resp, err := IPCClient().Do(req)
		if err != nil {
			fmt.Printf("Moderation request failed: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			fmt.Printf("Daemon rejected moderation (HTTP %d): %s\n", resp.StatusCode, string(respBody))
			os.Exit(1)
		}

		fmt.Println("Executed cleanly.")
		fmt.Println(string(respBody))
	},
}

func init() {
	modCmd.Flags().StringVar(&modAction, "action", "", "ban|unban|mute|kick")
	modCmd.Flags().StringVar(&modPlatform, "platform", "matrix", "Target platform")
	modCmd.Flags().StringVar(&modRoom, "room", "", "Target room ID")
	modCmd.Flags().StringVar(&modUser, "user", "", "User identity")
	modCmd.Flags().StringVar(&modReason, "reason", "", "Audit log reason")
	modCmd.Flags().BoolVar(&modDryRun, "dry-run", false, "Test mode")
	
	rootCmd.AddCommand(modCmd)
}
