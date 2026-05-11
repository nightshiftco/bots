{{/*
Fully-qualified resource name. The release name typically equals the
bot name (e.g. `bug-bot`), so we just use Release.Name truncated.
*/}}
{{- define "slack-bot.fullname" -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Component name = botName, defaulted to Release.Name. Used as
app.kubernetes.io/component so multiple bots in the same namespace
are individually selectable.
*/}}
{{- define "slack-bot.component" -}}
{{- default .Release.Name .Values.botName }}
{{- end }}

{{/*
Image reference.
*/}}
{{- define "slack-bot.image" -}}
{{- $tag := default .Chart.AppVersion .Values.image.tag -}}
{{- printf "%s/%s:%s" .Values.image.registry .Values.image.repository $tag }}
{{- end }}

{{/*
ServiceAccount name.
*/}}
{{- define "slack-bot.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "slack-bot.fullname" .) .Values.serviceAccount.name }}
{{- else -}}
{{- default "default" .Values.serviceAccount.name }}
{{- end -}}
{{- end }}

{{/*
Default nightshift API URL: in-cluster Service for the parent chart's
default release name `nightshift`, in this release's namespace.
*/}}
{{- define "slack-bot.nightshiftAPIURL" -}}
{{- if .Values.nightshift.apiUrl -}}
{{- .Values.nightshift.apiUrl -}}
{{- else -}}
{{- printf "http://nightshift-nightshift-api.%s.svc:8080" .Release.Namespace -}}
{{- end -}}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "slack-bot.labels" -}}
app.kubernetes.io/name: nightshift-slack-bot
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: {{ include "slack-bot.component" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: nightshift
{{- with .Values.extraLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Selector labels (subset of common labels — must be immutable).
*/}}
{{- define "slack-bot.selectorLabels" -}}
app.kubernetes.io/name: nightshift-slack-bot
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: {{ include "slack-bot.component" . }}
{{- end }}
