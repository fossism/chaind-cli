package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
)

var (
	tokName    string
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
		fmt.Printf("Issued token: %s (scopes: %s)\n", tokName, tokScopes)
		fmt.Println("Keep this secret.")
	},
}

var tokenListCmd = &cobra.Command{
	Use:   "list",
	Short: "List active capability tokens",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Active tokens:")
		fmt.Println("- owner (tier 0)")
	},
}

var tokenRevokeCmd = &cobra.Command{
	Use:   "revoke [name]",
	Short: "Revoke a token",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Revoked %s.\n", args[0])
	},
}

func init() {
	tokenIssueCmd.Flags().StringVar(&tokName, "name", "", "Identifier")
	tokenIssueCmd.Flags().StringVar(&tokScopes, "scopes", "", "Authorization scopes")
	tokenIssueCmd.Flags().StringVar(&tokExpires, "expires", "30d", "Expiry")
	tokenIssueCmd.Flags().StringVar(&tokPii, "pii-scrub", "", "PII to scrub (email,phone,pan)")
	
	tokenCmd.AddCommand(tokenIssueCmd)
	tokenCmd.AddCommand(tokenListCmd)
	tokenCmd.AddCommand(tokenRevokeCmd)
	rootCmd.AddCommand(tokenCmd)
}
