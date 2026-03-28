package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/fossism/chaind-cli/internal/format"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

var (
	sendPlatform string
	sendRoom     string
	sendText     string
	sendAt       string
	sendApproval bool
	sendDryRun   bool

	replyMsgID string
	reactEmoji string
)

var sendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send a message to a unified room",
	Run: func(cmd *cobra.Command, args []string) {
		payload := map[string]interface{}{
			"platform":         sendPlatform,
			"room":             sendRoom,
			"text":             sendText,
			"require_approval": sendApproval,
		}

		body, _ := json.Marshal(payload)
		req, err := http.NewRequest("POST", "http://unix/api/v1/messages/send", bytes.NewReader(body))
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
			fmt.Printf("Transmission failed: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			fmt.Printf("Daemon rejected send (HTTP %d): %s\n", resp.StatusCode, string(respBody))
			os.Exit(1)
		}

		fmt.Println("Status: delivered.")
		fmt.Println("Receipt:", string(respBody))
	},
}

var replyCmd = &cobra.Command{
	Use:   "reply",
	Short: "Reply to a specific message",
	Run: func(cmd *cobra.Command, args []string) {
		payload := map[string]interface{}{
			"platform": sendPlatform,
			"id":       replyMsgID,
			"text":     sendText,
		}

		body, _ := json.Marshal(payload)
		req, err := http.NewRequest("POST", "http://unix/api/v1/messages/reply", bytes.NewReader(body))
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
			fmt.Printf("Transmission failed: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			fmt.Printf("Daemon rejected reply (HTTP %d): %s\n", resp.StatusCode, string(respBody))
			os.Exit(1)
		}

		fmt.Println("Reply sent.")
		fmt.Println("Receipt:", string(respBody))
	},
}

var reactCmd = &cobra.Command{
	Use:   "react",
	Short: "React to a specific message with an emoji",
	Run: func(cmd *cobra.Command, args []string) {
		payload := map[string]interface{}{
			"platform": sendPlatform,
			"id":       replyMsgID,
			"emoji":    reactEmoji,
		}

		body, _ := json.Marshal(payload)
		req, err := http.NewRequest("POST", "http://unix/api/v1/messages/react", bytes.NewReader(body))
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
			fmt.Printf("Transmission failed: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			fmt.Printf("Daemon rejected reaction (HTTP %d): %s\n", resp.StatusCode, string(respBody))
			os.Exit(1)
		}

		fmt.Println("Reaction sent.")
	},
}

var deleteMsgCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a specific message",
	Run: func(cmd *cobra.Command, args []string) {
		payload := map[string]interface{}{
			"platform": sendPlatform,
			"id":       replyMsgID,
		}

		body, _ := json.Marshal(payload)
		req, err := http.NewRequest("POST", "http://unix/api/v1/messages/delete", bytes.NewReader(body))
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
			fmt.Printf("Transmission failed: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			fmt.Printf("Daemon rejected delete (HTTP %d): %s\n", resp.StatusCode, string(respBody))
			os.Exit(1)
		}

		fmt.Println("Message deleted.")
	},
}

var broadcastCmd = &cobra.Command{
	Use:   "broadcast",
	Short: "Broadcast a message to multiple rooms simultaneously",
	Run: func(cmd *cobra.Command, args []string) {
		if sendText == "" {
			fmt.Println("Error: --text is required")
			os.Exit(1)
		}

		if sendRoom == "" {
			fmt.Println("Error: --rooms is required (e.g., matrix:!roomID,telegram:-10012345)")
			os.Exit(1)
		}

		rooms := strings.Split(sendRoom, ",")

		astNodes := format.ParseMarkdown(sendText)

		var eg errgroup.Group
		results := make(map[string]map[string]string)
		var mu sync.Mutex

		for _, combo := range rooms {
			parts := strings.SplitN(combo, ":", 2)
			if len(parts) != 2 {
				fmt.Printf("Invalid room format (platform:room): %s\n", combo)
				continue
			}
			platformStr, roomStr := parts[0], parts[1]

			eg.Go(func() error {
				var formatted string
				switch platformStr {
				case "telegram":
					formatted = format.TelegramRenderer{}.Render(astNodes)
				case "matrix":
					formatted = format.MatrixRenderer{}.Render(astNodes)
				default:
					formatted = format.PlainRenderer{}.Render(astNodes)
				}

				if sendDryRun {
					mu.Lock()
					if results[platformStr] == nil {
						results[platformStr] = make(map[string]string)
					}
					results[platformStr][roomStr] = "[DRY_RUN] " + formatted
					mu.Unlock()
					return nil
				}

				payload := map[string]interface{}{
					"platform":         platformStr,
					"room":             roomStr,
					"text":             formatted,
					"require_approval": sendApproval,
				}

				body, _ := json.Marshal(payload)
				req, _ := http.NewRequest("POST", "http://unix/api/v1/messages/send", bytes.NewReader(body))
				tok := os.Getenv("CHAIND_TOKEN")
				if tok != "" {
					req.Header.Set("Authorization", "Bearer "+tok)
				}

				resp, err := IPCClient().Do(req)
				resText := ""
				if err != nil {
					resText = fmt.Sprintf("Error: %v", err)
				} else {
					defer resp.Body.Close()
					respBody, _ := io.ReadAll(resp.Body)
					resText = string(respBody)
					if resp.StatusCode != http.StatusOK {
						resText = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, resText)
					}
				}

				mu.Lock()
				if results[platformStr] == nil {
					results[platformStr] = make(map[string]string)
				}
				results[platformStr][roomStr] = resText
				mu.Unlock()

				return nil
			})
		}

		_ = eg.Wait()

		out, _ := json.MarshalIndent(results, "", "  ")
		fmt.Println(string(out))
	},
}

func init() {
	sendCmd.Flags().StringVar(&sendPlatform, "platform", "matrix", "Target platform")
	sendCmd.Flags().StringVar(&sendRoom, "room", "", "Room alias")
	sendCmd.Flags().StringVar(&sendText, "text", "", "Message content")
	sendCmd.Flags().StringVar(&sendAt, "at", "", "Schedule time")
	sendCmd.Flags().BoolVar(&sendApproval, "require-approval", false, "Require HitL")

	replyCmd.Flags().StringVar(&sendPlatform, "platform", "matrix", "Target platform")
	replyCmd.Flags().StringVar(&replyMsgID, "id", "", "ULID of the message to reply to")
	replyCmd.Flags().StringVar(&sendText, "text", "", "Reply content")

	reactCmd.Flags().StringVar(&sendPlatform, "platform", "matrix", "Target platform")
	reactCmd.Flags().StringVar(&replyMsgID, "id", "", "ULID of the message to react to")
	reactCmd.Flags().StringVar(&reactEmoji, "emoji", "", "Emoji to react with")

	deleteMsgCmd.Flags().StringVar(&sendPlatform, "platform", "matrix", "Target platform")
	deleteMsgCmd.Flags().StringVar(&replyMsgID, "id", "", "ULID of the message to delete")

	broadcastCmd.Flags().StringVar(&sendPlatform, "platform", "matrix,telegram", "Target platforms")
	broadcastCmd.Flags().StringVar(&sendRoom, "rooms", "", "Comma-separated rooms (e.g. matrix:!room1,telegram:-100123)")
	broadcastCmd.Flags().StringVar(&sendText, "text", "", "Message content")
	broadcastCmd.Flags().BoolVar(&sendDryRun, "dry-run", false, "Test formats without sending")
	broadcastCmd.Flags().BoolVar(&sendApproval, "require-approval", false, "Require HitL")

	rootCmd.AddCommand(sendCmd)
	rootCmd.AddCommand(replyCmd)
	rootCmd.AddCommand(reactCmd)
	rootCmd.AddCommand(deleteMsgCmd)
	rootCmd.AddCommand(broadcastCmd)
}
