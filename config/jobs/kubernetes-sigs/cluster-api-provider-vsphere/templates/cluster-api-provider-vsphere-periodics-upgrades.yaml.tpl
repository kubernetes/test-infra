{{ $modes := list "govmomi" "supervisor" -}}
{{ range $i, $mode := $modes -}}
{{ $modeFocus := "" -}}
{{ if eq $mode "supervisor" }}{{ $modeFocus = "\\\\[supervisor\\\\] " }}{{ end -}}
{{/* Run govmomi at 00:00 UTC, supervisor at 03:00 UTC */ -}}
{{ $cron := "'0 0 * * *'" -}}
{{ if eq $mode "supervisor" }}{{ $cron = "'0 3 * * *'" }}{{ end -}}
{{/* e2e upgrade has been introduced in release-1.9 */ -}}
{{/* e2e upgrade in supervisor mode has been introduced in release-1.10 */ -}}
{{ $skipInBranch := list "release-1.5" "release-1.6" "release-1.7" "release-1.8" -}}
{{ if eq $mode "supervisor" }}{{ $skipInBranch = list "release-1.5" "release-1.6" "release-1.7" "release-1.8" "release-1.9" }}{{ end -}}
{{ if has $.branch $skipInBranch | not -}}
{{ if eq $i 0 -}}
periodics:
{{ end -}}
{{- range $_, $upgrade := $.config.Upgrades }}
- name: periodic-cluster-api-provider-vsphere-e2e-{{ $mode }}-upgrade-{{ ReplaceAll (TrimPrefix (TrimPrefix $upgrade.From "stable-") "ci/latest-") "." "-" }}-{{ ReplaceAll (TrimPrefix (TrimPrefix $upgrade.To "stable-") "ci/latest-") "." "-" }}-{{ ReplaceAll $.branch "." "-" }}
  cron: {{ $cron }}
  decorate: true
  rerun_auth_config:
    github_team_slugs:
    - org: kubernetes-sigs
      slug: cluster-api-provider-vsphere-maintainers
  labels:
    preset-dind-enabled: "true"
    preset-cluster-api-provider-vsphere-e2e-config: "true"
    preset-cluster-api-provider-vsphere-gcs-creds: "true"
    preset-kind-volume-mounts: "true"
  extra_refs:
  - org: kubernetes-sigs
    repo: cluster-api-provider-vsphere
    base_ref: {{ $.branch }}
    path_alias: sigs.k8s.io/cluster-api-provider-vsphere
  spec:
    containers:
    - image: {{ $.config.TestImage }}
      command:
      - runner.sh
      args:
      - ./hack/e2e.sh
      env:
      - name: GINKGO_FOCUS
        value: "{{ $modeFocus }}\\[Conformance\\] \\[K8s-Upgrade\\]"
      - name: KUBERNETES_VERSION_UPGRADE_FROM
        value: "{{ index (index $.versions $upgrade.From) "k8sRelease" }}"
      - name: KUBERNETES_VERSION_UPGRADE_TO
        value: "{{ index (index $.versions $upgrade.To) "k8sRelease" }}"
      # we need privileged mode in order to do docker in docker
      securityContext:
        privileged: true
        capabilities:
          add: ["NET_ADMIN"]
      resources:
        requests:
          cpu: "4000m"
          memory: "6Gi"
  annotations:
    testgrid-dashboards: vmware-cluster-api-provider-vsphere, sig-cluster-lifecycle-cluster-api-provider-vsphere
    testgrid-tab-name: periodic-e2e-{{ $mode }}-{{ ReplaceAll $.branch "." "-" }}-{{ ReplaceAll (TrimPrefix (TrimPrefix $upgrade.From "stable-") "ci/latest-") "." "-" }}-{{ ReplaceAll (TrimPrefix (TrimPrefix $upgrade.To "stable-") "ci/latest-") "." "-" }}
    testgrid-alert-email: sig-cluster-lifecycle-cluster-api-vsphere-alerts@kubernetes.io
    testgrid-num-failures-to-alert: "4"
{{ end -}}
{{ end -}}
{{ end -}}
