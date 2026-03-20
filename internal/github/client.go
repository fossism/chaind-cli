package github

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/fossism/chaind-cli/internal/config"
	"github.com/fossism/chaind-cli/internal/models"
)

type GitHubSearchResponse struct {
	Items []GitHubItem `json:"items"`
}

type GitHubItem struct {
	ID            int       `json:"id"`
	Number        int       `json:"number"`
	Title         string    `json:"title"`
	Body          string    `json:"body"`
	State         string    `json:"state"`
	HTMLURL       string    `json:"html_url"`
	RepositoryURL string    `json:"repository_url"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	PullRequest   *struct{} `json:"pull_request,omitempty"`
}

func FetchAssignedTasks(cfg *config.Config) ([]models.Task, error) {
	query := fmt.Sprintf("is:open assignee:%s", cfg.Username)
	url := fmt.Sprintf("https://api.github.com/search/issues?q=%s&per_page=50", query)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "token "+cfg.Token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned status %d", resp.StatusCode)
	}

	var searchResp GitHubSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, err
	}

	var tasks []models.Task
	for _, item := range searchResp.Items {
		itemType := "issue"
		if item.PullRequest != nil {
			itemType = "pr"
		}

		repo := strings.Replace(item.RepositoryURL, "https://api.github.com/repos/", "", 1)
		
		bodyTrunc := item.Body
		if len(bodyTrunc) > 200 {
			bodyTrunc = bodyTrunc[:200]
		}

		task := models.Task{
			GithubID:    item.ID,
			Repo:        repo,
			Number:      item.Number,
			Title:       item.Title,
			Body:        bodyTrunc,
			State:       item.State,
			Type:        itemType,
			HTMLURL:     item.HTMLURL,
			CreatedAt:   item.CreatedAt,
			UpdatedAt:   item.UpdatedAt,
			LocalStatus: "todo",
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}
