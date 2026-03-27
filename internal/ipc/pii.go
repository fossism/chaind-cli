package ipc

import (
	"context"
	"regexp"

	"github.com/fossism/chaind-cli/internal/schema"
	"github.com/fossism/chaind-cli/internal/store"
)

var (
	rxEmail = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	rxPhone = regexp.MustCompile(`(?:\+91|91)?\s?[6-9]\d{9}`)
	rxPan   = regexp.MustCompile(`[A-Z]{5}[0-9]{4}[A-Z]{1}`)
)

// ScrubMessage reads the bound capability token in context and redacts PII if requested.
func ScrubMessage(ctx context.Context, msg *schema.Message) {
	tok, ok := ctx.Value("token").(*store.Token)
	if !ok || tok == nil || tok.PiiScrub == "" {
		return
	}

	content := msg.Content.Text
	if content == "" {
		return
	}

	// Basic best-effort scrubbing
	if containsConfig(tok.PiiScrub, "email") {
		content = rxEmail.ReplaceAllString(content, "[REDACTED_EMAIL]")
	}
	if containsConfig(tok.PiiScrub, "phone") {
		content = rxPhone.ReplaceAllString(content, "[REDACTED_PHONE]")
	}
	if containsConfig(tok.PiiScrub, "pan") {
		content = rxPan.ReplaceAllString(content, "[REDACTED_PAN]")
	}

	msg.Content.Text = content
}

// containsConfig checks if a comma-separated list has the target value
func containsConfig(config, target string) bool {
	return regexp.MustCompile(`\b` + target + `\b`).MatchString(config)
}
