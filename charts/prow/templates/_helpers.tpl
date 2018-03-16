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

{{- define "job_url_template" -}}
{{-  printf `https://k8s-gubernator.appspot.com/build/kubernetes-jenkins/{{if eq .Spec.Type "presubmit"}}pr-logs/pull{{else if eq .Spec.Type "batch"}}pr-logs/pull{{else}}logs{{end}}{{if ne .Spec.Refs.Org ""}}{{if ne .Spec.Refs.Org "kubernetes"}}/{{.Spec.Refs.Org}}_{{.Spec.Refs.Repo}}{{else if ne .Spec.Refs.Repo "kubernetes"}}/{{.Spec.Refs.Repo}}{{end}}{{end}}{{if eq .Spec.Type "presubmit"}}/{{with index .Spec.Refs.Pulls 0}}{{.Number}}{{end}}{{else if eq .Spec.Type "batch"}}/batch{{end}}/{{.Spec.Job}}/{{.Status.BuildID}}/` -}}
{{- end -}}

{{- define "report_template" -}}
{{-  printf `[Full PR test history](https://k8s-gubernator.appspot.com/pr/{{if eq .Spec.Refs.Org "kubernetes"}}{{if eq .Spec.Refs.Repo "kubernetes"}}{{else}}{{.Spec.Refs.Repo}}/{{end}}{{else}}{{.Spec.Refs.Org}}_{{.Spec.Refs.Repo}}/{{end}}{{with index .Spec.Refs.Pulls 0}}{{.Number}}{{end}}). [Your PR dashboard](https://k8s-gubernator.appspot.com/pr/{{with index .Spec.Refs.Pulls 0}}{{.Author}}{{end}}). Please help us cut down on flakes by [linking to](https://github.com/kubernetes/community/blob/master/contributors/devel/flaky-tests.md#filing-issues-for-flaky-tests) an [open issue](https://github.com/{{.Spec.Refs.Org}}/{{.Spec.Refs.Repo}}/issues?q=is:issue+is:open) when you hit one in your PR.` -}}
{{- end -}}
