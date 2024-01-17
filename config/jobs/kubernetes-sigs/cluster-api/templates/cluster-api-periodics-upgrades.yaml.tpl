periodics:
{{- range $_, $upgrade := $.config.Upgrades }}
- name: periodic-cluster-api-e2e-workload-upgrade-{{ ReplaceAll (TrimPrefix (TrimPrefix $upgrade.From "stable-") "ci/latest-") "." "-" }}-{{ ReplaceAll (TrimPrefix (TrimPrefix $upgrade.To "stable-") "ci/latest-") "." "-" }}-{{ ReplaceAll $.branch "." "-" }}
  cluster: eks-prow-build-cluster
  interval: {{ $.config.UpgradesInterval }}
  decorate: true
  rerun_auth_config:
    github_team_slugs:
    - org: kubernetes-sigs
      slug: cluster-api-maintainers
  labels:
    preset-dind-enabled: "true"
    preset-kind-volume-mounts: "true"
  extra_refs:
  - org: kubernetes-sigs
    repo: cluster-api
    base_ref: {{ $.branch }}
    path_alias: sigs.k8s.io/cluster-api
  - org: kubernetes
    repo: kubernetes
    base_ref: master
    path_alias: k8s.io/kubernetes
  spec:
    containers:
    - image: {{ $.config.TestImage }}
      args:
      - runner.sh
      - "./scripts/ci-e2e.sh"
      env:
      - name: ALWAYS_BUILD_KIND_IMAGES
        value: "true"
      - name: KUBERNETES_VERSION_UPGRADE_FROM
        value: "{{ index (index $.versions $upgrade.From) "k8sRelease" }}"
      - name: KUBERNETES_VERSION_UPGRADE_TO
        value: "{{ index (index $.versions $upgrade.To) "k8sRelease" }}"
      - name: ETCD_VERSION_UPGRADE_TO
        value: "{{ index (index $.versions $upgrade.To) "etcd" }}"
      - name: COREDNS_VERSION_UPGRADE_TO
        value: "{{ index (index $.versions $upgrade.To) "coreDNS" }}"
      - name: GINKGO_FOCUS
        value: "\\[K8s-Upgrade\\]"
      # we need privileged mode in order to do docker in docker
      securityContext:
        privileged: true
      resources:
        requests:
          cpu: 7300m
          memory: 32Gi
        limits:
          cpu: 7300m
          memory: 32Gi
  annotations:
    testgrid-dashboards: sig-cluster-lifecycle-cluster-api{{ if eq $.branch "main" | not -}}{{ TrimPrefix $.branch "release" }}{{- end }}
    testgrid-tab-name: capi-e2e-{{ ReplaceAll $.branch "." "-" }}-{{ ReplaceAll (TrimPrefix (TrimPrefix $upgrade.From "stable-") "ci/latest-") "." "-" }}-{{ ReplaceAll (TrimPrefix (TrimPrefix $upgrade.To "stable-") "ci/latest-") "." "-" }}
    testgrid-alert-email: sig-cluster-lifecycle-cluster-api-alerts@kubernetes.io
    testgrid-num-failures-to-alert: "4"
{{ end -}}
