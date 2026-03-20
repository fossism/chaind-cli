package models

import "time"

type Task struct {
	ID          uint      `gorm:"primaryKey"`
	GithubID    int       `gorm:"uniqueIndex"`
	Repo        string
	Number      int
	Title       string
	Body        string
	State       string
	Type        string
	HTMLURL     string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	LocalStatus string `gorm:"default:'todo'"` // todo, in-progress, done
}
