package ipc

import (
	"context"
	"regexp"
	"strings"

	"github.com/fossism/chaind-cli/internal/schema"
	"github.com/fossism/chaind-cli/internal/store"
)

var (
	rxEmail = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	rxPhone = regexp.MustCompile(`(?:\+91|91)?\s?[6-9]\d{9}`)
	rxPan   = regexp.MustCompile(`[A-Z]{5}[0-9]{4}[A-Z]{1}`)
)

// tokenFromContext resolves the capability token bound by requireToken.
// Handles both the typed key and legacy string keys.
func tokenFromContext(ctx context.Context) *store.Token {
	if tok, ok := ctx.Value(tokenKey).(*store.Token); ok && tok != nil {
		return tok
	}
	if tok, ok := ctx.Value("token").(*store.Token); ok && tok != nil {
		return tok
	}
	if tok, ok := ctx.Value(contextKey("token")).(*store.Token); ok && tok != nil {
		return tok
	}
	return nil
}

// ScrubMessage reads the bound capability token in context and redacts PII if requested.
func ScrubMessage(ctx context.Context, msg *schema.Message) {
	tok := tokenFromContext(ctx)
	if tok == nil || tok.PiiScrub == "" {
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

// containsConfig checks if a comma-separated allowlist has the target value.
func containsConfig(config, target string) bool {
	for _, part := range strings.Split(config, ",") {
		if strings.EqualFold(strings.TrimSpace(part), target) {
			return true
		}
	}
	return false
}

// ScrubJSON redacts PII in an already-marshaled JSON payload using only the
// fixed allowlisted detectors (email/phone/pan). The token field is never
// treated as a regular expression, avoiding ReDoS from token-controlled input.
func ScrubJSON(ctx context.Context, data []byte) []byte {
	tok := tokenFromContext(ctx)
	if tok == nil || tok.PiiScrub == "" || len(data) == 0 {
		return data
	}
	out := data
	if containsConfig(tok.PiiScrub, "email") {
		out = rxEmail.ReplaceAll(out, []byte("[REDACTED_EMAIL]"))
	}
	if containsConfig(tok.PiiScrub, "phone") {
		out = rxPhone.ReplaceAll(out, []byte("[REDACTED_PHONE]"))
	}
	if containsConfig(tok.PiiScrub, "pan") {
		out = rxPan.ReplaceAll(out, []byte("[REDACTED_PAN]"))
	}
	return out
}
