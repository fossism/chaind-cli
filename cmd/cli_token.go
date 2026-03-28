package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/fossism/chaind-cli/internal/store"
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
			name = token[:8] // use prefix as default name if not provided
		}

		tier := 2
		if tokRole == "owner" {
			tier = 0
		} else if tokRole == "readonly" {
			tier = 4
		}

		st, err := store.NewStore()
		if err != nil {
			fmt.Printf("Failed to open store: %v\n", err)
			return
		}
		defer st.Close()

		t := store.Token{
			Name:     token,
			Tier:     tier,
			Rooms:    tokScopes,
			PiiScrub: tokPii,
			Expires:  time.Now().Add(365 * 24 * time.Hour).Format(time.RFC3339), // default 1 year
			Revoked:  false,
		}

		if err := st.SaveToken(context.Background(), t); err != nil {
			fmt.Printf("Failed to persist token to DB: %v\n", err)
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
		st, err := store.NewStore()
		if err != nil {
			fmt.Printf("Failed to open store: %v\n", err)
			return
		}
		defer st.Close()

		tokens, err := st.ListTokens(context.Background())
		if err != nil {
			fmt.Printf("Failed to list tokens: %v\n", err)
			return
		}

		fmt.Println("Active tokens:")
		for _, t := range tokens {
			status := "active"
			if t.Revoked {
				status = "revoked"
			}
			fmt.Printf("  %s... (Tier: %d, Scopes: %s, Status: %s)\n", t.Name[:8], t.Tier, t.Rooms, status)
		}
	},
}

var tokenRevokeCmd = &cobra.Command{
	Use:   "revoke [token_prefix]",
	Short: "Revoke a token",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		prefix := args[0]
		st, err := store.NewStore()
		if err != nil {
			fmt.Printf("Failed to open store: %v\n", err)
			return
		}
		defer st.Close()

		tokens, _ := st.ListTokens(context.Background())
		for _, t := range tokens {
			if t.Name == prefix || (len(prefix) >= 8 && t.Name[:len(prefix)] == prefix) {
				if err := st.RevokeToken(context.Background(), t.Name); err != nil {
					fmt.Printf("Failed to revoke %s: %v\n", t.Name, err)
				} else {
					fmt.Printf("Revoked token: %s\n", t.Name)
				}
				return
			}
		}
		fmt.Printf("Token not found: %s\n", prefix)
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
