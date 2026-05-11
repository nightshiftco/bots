// Package seed performs the one-shot bootstrap of the bot's dedicated
// nightshift user: register the GitHub connector, set the bot's PAT
// as the static token, and create one Skill per *.md file in the
// configured skills directory. All steps treat AlreadyExists as
// success, so re-running on pod restart is safe.
package seed

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	nsv1 "github.com/nightshiftco/nightshift/gen/go/nightshift/v1"

	"github.com/nightshiftco/bots/internal/nightshift"
)

type Config struct {
	UserID    string
	GitHubPAT string
	SkillsDir string
}

// Connector describes a connector to register at seed time. MVP only
// supports the static-token shape; OAuth connectors would require an
// admin-side OpenBao client_id/secret pair to be in place out-of-band.
type Connector struct {
	Name            string
	Description     string
	McpURL          string
	McpAllowedTools []string
	// TokenSource is a function the seeder calls to obtain the PAT.
	// Closure over cfg.GitHubPAT keeps secrets out of the connector
	// catalog struct itself.
	TokenSource func() string
}

// Run performs the seed. Returns an error only if a step fails with
// something other than AlreadyExists. Calling Run again is safe.
func Run(ctx context.Context, c *nightshift.Client, cfg Config, connectors []Connector, log *slog.Logger) error {
	for _, conn := range connectors {
		if err := seedConnector(ctx, c, cfg, conn, log); err != nil {
			return err
		}
	}
	return seedSkills(ctx, c, cfg, log)
}

func seedConnector(ctx context.Context, c *nightshift.Client, cfg Config, conn Connector, log *slog.Logger) error {
	_, err := c.CreateConnector(ctx, &nsv1.CreateConnectorRequest{
		Name:            conn.Name,
		Description:     conn.Description,
		AuthType:        nsv1.ConnectorAuthType_CONNECTOR_AUTH_TYPE_STATIC_TOKEN,
		McpUrl:          conn.McpURL,
		McpAllowedTools: conn.McpAllowedTools,
	})
	switch {
	case err == nil:
		log.Info("seeded connector", "name", conn.Name, "state", "created")
	case nightshift.IsAlreadyExists(err):
		log.Info("seeded connector", "name", conn.Name, "state", "already-exists")
	default:
		return fmt.Errorf("create connector %q: %w", conn.Name, err)
	}

	token := ""
	if conn.TokenSource != nil {
		token = conn.TokenSource()
	}
	if token == "" {
		return fmt.Errorf("connector %q: empty static token", conn.Name)
	}

	if err := c.SetConnectorStaticToken(ctx, &nsv1.SetConnectorStaticTokenRequest{
		UserId:        cfg.UserID,
		ConnectorName: conn.Name,
		Token:         token,
	}); err != nil {
		return fmt.Errorf("set static token for %q: %w", conn.Name, err)
	}
	log.Info("seeded connector static token", "name", conn.Name, "user_id", cfg.UserID)
	return nil
}

// Skill `name` is the filename stem and must match this regex (enforced
// server-side at internal/api/config/skills.go).
var skillNameRE = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)

func seedSkills(ctx context.Context, c *nightshift.Client, cfg Config, log *slog.Logger) error {
	entries, err := os.ReadDir(cfg.SkillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			log.Info("no skills dir; skipping", "dir", cfg.SkillsDir)
			return nil
		}
		return fmt.Errorf("read skills dir: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		if !skillNameRE.MatchString(name) {
			log.Warn("skipping skill: name does not match required regex",
				"file", e.Name(), "regex", skillNameRE.String())
			continue
		}
		content, err := os.ReadFile(filepath.Join(cfg.SkillsDir, e.Name()))
		if err != nil {
			return fmt.Errorf("read skill %q: %w", e.Name(), err)
		}
		desc := firstFrontmatterDescription(string(content))

		_, err = c.CreateSkill(ctx, &nsv1.CreateSkillRequest{
			UserId:      cfg.UserID,
			Name:        name,
			Description: desc,
			Content:     string(content),
		})
		switch {
		case err == nil:
			log.Info("seeded skill", "name", name, "state", "created")
		case nightshift.IsAlreadyExists(err):
			log.Info("seeded skill", "name", name, "state", "already-exists")
		default:
			return fmt.Errorf("create skill %q: %w", name, err)
		}
	}
	return nil
}

// firstFrontmatterDescription pulls a `description:` line out of YAML
// frontmatter at the top of the file. Returns an empty string if the
// file has no frontmatter or no description key — the API treats
// description as optional.
func firstFrontmatterDescription(s string) string {
	if !strings.HasPrefix(s, "---") {
		return ""
	}
	body := strings.TrimPrefix(s, "---\n")
	end := strings.Index(body, "\n---")
	if end < 0 {
		return ""
	}
	for _, line := range strings.Split(body[:end], "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "description:"); ok {
			return strings.TrimSpace(strings.Trim(rest, `"' `))
		}
	}
	return ""
}
