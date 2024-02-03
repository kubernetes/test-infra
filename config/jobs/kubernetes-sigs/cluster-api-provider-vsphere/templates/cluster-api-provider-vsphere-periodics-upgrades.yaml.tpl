{{ if eq $.branch "release-1.5" "release-1.6" "release-1.7" "release-1.8" | not }}
periodics:
{{- range $_, $upgrade := $.config.Upgrades }}
- name: periodic-cluster-api-provider-vsphere-e2e-upgrade-{{ ReplaceAll (TrimPrefix (TrimPrefix $upgrade.From "stable-") "ci/latest-") "." "-" }}-{{ ReplaceAll (TrimPrefix (TrimPrefix $upgrade.To "stable-") "ci/latest-") "." "-" }}-{{ ReplaceAll $.branch "." "-" }}
  interval: {{ $.config.UpgradesInterval }}
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
        value: "\\[Conformance\\] \\[K8s-Upgrade\\]"
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
    testgrid-tab-name: periodic-e2e-{{ ReplaceAll $.branch "." "-" }}-{{ ReplaceAll (TrimPrefix (TrimPrefix $upgrade.From "stable-") "ci/latest-") "." "-" }}-{{ ReplaceAll (TrimPrefix (TrimPrefix $upgrade.To "stable-") "ci/latest-") "." "-" }}
    testgrid-alert-email: sig-cluster-lifecycle-cluster-api-vsphere-alerts@kubernetes.io
    testgrid-num-failures-to-alert: "4"
{{ end -}}
{{ end -}}
