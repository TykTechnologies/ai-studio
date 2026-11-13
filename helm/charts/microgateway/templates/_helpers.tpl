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

{{/*
"true" when the edge may start from its last-known-good configuration
(config.startFromLastKnownGood, a boolean or the string "true"); empty
otherwise.
*/}}
{{- define "microgateway.startFromLastKnownGood" -}}
{{- if eq (toString .Values.config.startFromLastKnownGood) "true" -}}true{{- end -}}
{{- end }}

{{/*
config.credentialSync, checked: "on_demand" or "full" ("" when unset). Any
other value fails the render, as the edge would refuse it at start.
*/}}
{{- define "microgateway.credentialSync" -}}
{{- $mode := toString (.Values.config.credentialSync | default "") -}}
{{- if not (has $mode (list "" "on_demand" "full")) -}}
{{- fail (printf "config.credentialSync must be on_demand or full, not %q" $mode) -}}
{{- end -}}
{{- $mode -}}
{{- end }}

{{/*
"true" when the edge's database is on a persistent volume: the StatefulSet
form with persistence. A Deployment's emptyDir survives a container restart
but not a rescheduled pod.
*/}}
{{- define "microgateway.persistentDatabase" -}}
{{- if and .Values.persistence.enabled (eq (.Values.kind | default "Deployment") "StatefulSet") -}}true{{- end -}}
{{- end }}

{{/*
"true" when a setting is on that relies on the edge's database surviving a
restart (starting from last-known-good, full credential sync) while the
database is not on a persistent volume.
*/}}
{{- define "microgateway.needsPersistenceWarning" -}}
{{- $needs := or (include "microgateway.startFromLastKnownGood" .) (eq (include "microgateway.credentialSync" .) "full") -}}
{{- if and $needs (not (include "microgateway.persistentDatabase" .)) -}}true{{- end -}}
{{- end }}

{{/*
What the chart prints after an install or upgrade (templates/NOTES.txt):
the persistence warning, or nothing.
*/}}
{{- define "microgateway.notes" -}}
{{- if and .Values.enabled (include "microgateway.needsPersistenceWarning" .) }}
WARNING: this edge gateway keeps its database on an emptyDir volume.

{{- if include "microgateway.startFromLastKnownGood" . }}
  config.startFromLastKnownGood is on: the edge starts from the configuration
  it stored in its database when no control plane answers.
{{- end }}
{{- if eq (include "microgateway.credentialSync" .) "full" }}
  config.credentialSync is "full": the edge validates keys from the hashed
  credential list it stored in its database.
{{- end }}

  An emptyDir survives a container restart, so that case is covered. It does
  not survive a rescheduled pod (a node drain, an eviction, a rollout): such a
  pod starts with an empty database and needs a control plane, exactly as
  without the setting.

  To cover rescheduled pods too, give each replica a persistent volume:

    kind: StatefulSet
    persistence:
      enabled: true
{{- end }}
{{- end }}

{{/*
The control endpoint an edge of this chart uses when none is set: the AI
Studio Service of the parent chart (helm/), named as that chart names it
("<release>-midsommar", or the release name when it contains "midsommar"),
on its gRPC port, 50051. The parent chart checks at render time that this
matches the Service it renders (midsommar.validateEdge), and refuses with
the value to set when it does not (a name override, another gRPC port).
Installed on its own, this chart needs config.controlEndpoint(s).
*/}}
{{- define "microgateway.defaultControlEndpoint" -}}
{{- $studio := printf "%s-midsommar" .Release.Name -}}
{{- if contains "midsommar" .Release.Name -}}
{{- $studio = .Release.Name -}}
{{- end -}}
{{- printf "%s:50051" ($studio | trunc 63 | trimSuffix "-") -}}
{{- end }}
