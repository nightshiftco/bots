package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"

	"github.com/nightshiftco/bots/internal/config"
	nsclient "github.com/nightshiftco/bots/internal/nightshift"
	"github.com/nightshiftco/bots/internal/seed"
	"github.com/nightshiftco/bots/internal/slackbot"
	"github.com/nightshiftco/bots/internal/version"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	log.Info("starting", "bot", cfg.BotName, "version", version.Version, "user_id", cfg.UserID)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Health server. /healthz reports liveness; /readyz flips to OK once
	// seed completes and slack is connected.
	var ready atomic.Bool
	healthSrv := startHealthServer(cfg.HealthAddr, &ready, log)
	defer func() {
		shutdownCtx, c := context.WithTimeout(context.Background(), 5_000_000_000)
		defer c()
		_ = healthSrv.Shutdown(shutdownCtx)
	}()

	ns := nsclient.New(cfg.NightshiftAPIURL, cfg.AdminToken)

	// Seed once. AlreadyExists is treated as success inside seed.Run.
	seedCfg := seed.Config{
		UserID:    cfg.UserID,
		GitHubPAT: cfg.GitHubPAT,
		SkillsDir: cfg.SkillsDir,
	}
	var connectors []seed.Connector
	if cfg.GitHubPAT != "" {
		connectors = append(connectors, seed.Connector{
			Name:            "github",
			Description:     "GitHub — repositories, issues, pull requests",
			McpURL:          "https://api.githubcopilot.com/mcp",
			McpAllowedTools: []string{"mcp__github__*"},
			TokenSource:     func() string { return cfg.GitHubPAT },
		})
	}
	if cfg.HubSpotToken != "" {
		// `hubspot-pat` rather than `hubspot` because the platform
		// connector catalog already seeds `hubspot` as OAuth — names
		// are unique platform-wide and CreateConnector with the same
		// name + a different auth_type fails. Tool prefix follows the
		// connector name, so allowed tools live under mcp__hubspot-pat__*.
		connectors = append(connectors, seed.Connector{
			Name:            "hubspot-pat",
			Description:     "HubSpot (Private App token) — CRM contacts, companies, deals, tickets",
			McpURL:          "https://mcp.hubspot.com",
			McpAllowedTools: []string{"mcp__hubspot-pat__*"},
			TokenSource:     func() string { return cfg.HubSpotToken },
		})
	}
	if len(connectors) == 0 {
		log.Warn("no connectors configured; set GITHUB_PAT and/or HUBSPOT_TOKEN to seed connectors")
	}
	if err := seed.Run(ctx, ns, seedCfg, connectors, log); err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	log.Info("seed complete")

	// Slack clients. The Web API client uses the bot token; socket mode
	// adds the app-level token for the dial-out connection.
	web := slack.New(cfg.SlackBotToken, slack.OptionAppLevelToken(cfg.SlackAppToken))
	authResp, err := web.AuthTestContext(ctx)
	if err != nil {
		return fmt.Errorf("slack auth.test: %w", err)
	}
	log.Info("slack auth ok", "bot_user_id", authResp.UserID, "team", authResp.Team)

	persona := loadPersona(cfg.PersonaPath, log)

	sock := socketmode.New(web)

	bot := slackbot.New(web, sock, ns, slackbot.Config{
		BotUserID:    authResp.UserID,
		UserID:       cfg.UserID,
		Persona:      persona,
		RunMaxWall:   cfg.RunMaxWall,
		PollInterval: cfg.PollInterval,
		PollMax:      cfg.PollMax,
	}, log)

	ready.Store(true)
	if err := bot.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("slack bot: %w", err)
	}
	return nil
}

// loadPersona reads the per-deployment system-prompt prefix written by
// the chart's persona ConfigMap. Missing/empty file → no prefix. The
// content is prepended verbatim to every CreateRun prompt by the
// handler.
func loadPersona(path string, log *slog.Logger) string {
	if path == "" {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Warn("persona: read failed; running without a persona prefix", "path", path, "err", err)
		}
		return ""
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		log.Info("persona: file present but empty; running without a persona prefix", "path", path)
		return ""
	}
	log.Info("persona: loaded", "path", path, "chars", len(s))
	return s
}

func startHealthServer(addr string, ready *atomic.Bool, log *slog.Logger) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !ready.Load() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ready"))
	})
	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("health server", "err", err)
		}
	}()
	return srv
}
