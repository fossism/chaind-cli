package cmd

import (
	"database/sql"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/fossism/chaind-cli/internal/auth"
	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/cobra"
)

var doctorFix bool

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run self-diagnostics on chaind configuration and store",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("chaind doctor — diagnostic report")
		fmt.Println("─────────────────────────────────")
		home, _ := os.UserHomeDir()
		passed := 0
		failed := 0

		// 1. Check data directory
		dataDir := filepath.Join(home, ".local", "share", "chaind")
		if info, err := os.Stat(dataDir); err == nil && info.IsDir() {
			fmt.Printf("  ✓ Data directory exists: %s\n", dataDir)
			passed++
		} else {
			fmt.Printf("  ✗ Data directory missing: %s\n", dataDir)
			failed++
			if doctorFix {
				os.MkdirAll(dataDir, 0700)
				fmt.Println("    → Created.")
			}
		}

		// 2. Check SQLite database
		dbPath := filepath.Join(dataDir, "messages.db")
		if _, err := os.Stat(dbPath); err == nil {
			fmt.Printf("  ✓ SQLite database exists: %s\n", dbPath)
			passed++
			
			db, err := sql.Open("sqlite3", dbPath)
			if err == nil {
				defer db.Close()
				var checkResult string
				err = db.QueryRow("PRAGMA integrity_check;").Scan(&checkResult)
				if err == nil && checkResult == "ok" {
					fmt.Printf("  ✓ SQLite PRAGMA integrity_check passed\n")
					passed++
				} else {
					fmt.Printf("  ✗ SQLite integrity error: %v (result: %s)\n", err, checkResult)
					failed++
				}
			} else {
				fmt.Printf("  ✗ Failed to open SQLite database: %v\n", err)
				failed++
			}
		} else {
			fmt.Println("  ✗ SQLite database missing (run 'chaind daemon start' to create)")
			failed++
		}

		// 3. Check media directory
		mediaDir := filepath.Join(dataDir, "media")
		if info, err := os.Stat(mediaDir); err == nil && info.IsDir() {
			fmt.Printf("  ✓ Media cache directory exists: %s\n", mediaDir)
			passed++
		} else {
			fmt.Println("  ✗ Media cache directory missing")
			failed++
			if doctorFix {
				os.MkdirAll(mediaDir, 0700)
				fmt.Println("    → Created.")
			}
		}

		// 4. Check config directory
		configDir := filepath.Join(home, ".config", "chaind")
		if info, err := os.Stat(configDir); err == nil && info.IsDir() {
			fmt.Printf("  ✓ Config directory exists: %s\n", configDir)
			passed++
		} else {
			fmt.Printf("  ✗ Config directory missing: %s\n", configDir)
			failed++
		}

		// 5. Check Unix socket
		sockPath := filepath.Join(configDir, "chaind.sock")
		if _, err := os.Stat(sockPath); err == nil {
			// Try connecting
			conn, err := net.DialTimeout("unix", sockPath, 2*time.Second)
			if err == nil {
				conn.Close()
				fmt.Printf("  ✓ Daemon socket active: %s\n", sockPath)
				passed++
			} else {
				fmt.Printf("  ✗ Socket exists but daemon not responding: %s\n", sockPath)
				failed++
				if doctorFix {
					os.Remove(sockPath)
					fmt.Println("    → Removed stale socket.")
				}
			}
		} else {
			fmt.Println("  ✗ Daemon not running (socket missing)")
			failed++
		}

		// 6. Socket permissions
		if info, err := os.Stat(sockPath); err == nil {
			perm := info.Mode().Perm()
			if perm == 0600 {
				fmt.Printf("  ✓ Socket permissions correct: %o\n", perm)
				passed++
			} else {
				fmt.Printf("  ✗ Socket permissions insecure: %o (expected 0600)\n", perm)
				failed++
				if doctorFix {
					os.Chmod(sockPath, 0600)
					fmt.Println("    → Fixed to 0600.")
				}
			}
		}

		// 7. Check platform credentials & keyring
		err := auth.SaveCredential("chaind_doctor_test", "ok")
		if err == nil {
			auth.DeleteCredential("chaind_doctor_test")
			fmt.Println("  ✓ System go-keyring service available and writable")
			passed++
		} else {
			fmt.Printf("  ✗ System go-keyring service unavailable: %v\n", err)
			failed++
		}

		platforms := []string{"telegram", "matrix"}
		for _, p := range platforms {
			_, err := auth.GetCredential(p)
			if err == nil {
				fmt.Printf("  ✓ %s credentials found in keyring\n", p)
				passed++
			} else {
				fmt.Printf("  ✗ %s credentials missing (run 'chaind auth %s')\n", p, p)
				failed++
			}
		}

		// 7b. Check WhatsApp session
		waDB := filepath.Join(dataDir, "whatsapp.db")
		if _, err := os.Stat(waDB); err == nil {
			fmt.Printf("  ✓ whatsapp session found: %s\n", waDB)
			passed++
		} else {
			fmt.Println("  ! whatsapp session missing (link via 'chaind daemon start')")
		}

		// 8. Check environment variables
		envVars := []string{"CHAIND_TELEGRAM_API_ID", "CHAIND_TELEGRAM_API_HASH"}
		for _, env := range envVars {
			if os.Getenv(env) != "" {
				fmt.Printf("  ✓ %s is set\n", env)
				passed++
			} else {
				fmt.Printf("  ! %s not set (using defaults)\n", env)
			}
		}

		fmt.Println("─────────────────────────────────")
		fmt.Printf("Results: %d passed, %d failed\n", passed, failed)
		if failed > 0 {
			fmt.Println("Run 'chaind doctor --fix' to attempt auto-repair.")
		} else {
			fmt.Println("All systems nominal.")
		}
	},
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorFix, "fix", false, "Attempt auto-repair of issues")
	rootCmd.AddCommand(doctorCmd)
}
