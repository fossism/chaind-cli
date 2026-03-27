package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

var lsToken string

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List recent unified messages from the local IPC socket (AI Agent PoC)",
	Run: func(cmd *cobra.Command, args []string) {
		// Construct secure request
		req, err := http.NewRequest("GET", "http://unix/api/v1/messages/recent", nil)
		if err != nil {
			fmt.Printf("Failed to build request: %v\n", err)
			os.Exit(1)
		}

		if lsToken != "" {
			req.Header.Set("Authorization", "Bearer "+lsToken)
		}

		// Dial via the HTTP Host standard using shared client
		resp, err := IPCClient().Do(req)
		if err != nil {
			fmt.Printf("Failed to contact chaind daemon: %v\n", err)
			fmt.Println("Is it running? (run 'chaind daemon start')")
			os.Exit(1)
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("IPC Request Failed (HTTP %d): %s\n", resp.StatusCode, string(body))
			os.Exit(1)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Printf("Error reading response: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("=== UNIFIED LOCAL MESSAGE AGENT VIEW ===")
		fmt.Println(string(body))
	},
}

func init() {
	lsCmd.Flags().StringVarP(&lsToken, "token", "t", "", "Capability Token for IPC Authorization")
	rootCmd.AddCommand(lsCmd)
}
