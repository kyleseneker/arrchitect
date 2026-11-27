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
{{- $enabled := list }}
{{- range $app, $on := .Values.apps }}
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
{{- $pvr := list "sonarr" "radarr" "lidarr" "readarr" }}
{{- $plurals := dict "plex" "plexes" }}
{{- $resources := list }}
{{- range $app := splitList "," (include "arrchitect.enabledApps" .) }}
{{- $resources = append $resources (get $plurals $app | default (printf "%ss" $app)) }}
{{- if has $app $pvr }}
{{- $resources = concat $resources (list (printf "%sindexers" $app) (printf "%sdownloadclients" $app)) }}
{{- end }}
{{- end }}
{{- join "," $resources }}
{{- end }}

{{- define "arrchitect.managerRules" -}}
{{- $resources := splitList "," (include "arrchitect.resources" .) }}
- apiGroups:
    - arrchitect.dev
  resources:
    {{- range $resources }}
    - {{ . }}
    {{- end }}
  verbs:
    - get
    - list
    - watch
    - update
    - patch
- apiGroups:
    - arrchitect.dev
  resources:
    {{- range $resources }}
    - {{ . }}/status
    {{- end }}
  verbs:
    - get
    - update
    - patch
- apiGroups:
    - arrchitect.dev
  resources:
    {{- range $resources }}
    - {{ . }}/finalizers
    {{- end }}
  verbs:
    - update
- apiGroups:
    - ""
  resources:
    - events
  verbs:
    - create
    - patch
- apiGroups:
    - events.k8s.io
  resources:
    - events
  verbs:
    - create
    - patch
- apiGroups:
    - ""
  resources:
    - secrets
  verbs:
    - get
    - list
    - watch
{{- end }}

{{- define "arrchitect.watchNamespaces" -}}
{{- $namespaces := list }}
{{- range splitList "," .Values.watchNamespace }}
{{- with trim . }}
{{- $namespaces = append $namespaces . }}
{{- end }}
{{- end }}
{{- if and .Values.watchNamespace (not $namespaces) }}
{{- fail "watchNamespace must list at least one namespace" }}
{{- end }}
{{- join "," (uniq $namespaces) }}
{{- end }}
