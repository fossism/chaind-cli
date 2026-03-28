package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

var approveCmd = &cobra.Command{
	Use:   "approve",
	Short: "Human-in-the-Loop approval queue management",
}

var approveListCmd = &cobra.Command{
	Use:   "list",
	Short: "List pending high-risk actions",
	Run: func(cmd *cobra.Command, args []string) {
		req, _ := http.NewRequest("GET", "http://unix/api/v1/queue", nil)
		tok := os.Getenv("CHAIND_TOKEN")
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}

		resp, err := IPCClient().Do(req)
		if err != nil {
			fmt.Printf("Daemon offline: %v\n", err)
			return
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			fmt.Printf("Error (HTTP %d): %s\n", resp.StatusCode, string(body))
			return
		}

		var items []map[string]interface{}
		json.Unmarshal(body, &items)
		if len(items) == 0 {
			fmt.Println("No pending actions.")
			return
		}

		for _, it := range items {
			fmt.Printf("- [%s] %s to %s : %s\n", it["id"], it["action_type"], it["platform"], it["payload"])
		}
	},
}

var approveExecCmd = &cobra.Command{
	Use:   "exec [req_id]",
	Short: "Execute a pending action",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		req, _ := http.NewRequest("POST", "http://unix/api/v1/queue/exec?id="+args[0], nil)
		tok := os.Getenv("CHAIND_TOKEN")
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}

		resp, err := IPCClient().Do(req)
		if err != nil {
			fmt.Printf("Daemon offline: %v\n", err)
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			fmt.Printf("Error: %s\n", string(body))
			return
		}
		fmt.Printf("Executed %s.\nResult: %s\n", args[0], string(body))
	},
}

var approveDenyCmd = &cobra.Command{
	Use:   "deny [req_id]",
	Short: "Deny a pending action",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		req, _ := http.NewRequest("POST", "http://unix/api/v1/queue/deny?id="+args[0], nil)
		tok := os.Getenv("CHAIND_TOKEN")
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}

		resp, err := IPCClient().Do(req)
		if err != nil {
			fmt.Printf("Daemon offline: %v\n", err)
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			fmt.Printf("Error: %s\n", string(body))
			return
		}
		fmt.Printf("Denied %s.\n", args[0])
	},
}

func init() {
	approveCmd.AddCommand(approveListCmd)
	approveCmd.AddCommand(approveExecCmd)
	approveCmd.AddCommand(approveDenyCmd)
	rootCmd.AddCommand(approveCmd)
}
