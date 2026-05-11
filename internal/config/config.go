package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	BotName  string
	UserID   string
	LogLevel string

	NightshiftAPIURL string
	AdminToken       string

	SlackBotToken string
	SlackAppToken string

	GitHubPAT    string
	HubSpotToken string

	PersonaPath string
	SkillsDir   string

	RunMaxWall   time.Duration
	PollInterval time.Duration
	PollMax      time.Duration

	HealthAddr string
}

func Load() (*Config, error) {
	c := &Config{
		BotName:          envOr("BOT_NAME", ""),
		UserID:           envOr("USER_ID", ""),
		LogLevel:         envOr("LOG_LEVEL", "info"),
		NightshiftAPIURL: strings.TrimRight(envOr("NS_API_URL", ""), "/"),
		AdminToken:       os.Getenv("NS_ADMIN_TOKEN"),
		SlackBotToken:    os.Getenv("SLACK_BOT_TOKEN"),
		SlackAppToken:    os.Getenv("SLACK_APP_TOKEN"),
		GitHubPAT:        os.Getenv("GITHUB_PAT"),
		HubSpotToken:     os.Getenv("HUBSPOT_TOKEN"),
		PersonaPath:      envOr("PERSONA_PATH", "/etc/persona/system.md"),
		SkillsDir:        envOr("SKILLS_DIR", "/etc/skills"),
		HealthAddr:       envOr("HEALTH_ADDR", ":8081"),
	}

	var err error
	if c.RunMaxWall, err = durOr("RUN_MAX_WALL", 10*time.Minute); err != nil {
		return nil, err
	}
	if c.PollInterval, err = durOr("POLL_INTERVAL", 2*time.Second); err != nil {
		return nil, err
	}
	if c.PollMax, err = durOr("POLL_MAX", 15*time.Second); err != nil {
		return nil, err
	}

	var missing []string
	if c.BotName == "" {
		missing = append(missing, "BOT_NAME")
	}
	if c.UserID == "" {
		missing = append(missing, "USER_ID")
	}
	if c.NightshiftAPIURL == "" {
		missing = append(missing, "NS_API_URL")
	}
	if c.AdminToken == "" {
		missing = append(missing, "NS_ADMIN_TOKEN")
	}
	if c.SlackBotToken == "" {
		missing = append(missing, "SLACK_BOT_TOKEN")
	}
	if c.SlackAppToken == "" {
		missing = append(missing, "SLACK_APP_TOKEN")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required env: %s", strings.Join(missing, ", "))
	}

	if !strings.HasPrefix(c.SlackBotToken, "xoxb-") {
		return nil, errors.New("SLACK_BOT_TOKEN must start with xoxb-")
	}
	if !strings.HasPrefix(c.SlackAppToken, "xapp-") {
		return nil, errors.New("SLACK_APP_TOKEN must start with xapp- (Socket Mode app-level token)")
	}

	return c, nil
}

func envOr(k, def string) string {
	if v, ok := os.LookupEnv(k); ok {
		return v
	}
	return def
}

func durOr(k string, def time.Duration) (time.Duration, error) {
	v, ok := os.LookupEnv(k)
	if !ok || v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("invalid duration for %s: %w", k, err)
	}
	return d, nil
}
