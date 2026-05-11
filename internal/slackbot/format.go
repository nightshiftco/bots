package slackbot

import (
	"regexp"
	"strings"
)

// slackMessageLimit is the per-message character cap before Slack
// truncates. Leave a small safety margin.
const slackMessageLimit = 3900

var mentionRE = regexp.MustCompile(`<@[A-Z0-9]+>`)

// StripMention removes a leading <@BOTID> mention (with optional
// trailing whitespace) from the event text. Mid-message mentions
// are left intact — they may be intentional references to other
// users.
func StripMention(text, botUserID string) string {
	prefix := "<@" + botUserID + ">"
	if strings.HasPrefix(text, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(text, prefix))
	}
	// Some clients prepend whitespace before the mention.
	trimmed := strings.TrimLeft(text, " \t")
	if strings.HasPrefix(trimmed, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
	}
	// Fall back: strip the first mention regardless of which user.
	if loc := mentionRE.FindStringIndex(text); loc != nil && loc[0] == 0 {
		return strings.TrimSpace(text[loc[1]:])
	}
	return text
}

// ChunkForSlack splits a message into pieces that each fit under
// Slack's per-message limit. Splits on paragraph boundaries when
// possible to keep code blocks intact.
func ChunkForSlack(s string) []string {
	if len(s) <= slackMessageLimit {
		return []string{s}
	}
	var chunks []string
	remaining := s
	for len(remaining) > slackMessageLimit {
		cut := slackMessageLimit
		if nl := strings.LastIndex(remaining[:cut], "\n\n"); nl > slackMessageLimit/2 {
			cut = nl + 2
		} else if nl := strings.LastIndex(remaining[:cut], "\n"); nl > slackMessageLimit/2 {
			cut = nl + 1
		}
		chunks = append(chunks, remaining[:cut])
		remaining = remaining[cut:]
	}
	if remaining != "" {
		chunks = append(chunks, remaining)
	}
	return chunks
}
