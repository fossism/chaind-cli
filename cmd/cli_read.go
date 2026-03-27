package cmd

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

var (
	readPlatform string
	readRoom     string
	readSince    string
	readLimit    int
	readToken    string
	readAgent    bool
)

var readCmd = &cobra.Command{
	Use:   "read",
	Short: "Read historical messages from unified platforms",
	Run: func(cmd *cobra.Command, args []string) {
		req, err := http.NewRequest("GET", "http://unix/api/v1/messages/recent", nil)
		if err != nil {
			fmt.Println("Error building request:", err)
			return
		}

		if readToken != "" {
			req.Header.Set("Authorization", "Bearer "+readToken)
		} else if os.Getenv("CHAIND_TOKEN") != "" {
			req.Header.Set("Authorization", "Bearer "+os.Getenv("CHAIND_TOKEN"))
		}

		resp, err := IPCClient().Do(req)
		if err != nil {
			fmt.Printf("Daemon not responding: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			fmt.Printf("Failed: HTTP %d %s\n", resp.StatusCode, string(b))
			os.Exit(1)
		}

		// Process JSON payload and format output depending on --agent flag
		body, _ := io.ReadAll(resp.Body)
		if readAgent {
			fmt.Println(string(body)) // Raw JSON schema out
		} else {
			fmt.Printf("Fetched %d bytes of historic messages\n", len(body))
			// Real implementation would unmarshal and print human-readable CLI chat logs here
		}
	},
}

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Live stream new messages from chaind daemon",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Connecting to realtime interface for %s room %s...\n", readPlatform, readRoom)
		url := fmt.Sprintf("http://unix/api/v1/messages/watch?platform=%s&room=%s", readPlatform, readRoom)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			fmt.Println("Error building request:", err)
			return
		}

		if readToken != "" {
			req.Header.Set("Authorization", "Bearer "+readToken)
		} else if os.Getenv("CHAIND_TOKEN") != "" {
			req.Header.Set("Authorization", "Bearer "+os.Getenv("CHAIND_TOKEN"))
		}

		resp, err := IPCClient().Do(req)
		if err != nil {
			fmt.Printf("Daemon not responding: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			fmt.Printf("Failed: HTTP %d %s\n", resp.StatusCode, string(b))
			os.Exit(1)
		}

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			fmt.Println(line)
		}
	},
}

func init() {
	readCmd.Flags().StringVar(&readPlatform, "platform", "matrix", "Target platform")
	readCmd.Flags().StringVar(&readRoom, "room", "", "Filter by room alias")
	readCmd.Flags().StringVar(&readSince, "since", "24h", "Time window")
	readCmd.Flags().IntVar(&readLimit, "limit", 50, "Max messages")
	readCmd.Flags().StringVar(&readToken, "token", "", "Capability Token (default: $CHAIND_TOKEN)")
	readCmd.Flags().BoolVar(&readAgent, "agent", false, "Strict JSON output for agent scripts")

	watchCmd.Flags().StringVar(&readPlatform, "platform", "matrix", "Target platform")
	watchCmd.Flags().StringVar(&readRoom, "room", "", "Watch specific room")
	watchCmd.Flags().StringVar(&readToken, "token", "", "Capability Token (default: $CHAIND_TOKEN)")
	watchCmd.Flags().BoolVar(&readAgent, "agent", false, "Strict JSON stream")

	rootCmd.AddCommand(readCmd)
	rootCmd.AddCommand(watchCmd)
}
