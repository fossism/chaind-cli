package cmd

import (
	"fmt"
	"os"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/fossism/chaind-cli/internal/db"
	"github.com/fossism/chaind-cli/internal/models"
	"github.com/spf13/cobra"
)

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List your synced tasks in a beautiful table",
	Run: func(cmd *cobra.Command, args []string) {
		database, _ := db.InitDB()

		var tasks []models.Task
		database.Find(&tasks)

		if len(tasks) == 0 {
			fmt.Println("No tasks found. Try running `chaind sync` first!")
			return
		}

		t := table.NewWriter()
		t.SetOutputMirror(os.Stdout)
		t.AppendHeader(table.Row{"ID", "Type", "Repo", "Title", "Status"})

		for _, task := range tasks {
			emoji := "🐛"
			if task.Type == "pr" {
				emoji = "🔗"
			}

			titleTrunc := task.Title
			if len(titleTrunc) > 50 {
				titleTrunc = titleTrunc[:47] + "..."
			}

			t.AppendRow(table.Row{
				task.Number,
				fmt.Sprintf("%s %s", emoji, task.Type),
				task.Repo,
				titleTrunc,
				task.LocalStatus,
			})
		}
		t.Render()
	},
}

func init() {
	rootCmd.AddCommand(lsCmd)
}
