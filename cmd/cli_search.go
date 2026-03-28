package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/spf13/cobra"
)

var (
	searchQuery string
	searchLimit int
	searchSince string
	searchAgent bool
)

var searchCmd = &cobra.Command{
	Use:   "search",
	Short: "Full-text search across all unified message history",
	Run: func(cmd *cobra.Command, args []string) {
		if searchQuery == "" && len(args) > 0 {
			searchQuery = args[0]
		}
		if searchQuery == "" {
			fmt.Println("Error: query is required. Usage: chaind search --query 'hello'")
			os.Exit(1)
		}

		u := fmt.Sprintf("http://unix/api/v1/messages/search?q=%s&limit=%d",
			url.QueryEscape(searchQuery), searchLimit)
		if searchSince != "" {
			u += fmt.Sprintf("&since=%s", url.QueryEscape(searchSince))
		}

		req, err := http.NewRequest("GET", u, nil)
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
			fmt.Printf("Daemon not responding: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			fmt.Printf("Search failed (HTTP %d): %s\n", resp.StatusCode, string(body))
			os.Exit(1)
		}

		if searchAgent {
			fmt.Println(string(body))
		} else {
			var msgs []map[string]interface{}
			json.Unmarshal(body, &msgs)
			if len(msgs) == 0 {
				fmt.Println("No results found.")
				return
			}
			fmt.Printf("Found %d results:\n\n", len(msgs))
			for i, msg := range msgs {
				platform, _ := msg["platform"].(string)
				content, _ := msg["content"].(map[string]interface{})
				text := ""
				if content != nil {
					text, _ = content["text"].(string)
				}
				author, _ := msg["author"].(map[string]interface{})
				authorID := ""
				if author != nil {
					authorID, _ = author["id"].(string)
				}
				fmt.Printf("  %d. [%s] %s: %s\n", i+1, platform, authorID, text)
			}
		}
	},
}

func init() {
	searchCmd.Flags().StringVarP(&searchQuery, "query", "q", "", "Search query")
	searchCmd.Flags().IntVar(&searchLimit, "limit", 20, "Max results")
	searchCmd.Flags().StringVar(&searchSince, "since", "", "Filter results since time duration (e.g. 365d, 24h)")
	searchCmd.Flags().BoolVar(&searchAgent, "agent", false, "Raw JSON output for agent scripts")
	rootCmd.AddCommand(searchCmd)
}
