package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/fossism/chaind-cli/internal/db"
	"github.com/fossism/chaind-cli/internal/models"
	"github.com/spf13/cobra"
)

var doneCmd = &cobra.Command{
	Use:   "done [repo] [number]",
	Short: "Mark a task as done locally",
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

		task.LocalStatus = "done"
		database.Save(&task)

		fmt.Printf("Awesome! `%s#%d` marked as done.\n", repo, number)
		fmt.Printf("To close it out, use this commit format:\n  git commit -m \"Fixes #%d: %s\"\n", number, task.Title)
	},
}

func init() {
	rootCmd.AddCommand(doneCmd)
}
