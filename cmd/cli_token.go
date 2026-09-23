package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
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
			Name:     store.HashToken(token),
			Tier:     tier,
			Rooms:    tokScopes,
			PiiScrub: tokPii,
			Expires:  parseTokenExpiry(tokExpires),
			Revoked:  false,
		}

		if err := st.SaveToken(context.Background(), t); err != nil {
			fmt.Printf("Failed to persist token to DB: %v\n", err)
			return
		}

		if tokName != "" {
			fmt.Fprintf(os.Stderr, "Issued token %q (hash %.8s...)\n", tokName, t.Name)
		}
		// Print only the raw token so it can be captured by shell scripts:
		// export CHAIND_TOKEN=$(./chaind token issue --role owner)
		fmt.Print(token)
	},
}

// parseTokenExpiry honors --expires (e.g. 30d, 24h, 720h). Falls back to 30d.
func parseTokenExpiry(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		s = "30d"
	}
	if strings.HasSuffix(s, "d") {
		if n, err := strconv.Atoi(strings.TrimSuffix(s, "d")); err == nil && n > 0 && n <= 3650 {
			return time.Now().Add(time.Duration(n) * 24 * time.Hour).Format(time.RFC3339)
		}
	} else if d, err := time.ParseDuration(s); err == nil && d > 0 && d <= 10*365*24*time.Hour {
		return time.Now().Add(d).Format(time.RFC3339)
	}
	return time.Now().Add(30 * 24 * time.Hour).Format(time.RFC3339)
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
			if t.IsExpired(time.Now()) {
				status += ", expired"
			}
			fmt.Printf("  %s... (Tier: %d, Scopes: %s, Status: %s)\n", shortHash(t.Name), t.Tier, t.Rooms, status)
		}
	},
}

var tokenRevokeCmd = &cobra.Command{
	Use:   "revoke [token_prefix]",
	Short: "Revoke a token",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		prefix := strings.TrimSpace(args[0])
		if prefix == "" {
			fmt.Println("Token prefix must not be empty")
			return
		}
		st, err := store.NewStore()
		if err != nil {
			fmt.Printf("Failed to open store: %v\n", err)
			return
		}
		defer st.Close()

		tokens, _ := st.ListTokens(context.Background())
		// Accept hash prefix, full hash, legacy name, or raw secret.
		rawHash := store.HashToken(prefix)
		for _, t := range tokens {
			if t.Name == prefix || t.Name == rawHash ||
				(len(prefix) >= 8 && strings.HasPrefix(t.Name, prefix)) {
				if err := st.RevokeToken(context.Background(), t.Name); err != nil {
					fmt.Printf("Failed to revoke %s: %v\n", shortHash(t.Name), err)
				} else {
					fmt.Printf("Revoked token: %s...\n", shortHash(t.Name))
				}
				return
			}
		}
		fmt.Printf("Token not found: %s\n", prefix)
	},
}

// shortHash safely truncates a stored hash for display.
func shortHash(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

// isHashPrefix reports whether s is already a hex prefix of a stored hash.
func isHashPrefix(s string) bool {
	if len(s) < 8 || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
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
