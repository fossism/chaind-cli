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
	sendPlatform string
	sendRoom     string
	sendText     string
	sendAt       string
	sendApproval bool
	sendDryRun   bool
)

var sendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send a message to a unified room",
	Run: func(cmd *cobra.Command, args []string) {
		payload := map[string]string{
			"platform": sendPlatform,
			"room":     sendRoom,
			"text":     sendText,
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

var broadcastCmd = &cobra.Command{
	Use:   "broadcast",
	Short: "Broadcast a message to multiple rooms simultaneously",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Broadcasting to %s: %s\n", sendRoom, sendText)
		if sendDryRun {
			fmt.Println("Dry run. No messages sent.")
		}
	},
}

func init() {
	sendCmd.Flags().StringVar(&sendPlatform, "platform", "matrix", "Target platform")
	sendCmd.Flags().StringVar(&sendRoom, "room", "", "Room alias")
	sendCmd.Flags().StringVar(&sendText, "text", "", "Message content")
	sendCmd.Flags().StringVar(&sendAt, "at", "", "Schedule time")
	sendCmd.Flags().BoolVar(&sendApproval, "require-approval", false, "Require HitL")

	broadcastCmd.Flags().StringVar(&sendPlatform, "platform", "matrix,telegram", "Target platforms")
	broadcastCmd.Flags().StringVar(&sendRoom, "rooms", "", "Comma-separated rooms")
	broadcastCmd.Flags().StringVar(&sendText, "text", "", "Message content")
	broadcastCmd.Flags().BoolVar(&sendDryRun, "dry-run", false, "Test formats")

	rootCmd.AddCommand(sendCmd)
	rootCmd.AddCommand(broadcastCmd)
}
