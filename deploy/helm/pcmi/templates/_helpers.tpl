{{/*
Common labels & names. Helm-best-practice helpers used by all templates.
*/}}

{{- define "pcmi.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "pcmi.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "pcmi.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "pcmi.labels" -}}
helm.sh/chart: {{ include "pcmi.chart" . }}
app.kubernetes.io/name: {{ include "pcmi.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "pcmi.api.selectorLabels" -}}
app.kubernetes.io/name: {{ include "pcmi.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: api
{{- end -}}

{{- define "pcmi.worker.selectorLabels" -}}
app.kubernetes.io/name: {{ include "pcmi.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: worker
{{- end -}}

{{/*
Resolve image tag — empty values.yaml override defers to Chart.AppVersion so
the chart never ships a `:latest` image by accident.
*/}}
{{- define "pcmi.api.image" -}}
{{- $tag := default .Chart.AppVersion .Values.image.api.tag -}}
{{- printf "%s:%s" .Values.image.api.repository $tag -}}
{{- end -}}

{{- define "pcmi.worker.image" -}}
{{- $tag := default .Chart.AppVersion .Values.image.worker.tag -}}
{{- printf "%s:%s" .Values.image.worker.repository $tag -}}
{{- end -}}
