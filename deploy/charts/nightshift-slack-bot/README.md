# nightshift-slack-bot chart

Deploys one Slack bot bridged into nightshift. Each Helm release is one
named bot (e.g. `bug-bot`).

## Pre-flight (out-of-band)

1. **Slack app** — Socket Mode ON, `app_mention` event subscription, bot
   scopes `app_mentions:read`, `chat:write`, `reactions:write`. Capture:
   - `SLACK_BOT_TOKEN` (xoxb-…) from OAuth & Permissions
   - `SLACK_APP_TOKEN` (xapp-…) generated under Basic Information with
     scope `connections:write`
2. **GitHub PAT** — fine-grained, scoped to `nightshiftco/nightshift`,
   contents=read + metadata=read.
3. **Nightshift admin token** —
   ```
   BOT_USER_ID=$(uuidgen)
   ADMIN_TOKEN=$(openssl rand -hex 32)
   bao kv patch secret/nightshift/static-tokens slack-bot-bug-bot="$ADMIN_TOKEN"
   ```
   Add `slack-bot-bug-bot` to `NS_AUTH_ADMIN_TOKENS` on the API
   (chart value `nightshift_api.auth.adminTokens`); restart the API.
4. **K8s Secrets** in the target namespace. The slack + nightshift
   Secrets are required; connector Secrets (github, hubspot) are
   optional — create only the ones you want this bot to seed, and
   reference them via the matching `secrets.*` value.
   ```
   kubectl -n <ns> create secret generic nightshift-slack-bot-slack \
     --from-literal=SLACK_BOT_TOKEN=xoxb-… \
     --from-literal=SLACK_APP_TOKEN=xapp-…
   kubectl -n <ns> create secret generic nightshift-slack-bot-api \
     --from-literal=NS_ADMIN_TOKEN="$ADMIN_TOKEN"
   # Optional: GitHub connector
   kubectl -n <ns> create secret generic nightshift-slack-bot-github \
     --from-literal=GITHUB_PAT=ghp_…
   # Optional: HubSpot connector (Private App access token)
   kubectl -n <ns> create secret generic nightshift-slack-bot-hubspot \
     --from-literal=HUBSPOT_TOKEN=pat-…
   ```

## Install

```
helm install bug-bot deploy/charts/nightshift-slack-bot \
  --namespace sandbox-prod \
  --set botName=bug-bot \
  --set userId="$BOT_USER_ID" \
  --set-file persona.systemPrompt=./persona.md
```

Argo CD users: see `customers/argocd/applications/sandbox-slack-bot-bug-bot.yaml`
(mirror the shape of `sandbox.yaml`).

## Values

See [`values.yaml`](values.yaml) for the full schema and defaults.
