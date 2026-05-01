{{/* vim: set filetype=mustache: */}}

{{/*
Expand the name of the chart.
*/}}
{{- define "webhook.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
Truncated at 63 chars because some Kubernetes name fields are limited to this
(DNS naming spec).
*/}}
{{- define "webhook.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{/*
Chart name + version label.
*/}}
{{- define "webhook.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
PKI helper names.
*/}}
{{- define "webhook.selfSignedIssuer" -}}
{{ printf "%s-selfsign" (include "webhook.fullname" .) }}
{{- end -}}

{{- define "webhook.rootCAIssuer" -}}
{{ printf "%s-ca" (include "webhook.fullname" .) }}
{{- end -}}

{{- define "webhook.rootCACertificate" -}}
{{ printf "%s-ca" (include "webhook.fullname" .) }}
{{- end -}}

{{- define "webhook.servingCertificate" -}}
{{ printf "%s-tls" (include "webhook.fullname" .) }}
{{- end -}}
