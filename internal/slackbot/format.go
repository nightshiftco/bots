package slackbot

import (
	"regexp"
	"strings"
)

// SlackifyMarkdown converts standard CommonMark-ish markdown (what the
// agent typically emits) to Slack mrkdwn. Best-effort: handles bold,
// links, headers, and `-`/`*` bullets. Inline code (backticks) and
// fenced code blocks (```) are passed through verbatim since Slack
// renders those natively.
//
// Single-asterisk italic (`*x*`) is intentionally NOT converted —
// disambiguating it from bold and bullet leaders is brittle. After the
// `**...**` → `*...*` pass, anything left in single-`*` form renders
// as bold in Slack, which is a tolerable degradation.
func SlackifyMarkdown(s string) string {
	var b strings.Builder
	lines := strings.Split(s, "\n")
	inFence := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			b.WriteString(line)
			b.WriteByte('\n')
			continue
		}
		if inFence {
			b.WriteString(line)
			b.WriteByte('\n')
			continue
		}
		b.WriteString(slackifyLine(line))
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

var (
	headerRE    = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*#*\s*$`)
	bulletRE    = regexp.MustCompile(`^(\s*)[-*]\s+(.+)$`)
	mdBoldRE    = regexp.MustCompile(`\*\*([^*\n]+?)\*\*`)
	mdUnderRE   = regexp.MustCompile(`__([^_\n]+?)__`)
	mdLinkRE    = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)
)

func slackifyLine(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	indent := line[:len(line)-len(trimmed)]

	if m := headerRE.FindStringSubmatch(trimmed); m != nil {
		return indent + "*" + slackifyInline(m[2]) + "*"
	}
	if m := bulletRE.FindStringSubmatch(line); m != nil {
		return m[1] + "• " + slackifyInline(m[2])
	}
	return slackifyInline(line)
}

// slackifyInline applies transformations to a line, skipping content
// inside inline-code backticks so we don't mangle code samples.
func slackifyInline(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '`' {
			j := strings.Index(s[i+1:], "`")
			if j < 0 {
				// Unmatched backtick — write the rest literally.
				b.WriteString(s[i:])
				return b.String()
			}
			b.WriteString(s[i : i+j+2])
			i = i + j + 2
			continue
		}
		end := len(s)
		if nbt := strings.Index(s[i:], "`"); nbt >= 0 {
			end = i + nbt
		}
		b.WriteString(transformSegment(s[i:end]))
		i = end
	}
	return b.String()
}

func transformSegment(s string) string {
	s = mdBoldRE.ReplaceAllString(s, "*$1*")
	s = mdUnderRE.ReplaceAllString(s, "*$1*")
	s = mdLinkRE.ReplaceAllString(s, "<$2|$1>")
	return s
}

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
