package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/fossism/chaind-cli/internal/auth"
	"github.com/spf13/cobra"
)

var (
	tokName    string
	tokRole    string
	tokScopes  string
	tokExpires string
	tokPii     string
)

var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Manage IPC Capability Tokens",
}

var tokenIssueCmd = &cobra.Command{
	Use:   "issue",
	Short: "Issue a new capability token",
	Run: func(cmd *cobra.Command, args []string) {
		// Generate a cryptographically secure random token
		tokenBytes := make([]byte, 32)
		if _, err := rand.Read(tokenBytes); err != nil {
			fmt.Printf("Failed to generate token: %v\n", err)
			return
		}
		token := hex.EncodeToString(tokenBytes)

		name := tokName
		if name == "" {
			name = tokRole
		}

		// Store it in the OS Keyring so the daemon can validate it
		if err := auth.SaveCredential("chaind-token-"+name, token); err != nil {
			fmt.Printf("Failed to persist token: %v\n", err)
			return
		}

		// Print only the raw token so it can be captured by shell scripts:
		// export CHAIND_TOKEN=$(./chaind token issue --role owner)
		fmt.Print(token)
	},
}

var tokenListCmd = &cobra.Command{
	Use:   "list",
	Short: "List active capability tokens",
	Run: func(cmd *cobra.Command, args []string) {
		// Check for the common roles
		roles := []string{"owner", "agent", "readonly"}
		fmt.Println("Active tokens:")
		for _, role := range roles {
			_, err := auth.GetCredential("chaind-token-" + role)
			if err == nil {
				fmt.Printf("  ✓ %s\n", role)
			}
		}
	},
}

var tokenRevokeCmd = &cobra.Command{
	Use:   "revoke [name]",
	Short: "Revoke a token",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		// Overwrite with empty string to effectively revoke
		if err := auth.SaveCredential("chaind-token-"+name, ""); err != nil {
			fmt.Printf("Failed to revoke: %v\n", err)
			return
		}
		fmt.Printf("Revoked token: %s\n", name)
	},
}

func init() {
	tokenIssueCmd.Flags().StringVar(&tokName, "name", "", "Identifier for this token")
	tokenIssueCmd.Flags().StringVar(&tokRole, "role", "agent", "Role: owner, agent, readonly")
	tokenIssueCmd.Flags().StringVar(&tokScopes, "scopes", "", "Authorization scopes")
	tokenIssueCmd.Flags().StringVar(&tokExpires, "expires", "30d", "Expiry")
	tokenIssueCmd.Flags().StringVar(&tokPii, "pii-scrub", "", "PII to scrub (email,phone,pan)")

	tokenCmd.AddCommand(tokenIssueCmd)
	tokenCmd.AddCommand(tokenListCmd)
	tokenCmd.AddCommand(tokenRevokeCmd)
	rootCmd.AddCommand(tokenCmd)
}
