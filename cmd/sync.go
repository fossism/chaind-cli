package cmd

import (
	"fmt"
	"os"

	"github.com/charmbracelet/huh/spinner"
	"github.com/fossism/chaind-cli/internal/config"
	"github.com/fossism/chaind-cli/internal/db"
	"github.com/fossism/chaind-cli/internal/github"
	"github.com/fossism/chaind-cli/internal/models"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync your assigned issues and PRs from GitHub",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Println("Not authenticated. Run `chaind auth` first.")
			os.Exit(1)
		}

		var tasks []models.Task
		action := func() {
			var errFetch error
			tasks, errFetch = github.FetchAssignedTasks(cfg)
			err = errFetch
		}

		err = spinner.New().
			Title("Fetching tasks from GitHub...").
			Action(action).
			Run()

		if err != nil {
			fmt.Printf("Error fetching tasks: %v\n", err)
			os.Exit(1)
		}

		database, _ := db.InitDB()
		syncedCount := 0

		for _, t := range tasks {
			var existing models.Task
			res := database.Where("github_id = ?", t.GithubID).First(&existing)
			
			if res.RowsAffected > 0 {
				existing.Title = t.Title
				existing.State = t.State
				existing.UpdatedAt = t.UpdatedAt
				if existing.State == "closed" {
					existing.LocalStatus = "done"
				}
				database.Save(&existing)
			} else {
				database.Create(&t)
			}
			syncedCount++
		}

		fmt.Printf("Successfully synced %d tasks!\n", syncedCount)
	},
}

func init() {
	rootCmd.AddCommand(syncCmd)
}
