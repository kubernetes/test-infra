{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 24 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 24 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
*/}}
{{- define "fullname" -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- printf "%s-%s" .Release.Name $name | trunc 24 | trimSuffix "-" -}}
{{- end -}}

{{- define "tot" -}}

{{- printf "tot-%s" .Release.Name | trunc 24 | trimSuffix "-" -}}
{{- end -}}

{{- define "splice" -}}
{{- printf "splice-%s" .Release.Name | trunc 24 | trimSuffix "-" -}}
{{- end -}}

{{- define "sinker" -}}
{{- printf "sinker-%s" .Release.Name | trunc 24 | trimSuffix "-" -}}
{{- end -}}

{{- define "marque" -}}
{{- printf "marque-%s" .Release.Name | trunc 24 | trimSuffix "-" -}}
{{- end -}}

{{- define "horologium" -}}
{{- printf "horologium-%s" .Release.Name | trunc 24 | trimSuffix "-" -}}
{{- end -}}

{{- define "hook" -}}
{{- printf "hook-%s" .Release.Name | trunc 24 | trimSuffix "-" -}}
{{- end -}}

{{- define "deck" -}}
{{- printf "deck-%s" .Release.Name | trunc 24 | trimSuffix "-" -}}
{{- end -}}

{{- define "crier" -}}
{{- printf "crier-%s" .Release.Name | trunc 24 | trimSuffix "-" -}}
{{- end -}}
