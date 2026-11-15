{{- define "media-operator.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "media-operator.fullname" -}}
{{- printf "%s" .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "media-operator.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/name: {{ include "media-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "media-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "media-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "media-operator.image" -}}
{{- if .Values.image.tag -}}
{{ .Values.image.repository }}:{{ .Values.image.tag }}
{{- else -}}
{{ .Values.image.repository }}:{{ .Chart.AppVersion }}
{{- end -}}
{{- end }}

{{- define "media-operator.enabledApps" -}}
{{- $known := list "sonarr" "radarr" "lidarr" "readarr" "prowlarr" "flaresolverr" "bazarr" "qbittorrent" "sabnzbd" "jellyfin" "plex" "seerr" "maintainerr" "tdarr" "autobrr" }}
{{- $enabled := list }}
{{- range $app, $on := .Values.apps }}
{{- if not (has $app $known) }}
{{- fail (printf "apps.%s is not an app; the apps are %s" $app (join ", " $known)) }}
{{- end }}
{{- if $on }}
{{- $enabled = append $enabled $app }}
{{- end }}
{{- end }}
{{- if not $enabled }}
{{- fail "apps: turn on at least one app" }}
{{- end }}
{{- join "," (sortAlpha $enabled) }}
{{- end }}
