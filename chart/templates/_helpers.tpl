{{- define "dell-bios.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "dell-bios.fullname" -}}
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

{{- define "dell-bios.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "dell-bios.labels" -}}
helm.sh/chart: {{ include "dell-bios.chart" . }}
{{ include "dell-bios.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "dell-bios.selectorLabels" -}}
app.kubernetes.io/name: {{ include "dell-bios.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "dell-bios.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "dell-bios.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "dell-bios.shouldRenderPrometheus" -}}
{{- if or (eq .Values.monitoring.stack "prometheus") (eq .Values.monitoring.stack "both") -}}true{{- end -}}
{{- end -}}

{{- define "dell-bios.shouldRenderVictoriaMetrics" -}}
{{- if or (eq .Values.monitoring.stack "victoriametrics") (eq .Values.monitoring.stack "both") -}}true{{- end -}}
{{- end -}}
