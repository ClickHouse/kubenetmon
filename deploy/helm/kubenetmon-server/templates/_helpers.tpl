{{/*
Expand the name of the chart.
*/}}
{{- define "kubenetmon-server.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "kubenetmon-server.fullname" -}}
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
{{- define "kubenetmon-server.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "kubenetmon-server.labels" -}}
helm.sh/chart: {{ include "kubenetmon-server.chart" . }}
{{ include "kubenetmon-server.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "kubenetmon-server.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kubenetmon-server.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "kubenetmon-server.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "kubenetmon-server.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
configmap checksum calculator
*/}}
{{- define "kubenetmon-server.configmap.checksum" -}}
{{- $fileContent := include (print .Template.BasePath "/configMap.yaml") . | fromYaml }}
checksum/configmap: {{ $fileContent.data | toYaml | sha256sum }}
{{- end }}

{{/*
secret checksum calculator
*/}}
{{- define "kubenetmon-server.secret.checksum" -}}
{{- $fileContent := include (print .Template.BasePath "/secret.yaml") . | fromYaml }}
checksum/secret: {{ $fileContent.data | toYaml | sha256sum }}
{{- end }}
