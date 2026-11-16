{{- define "arrchitect.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "arrchitect.fullname" -}}
{{- printf "%s" .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "arrchitect.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/name: {{ include "arrchitect.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "arrchitect.selectorLabels" -}}
app.kubernetes.io/name: {{ include "arrchitect.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "arrchitect.image" -}}
{{- if .Values.image.tag -}}
{{ .Values.image.repository }}:{{ .Values.image.tag }}
{{- else -}}
{{ .Values.image.repository }}:{{ .Chart.AppVersion }}
{{- end -}}
{{- end }}

{{- define "arrchitect.enabledApps" -}}
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

{{- define "arrchitect.resources" -}}
{{- $kinds := dict "sonarr" (list "sonarrs" "sonarrindexers" "sonarrdownloadclients") }}
{{- $resources := list }}
{{- range $app := splitList "," (include "arrchitect.enabledApps" .) }}
{{- $resources = concat $resources (get $kinds $app | default (list (printf "%sconfigs" $app))) }}
{{- end }}
{{- join "," $resources }}
{{- end }}
