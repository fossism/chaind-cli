package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/fossism/chaind-cli/internal/db"
	"github.com/fossism/chaind-cli/internal/models"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start [repo] [number]",
	Short: "Start working on a task (sets status to in-progress and checks out branch)",
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
			fmt.Println("Task not found in local DB. Try `chaind sync`!")
			os.Exit(1)
		}

		task.LocalStatus = "in-progress"
		database.Save(&task)

		branchName := fmt.Sprintf("issue-%d", number)
		gitCmd := exec.Command("git", "checkout", "-b", branchName)
		if err := gitCmd.Run(); err != nil {
			fmt.Printf("Could not create git branch automatically: %v\n", err)
		} else {
			fmt.Printf("Checked out new branch: %s\n", branchName)
		}

		fmt.Printf("Marked `%s#%d` as in-progress!\n", repo, number)
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
}
