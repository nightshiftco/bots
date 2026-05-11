package slackbot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	nsv1 "github.com/nightshiftco/nightshift/gen/go/nightshift/v1"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/nightshiftco/bots/internal/nightshift"
)

type Config struct {
	BotUserID    string
	UserID       string // the nightshift user_id this bot owns
	Persona      string // optional system-prompt prefix prepended to every CreateRun prompt
	RunMaxWall   time.Duration
	PollInterval time.Duration
	PollMax      time.Duration
}

type Bot struct {
	cfg      Config
	web      *slack.Client
	sock     *socketmode.Client
	ns       *nightshift.Client
	sessions *Sessions
	log      *slog.Logger
}

func New(web *slack.Client, sock *socketmode.Client, ns *nightshift.Client, cfg Config, log *slog.Logger) *Bot {
	return &Bot{
		cfg:      cfg,
		web:      web,
		sock:     sock,
		ns:       ns,
		sessions: NewSessions(),
		log:      log,
	}
}

// Run blocks on the socketmode event loop until ctx is cancelled.
// Returns ctx.Err() on shutdown.
func (b *Bot) Run(ctx context.Context) error {
	go b.consumeEvents(ctx)
	// socketmode.RunContext blocks until the context is cancelled or
	// an unrecoverable error occurs.
	return b.sock.RunContext(ctx)
}

func (b *Bot) consumeEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case env, ok := <-b.sock.Events:
			if !ok {
				return
			}
			b.handleEnvelope(ctx, env)
		}
	}
}

func (b *Bot) handleEnvelope(ctx context.Context, env socketmode.Event) {
	switch env.Type {
	case socketmode.EventTypeConnecting:
		b.log.Info("slack socketmode: connecting")
	case socketmode.EventTypeConnected:
		b.log.Info("slack socketmode: connected", "bot_user_id", b.cfg.BotUserID)
	case socketmode.EventTypeDisconnect:
		b.log.Warn("slack socketmode: disconnected")
	case socketmode.EventTypeEventsAPI:
		// Ack immediately; do the work asynchronously so the events
		// channel stays unblocked. EnvelopeID is unique per Slack
		// delivery (including retries), so it's the right idempotency
		// anchor.
		var envelopeID string
		if env.Request != nil {
			envelopeID = env.Request.EnvelopeID
			b.sock.Ack(*env.Request)
		}
		payload, ok := env.Data.(slackevents.EventsAPIEvent)
		if !ok {
			return
		}
		if payload.Type != slackevents.CallbackEvent {
			return
		}
		ev, ok := payload.InnerEvent.Data.(*slackevents.AppMentionEvent)
		if !ok {
			return
		}
		go b.handleAppMention(ctx, ev, envelopeID)
	}
}

func (b *Bot) handleAppMention(parent context.Context, ev *slackevents.AppMentionEvent, envelopeID string) {
	// React with eyes so the user sees we received the mention. Ignore
	// the error — at worst the user just doesn't see the eyes.
	msgRef := slack.NewRefToMessage(ev.Channel, ev.TimeStamp)
	_ = b.web.AddReactionContext(parent, "eyes", msgRef)
	defer func() { _ = b.web.RemoveReactionContext(parent, "eyes", msgRef) }()

	userText := StripMention(ev.Text, b.cfg.BotUserID)
	if strings.TrimSpace(userText) == "" {
		return
	}
	// Prepend the operator-configured persona as a per-run system prefix.
	// nightshift's CreateRunRequest has no top-level system_prompt field,
	// so this is the canonical way to inject deployment-level framing.
	prompt := userText
	if b.cfg.Persona != "" {
		prompt = b.cfg.Persona + "\n\n---\n\nUser request:\n" + userText
	}

	threadTS := ev.ThreadTimeStamp
	if threadTS == "" {
		threadTS = ev.TimeStamp
	}
	sessionID, _ := b.sessions.Get(threadTS)

	ctx, cancel := context.WithTimeout(parent, b.cfg.RunMaxWall)
	defer cancel()

	idemKey := "slack:" + envelopeID
	if envelopeID == "" {
		// Fallback when the envelope id is unavailable; channel+ts is
		// unique per posted message.
		idemKey = "slack:" + ev.Channel + ":" + ev.TimeStamp
	}

	b.log.Info("creating run", "user", ev.User, "channel", ev.Channel, "thread_ts", threadTS, "session_id", sessionID)

	run, err := b.ns.CreateRun(ctx, &nsv1.CreateRunRequest{
		Prompt:         prompt,
		SessionId:      sessionID,
		UserId:         b.cfg.UserID,
		InvokerType:    nsv1.InvokerType_INVOKER_TYPE_USER,
		InvokerId:      ev.User,
		IdempotencyKey: idemKey,
	})
	if err != nil {
		b.log.Error("create run", "err", err)
		b.postError(ctx, ev.Channel, threadTS, fmt.Sprintf("couldn't start run: %v", err))
		return
	}
	runID := run.GetId()
	if sessionID == "" && run.GetSessionId() != "" {
		b.sessions.Put(threadTS, run.GetSessionId())
	}

	status, err := b.ns.WaitForTerminal(ctx, runID, b.cfg.PollInterval, b.cfg.PollMax)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			b.postError(ctx, ev.Channel, threadTS,
				fmt.Sprintf(":warning: run `%s` did not terminate within %s", runID, b.cfg.RunMaxWall))
			return
		}
		b.log.Error("wait terminal", "run_id", runID, "err", err)
		b.postError(ctx, ev.Channel, threadTS, fmt.Sprintf("failed to wait for run `%s`: %v", runID, err))
		return
	}

	// Re-fetch the run so we have the final event_count for paging the
	// events list. WaitForTerminal returned the terminal status from
	// its last poll; that response also had event_count, but we don't
	// keep it — one extra GetRun isn't worth the plumbing.
	terminalRun, err := b.ns.GetRun(ctx, runID)
	if err != nil {
		b.log.Error("getrun terminal", "run_id", runID, "err", err)
		b.postError(ctx, ev.Channel, threadTS, fmt.Sprintf(":warning: run `%s` terminal=%s but couldn't refetch: %v", runID, status, err))
		return
	}

	final, err := b.ns.LastEvent(ctx, runID, terminalRun.GetEventCount())
	if err != nil {
		b.log.Error("fetch last event", "run_id", runID, "err", err)
		b.postError(ctx, ev.Channel, threadTS,
			fmt.Sprintf(":warning: run `%s` terminal=%s but couldn't fetch final event: %v", runID, status, err))
		return
	}

	text := renderFinal(runID, status, final)
	for _, chunk := range ChunkForSlack(text) {
		if _, _, err := b.web.PostMessageContext(ctx, ev.Channel,
			slack.MsgOptionText(chunk, false),
			slack.MsgOptionTS(threadTS),
		); err != nil {
			b.log.Error("post message", "err", err)
		}
	}
}

func renderFinal(runID string, status nsv1.RunStatus, final *nightshift.FinalEvent) string {
	switch {
	case final.Type == "result.success":
		if s, ok := final.Raw["result"].(string); ok && s != "" {
			return s
		}
		return fmt.Sprintf(":white_check_mark: run `%s` completed but emitted no result text", runID)
	case strings.HasPrefix(final.Type, "result."):
		body, _ := final.Raw["result"].(string)
		return fmt.Sprintf(":warning: run `%s` ended `%s`\n```\n%s\n```", runID, status, body)
	default:
		return fmt.Sprintf(":warning: run `%s` terminal=%s, last event type=`%s` (no result text)", runID, status, final.Type)
	}
}

func (b *Bot) postError(ctx context.Context, channel, threadTS, msg string) {
	if _, _, err := b.web.PostMessageContext(ctx, channel,
		slack.MsgOptionText(msg, false),
		slack.MsgOptionTS(threadTS),
	); err != nil {
		b.log.Error("post error message", "err", err)
	}
}
