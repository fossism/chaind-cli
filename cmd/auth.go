package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/fossism/chaind-cli/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate with GitHub using a Personal Access Token",
	Run: func(cmd *cobra.Command, args []string) {
		reader := bufio.NewReader(os.Stdin)

		fmt.Print("GitHub Username: ")
		username, _ := reader.ReadString('\n')
		username = strings.TrimSpace(username)

		fmt.Print("GitHub Personal Access Token (hidden input): ")
		byteToken, _ := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		token := strings.TrimSpace(string(byteToken))

		err := config.Save(username, token)
		if err != nil {
			fmt.Printf("Error saving config: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Successfully authenticated & cached locally!")
	},
}

func init() {
	rootCmd.AddCommand(authCmd)
}
