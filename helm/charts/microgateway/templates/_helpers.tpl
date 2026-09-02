{{/*
Standard labels. These go on object metadata and the pod template only —
NEVER on selector.matchLabels. A Deployment's selector is immutable, so
changing it breaks `helm upgrade` on every existing install.
*/}}
{{- define "microgateway.labels" -}}
app.kubernetes.io/name: microgateway
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: gateway
app.kubernetes.io/part-of: tyk-ai-studio
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/version: {{ .Values.image.tag | default .Chart.AppVersion | quote }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end -}}

{{/*
The immutable selector. Unchanged from the chart's original single label so
in-place upgrades keep working.
*/}}
{{- define "microgateway.selectorLabels" -}}
app: microgateway
{{- end -}}

{{- define "microgateway.fullname" -}}
{{ .Release.Name }}-microgateway
{{- end -}}

{{/*
Name of the Secret holding EDGE_AUTH_TOKEN / ENCRYPTION_KEY / TYK_AI_LICENSE.
When secrets.existingSecret is set the chart creates nothing and references it
instead, so the values file never has to hold the plaintext.
*/}}
{{- define "microgateway.secretName" -}}
{{- if .Values.secrets.existingSecret -}}
{{ .Values.secrets.existingSecret }}
{{- else -}}
{{ include "microgateway.fullname" . }}-secrets
{{- end -}}
{{- end -}}

{{- define "microgateway.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{ .Values.serviceAccount.name | default (include "microgateway.fullname" .) }}
{{- else -}}
{{ .Values.serviceAccount.name | default "default" }}
{{- end -}}
{{- end -}}
