# nightshift-slack-bot

Generic Slack bot that bridges `@app_mention` events into nightshift
runs. One image, parameterized per deployment via helm values:
bot name, Slack credentials, nightshift endpoint, persona, skills, and
connectors.

## First deployment

`bug-bot` against `sandbox-prod` — investigates issues against the
`nightshiftco/nightshift` codebase via the GitHub MCP. See
[`deploy/charts/nightshift-slack-bot/README.md`](deploy/charts/nightshift-slack-bot/README.md)
for the operator runbook.

## Build

```
make build      # binary at bin/nightshift-slack-bot
make vet test
make chart-lint chart-template
```

## How it works

1. On startup the bot **seeds** its dedicated nightshift user:
   - `CreateConnector(github, STATIC_TOKEN, …)`
   - `SetConnectorStaticToken(user_id, github, $GITHUB_PAT)`
   - `CreateSkill` for each `*.md` under `/etc/skills/`
   All calls treat `AlreadyExists` as success — re-running is safe.
2. Connects to Slack via **Socket Mode** (dial-out only, no ingress).
3. On `app_mention`:
   - Adds an `:eyes:` reaction
   - Calls `POST /v1/runs` with `prompt`, `user_id`, `invoker_id =
     <slack user>`, `idempotency_key = slack:<event_id>`, and the
     thread's `session_id` if one was already assigned
   - Polls `GET /v1/runs/{id}` until terminal
   - Fetches the last event via
     `GET /v1/runs/{id}/events?page_size=1&order_by=index desc`
   - Posts the assistant's text back as a threaded reply

Session continuity: the bot maintains an in-memory
`thread_ts → session_id` map so follow-up mentions in the same
thread continue the conversation. MVP: lost on pod restart.

## Auth model

The bot uses **one admin static token** for both the seed and runtime
CreateRun calls. The token's NAME (e.g. `slack-bot-bug-bot`) must be
listed in `NS_AUTH_ADMIN_TOKENS` on the nightshift API; the VALUE is
stored at `secret/nightshift/static-tokens/<name>` in OpenBao.
