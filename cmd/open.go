package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"

	"github.com/fossism/chaind-cli/internal/db"
	"github.com/fossism/chaind-cli/internal/models"
	"github.com/spf13/cobra"
)

var openCmd = &cobra.Command{
	Use:   "open [repo] [number]",
	Short: "Open the task in your browser",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		repo := args[0]
		numberStr := args[1]
		number, err := strconv.Atoi(numberStr)
		if err != nil {
			fmt.Println("Invalid issue/PR number")
			os.Exit(1)
		}

		database, _ := db.InitDB()
		var task models.Task
		res := database.Where("repo = ? AND number = ?", repo, number).First(&task)

		if res.RowsAffected == 0 {
			fmt.Println("Task not found in local DB.")
			os.Exit(1)
		}

		fmt.Printf("Opening %s...\n", task.HTMLURL)
		openURL(task.HTMLURL)
	},
}

func openURL(url string) {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		fmt.Printf("Failed to open browser: %v\n", err)
	}
}

func init() {
	rootCmd.AddCommand(openCmd)
}
