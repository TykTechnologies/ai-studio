{{/*
Expand the name of the chart.
*/}}
{{- define "midsommar.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "midsommar.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "midsommar.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "midsommar.labels" -}}
helm.sh/chart: {{ include "midsommar.chart" . }}
{{ include "midsommar.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "midsommar.selectorLabels" -}}
app.kubernetes.io/name: {{ include "midsommar.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Several Studio replicas (replicaCount > 1) need a database they can share:
an external PostgreSQL, not the bundled single-pod one and not SQLite, and
the same CSRF_KEY on every replica. Fail the render with the reason.
*/}}
{{- define "midsommar.validateReplicas" -}}
{{- if gt (int .Values.replicaCount) 1 }}
{{- if eq (toString .Values.config.databaseType) "sqlite" }}
{{- fail "replicaCount > 1 needs PostgreSQL: SQLite serves one Studio process. Set config.databaseType=postgres and an external database (database.internal=false), or replicaCount=1." }}
{{- end }}
{{- if .Values.database.internal }}
{{- fail "replicaCount > 1 needs an external PostgreSQL (database.internal=false with database.url, database.host or database.existingSecret): the bundled PostgreSQL is a single pod, so it would be the one thing every replica fails with." }}
{{- end }}
{{- if not .Values.config.csrfKey }}
{{- fail "replicaCount > 1 needs config.csrfKey: every replica must share one CSRF_KEY, or a form loaded from one replica is refused by another. Generate one with: openssl rand -hex 32" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
The Deployment strategy: midsommar.strategy when set; otherwise Recreate for
one replica (two Studios never run side by side during an upgrade) and a
rolling update that keeps every replica serving for several.
*/}}
{{- define "midsommar.strategy" -}}
{{- $s := .Values.midsommar.strategy }}
{{- if not $s }}
{{- if gt (int .Values.replicaCount) 1 }}
{{- $s = dict "type" "RollingUpdate" "rollingUpdate" (dict "maxUnavailable" 0 "maxSurge" 1) }}
{{- else }}
{{- $s = dict "type" "Recreate" }}
{{- end }}
{{- end }}
strategy:
  {{- toYaml $s | nindent 2 }}
{{- end }}

{{/*
terminationGracePeriodSeconds for Studio: shutdown.terminationGracePeriodSeconds
when set, otherwise the preStop delay plus Studio's shutdown (30 s) plus a
5 s margin. A value too short to cover the delay and the shutdown fails the
render: Kubernetes would kill Studio mid-drain.
*/}}
{{- define "midsommar.terminationGracePeriod" -}}
{{- $delay := int .Values.shutdown.preStopDelaySeconds -}}
{{- $need := add $delay 30 -}}
{{- $grace := int (default 0 .Values.shutdown.terminationGracePeriodSeconds) -}}
{{- if eq $grace 0 -}}
{{- $grace = add $need 5 -}}
{{- else if lt $grace $need -}}
{{- fail (printf "shutdown.terminationGracePeriodSeconds (%d) must be at least shutdown.preStopDelaySeconds (%d) plus Studio's 30-second shutdown: %d" $grace $delay $need) -}}
{{- end -}}
{{- $grace -}}
{{- end }}

{{/*
The preStop sleep in seconds, or 0: shutdown.preStopDelaySeconds on a
cluster whose API server is 1.30 or later. The sleep action needs no shell
(the release images are distroless), but older API servers reject it, so
on them no hook is rendered (NOTES.txt says what that costs).
*/}}
{{- define "midsommar.preStopDelay" -}}
{{- if semverCompare ">=1.30-0" .Capabilities.KubeVersion.Version -}}
{{- int .Values.shutdown.preStopDelaySeconds -}}
{{- else -}}
0
{{- end -}}
{{- end }}

{{/*
The bundled edge (microgateway subchart) must reach this release's Studio
and authenticate to it. Checked at render time:
- With no control endpoint set, the subchart's default
  (microgateway.defaultControlEndpoint) must be this release's Studio
  Service on its gRPC port; otherwise (a name override, another port) the
  render fails with the value to set.
- The gRPC Service must target the port Studio listens on
  (config.grpcPort), and an edge must use TLS when Studio serves it.
- When both sides are set in values, the edge's token and key must be
  Studio's: microgateway.secrets.edgeAuthToken = config.grpcAuthToken and
  microgateway.secrets.encryptionKey = config.microgatewayEncryptionKey.
*/}}
{{- define "midsommar.validateEdge" -}}
{{- $mgw := .Values.microgateway | default dict -}}
{{- if $mgw.enabled -}}
{{- $cfg := $mgw.config | default dict -}}
{{- if and (not $cfg.controlEndpoint) (not $cfg.controlEndpoints) -}}
{{- $port := 50051 -}}
{{- range .Values.midsommar.service.ports -}}
{{- if eq .name "grpc" -}}{{- $port = int .port -}}{{- end -}}
{{- end -}}
{{- $want := printf "%s:%d" (include "midsommar.fullname" .) $port -}}
{{- $studio := printf "%s-midsommar" .Release.Name -}}
{{- if contains "midsommar" .Release.Name -}}{{- $studio = .Release.Name -}}{{- end -}}
{{- $default := printf "%s:50051" ($studio | trunc 63 | trimSuffix "-") -}}
{{- if ne $want $default -}}
{{- fail (printf "the bundled edge would connect to %s, but this release's AI Studio gRPC Service is %s: set microgateway.config.controlEndpoint=%s" $default $want $want) -}}
{{- end -}}
{{- end -}}
{{- range .Values.midsommar.service.ports -}}
{{- if and (eq .name "grpc") (ne (toString .targetPort) (toString $.Values.config.grpcPort)) -}}
{{- fail (printf "the grpc Service port targets %v, but AI Studio listens for gRPC on config.grpcPort %v: make them the same" .targetPort $.Values.config.grpcPort) -}}
{{- end -}}
{{- end -}}
{{- if and (eq (toString .Values.config.grpcTlsInsecure) "false") (ne (toString $cfg.tlsEnabled) "true") -}}
{{- fail "config.grpcTlsInsecure is false (AI Studio serves gRPC over TLS), but microgateway.config.tlsEnabled is not true: the bundled edge would connect without TLS" -}}
{{- end -}}
{{- $sec := $mgw.secrets | default dict -}}
{{- if not $sec.existingSecret -}}
{{- if and $sec.edgeAuthToken .Values.config.grpcAuthToken (ne $sec.edgeAuthToken .Values.config.grpcAuthToken) -}}
{{- fail "microgateway.secrets.edgeAuthToken must equal config.grpcAuthToken: the bundled edge authenticates to AI Studio with it" -}}
{{- end -}}
{{- if and $sec.encryptionKey .Values.config.microgatewayEncryptionKey (ne $sec.encryptionKey .Values.config.microgatewayEncryptionKey) -}}
{{- fail "microgateway.secrets.encryptionKey must equal config.microgatewayEncryptionKey: the bundled edge decrypts what AI Studio sends with it" -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end }}
