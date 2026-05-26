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

{{- define "dell-bios.alertGroups" -}}
groups:
  - name: dell.bios.sysprofile
    rules:
      {{- if .Values.alerts.rules.drift.enabled }}
      - alert: DellBiosSysProfileDrift
        expr: dell_bios_sys_profile_matches_target == 0
        for: {{ .Values.alerts.rules.drift.for }}
        labels:
          severity: {{ .Values.alerts.rules.drift.severity }}
          {{- with .Values.alerts.additionalLabels }}
          {{- toYaml . | nindent 10 }}
          {{- end }}
        annotations:
          summary: "Dell {{`{{ $labels.node }}`}}: BIOS System Profile differs from target"
          description: |
            On node {{`{{ $labels.node }}`}} (Service Tag {{`{{ $labels.service_tag }}`}}, model {{`{{ $labels.model }}`}})
            the current System Profile = {{`{{ $labels.profile }}`}}, expected {{ .Values.alerts.targetProfile }}.
            Go to iDRAC -> Configuration -> BIOS Settings -> System Profile Settings and set Performance.
            A server reboot is required after the change.
      {{- end }}
      {{- if .Values.alerts.rules.scrapeFailing.enabled }}
      - alert: DellBiosRacadmFailing
        expr: dell_bios_racadm_success == 0
        for: {{ .Values.alerts.rules.scrapeFailing.for }}
        labels:
          severity: {{ .Values.alerts.rules.scrapeFailing.severity }}
          {{- with .Values.alerts.additionalLabels }}
          {{- toYaml . | nindent 10 }}
          {{- end }}
        annotations:
          summary: "Dell {{`{{ $labels.node }}`}}: exporter cannot read data via racadm"
          description: |
            Possible causes: iSM (dcismeng) not running, iDRAC unreachable over the internal channel,
            racadm binary missing or lacking permissions. Check:
            `systemctl status dcismeng` and `racadm get BIOS.SysProfileSettings.SysProfile`
            directly on node {{`{{ $labels.node }}`}}.
      {{- end }}
      {{- if .Values.alerts.rules.stale.enabled }}
      - alert: DellBiosSysProfileStale
        expr: time() - dell_bios_last_scrape_timestamp_seconds > {{ mul .Values.alerts.rules.stale.maxAgeMinutes 60 }}
        for: {{ .Values.alerts.rules.stale.for }}
        labels:
          severity: {{ .Values.alerts.rules.stale.severity }}
          {{- with .Values.alerts.additionalLabels }}
          {{- toYaml . | nindent 10 }}
          {{- end }}
        annotations:
          summary: "Dell {{`{{ $labels.node }}`}}: System Profile data is stale"
          description: |
            The last successful racadm poll was more than {{ .Values.alerts.rules.stale.maxAgeMinutes }} minutes ago.
            The current profile value may not reflect the real BIOS state.
      {{- end }}
{{- end -}}
