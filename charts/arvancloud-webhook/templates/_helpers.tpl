{{- define "arvancloud-webhook.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "arvancloud-webhook.fullname" -}}
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

{{- define "arvancloud-webhook.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/name: {{ include "arvancloud-webhook.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
{{- end -}}

{{- define "arvancloud-webhook.selectorLabels" -}}
app.kubernetes.io/name: {{ include "arvancloud-webhook.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "arvancloud-webhook.serviceAccountName" -}}
{{ include "arvancloud-webhook.fullname" . }}
{{- end -}}

{{- define "arvancloud-webhook.selfSignedIssuer" -}}
{{ printf "%s-selfsign" (include "arvancloud-webhook.fullname" .) }}
{{- end -}}

{{- define "arvancloud-webhook.rootCAIssuer" -}}
{{ printf "%s-ca" (include "arvancloud-webhook.fullname" .) }}
{{- end -}}

{{- define "arvancloud-webhook.rootCACertificate" -}}
{{ printf "%s-ca" (include "arvancloud-webhook.fullname" .) }}
{{- end -}}

{{- define "arvancloud-webhook.servingCertificate" -}}
{{ printf "%s-webhook-tls" (include "arvancloud-webhook.fullname" .) }}
{{- end -}}
