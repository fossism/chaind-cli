package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/fossism/chaind-cli/internal/auth"
)

var authCmd = &cobra.Command{
	Use:   "auth [platform]",
	Short: "Authenticate with a specific platform (whatsapp, telegram, matrix)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		platform := args[0]
		switch platform {
		case "whatsapp":
			fmt.Println("WhatsApp auth requires running chaind daemon directly to scan the QR code in the stdout.")
		case "telegram":
			fmt.Print("Enter Telegram Bot/MTProto Token: ")
			reader := bufio.NewReader(os.Stdin)
			token, _ := reader.ReadString('\n')
			token = strings.TrimSpace(token)
			if err := auth.SaveCredential("telegram", token); err != nil {
				fmt.Printf("Failed to secure token: %v\n", err)
			} else {
				fmt.Println("Telegram token securely stored in OS Keyring.")
			}
		case "matrix":
			fmt.Print("Enter Matrix Access Token: ")
			reader := bufio.NewReader(os.Stdin)
			token, _ := reader.ReadString('\n')
			token = strings.TrimSpace(token)
			if err := auth.SaveCredential("matrix", token); err != nil {
				fmt.Printf("Failed to secure token: %v\n", err)
			} else {
				fmt.Println("Matrix token securely stored in OS Keyring.")
			}
		default:
			fmt.Printf("Unknown platform: %s\n", platform)
			fmt.Println("Supported: whatsapp, telegram, matrix")
		}
	},
}

func init() {
	rootCmd.AddCommand(authCmd)
}
