package cmd

import (
	"fmt"
	"os"

	"github.com/fossism/chaind-cli/internal/db"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "chaind",
	Short: "ChainD - Developer Workflow Bridge",
	Long:  `ChainD brings your open-source tasks directly into your local development environment.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	// Initialize DB on run
	_, err := db.InitDB()
	if err != nil {
		fmt.Printf("Error initializing DB: %v\n", err)
		os.Exit(1)
	}
}
