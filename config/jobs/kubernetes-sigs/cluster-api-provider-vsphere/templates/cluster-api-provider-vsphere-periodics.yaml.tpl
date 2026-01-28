periodics:
- name: periodic-cluster-api-provider-vsphere-test-{{ ReplaceAll $.branch "." "-" }}
  cluster: eks-prow-build-cluster
  interval: 1h
  decorate: true
  decoration_config:
    timeout: 120m
  rerun_auth_config:
    github_team_slugs:
    - org: kubernetes-sigs
      slug: cluster-api-provider-vsphere-maintainers
  extra_refs:
  - org: kubernetes-sigs
    repo: cluster-api-provider-vsphere
    base_ref: {{ $.branch }}
    path_alias: sigs.k8s.io/cluster-api-provider-vsphere
  spec:
    containers:
    - image: {{ $.config.TestImage }}
      resources:
        limits:
          cpu: 2
          memory: 4Gi
        requests:
          cpu: 2
          memory: 4Gi
      command:
      - runner.sh
      args:
      - make
      - test-junit
  annotations:
    testgrid-dashboards: cluster-api-provider-vsphere-{{ TrimPrefix $.branch "release-" }}
    testgrid-tab-name: periodic-test-{{ ReplaceAll $.branch "." "-" }}
    testgrid-alert-email: sig-cluster-lifecycle-cluster-api-vsphere-alerts@kubernetes.io
    testgrid-num-failures-to-alert: "4"
    description: Runs unit tests
{{ $modes := list "govmomi" "supervisor" -}}
{{ range $i, $mode := $modes -}}
{{ $modeFocus := "" -}}
{{ if eq $mode "supervisor" }}{{ $modeFocus = "\\\\[supervisor\\\\] " }}{{ end -}}
{{/* Run govmomi at 00:00 UTC, supervisor at 03:00 UTC */ -}}
{{ $cron := "'0 21 * * *'" -}}
{{ if eq $mode "supervisor" }}{{ $cron = "'0 19 * * *'" }}{{ end -}}
- name: periodic-cluster-api-provider-vsphere-e2e-{{ $mode }}-{{ ReplaceAll $.branch "." "-" }}
  cluster: k8s-infra-prow-build
  labels:
    preset-dind-enabled: "true"
    preset-gcve-e2e-config: "true"
    preset-kind-volume-mounts: "true"
  cron: {{ $cron }}
  decorate: true
  decoration_config:
    timeout: 180m
  rerun_auth_config:
    github_team_slugs:
    - org: kubernetes-sigs
      slug: cluster-api-provider-vsphere-maintainers
  extra_refs:
  - org: kubernetes-sigs
    repo: cluster-api-provider-vsphere
    base_ref: {{ $.branch }}
    path_alias: sigs.k8s.io/cluster-api-provider-vsphere
  - org: kubernetes
    repo: kubernetes
    base_ref: master
    path_alias: k8s.io/kubernetes
  spec:
    containers:
    - image: {{ $.config.TestImage }}
      command:
      - runner.sh
      args:
      - ./hack/e2e.sh
      env:
{{- if ne $modeFocus "" }}
      - name: GINKGO_FOCUS
        value: "{{ trim $modeFocus }}"
{{- end }}
      - name: GINKGO_SKIP
        value: "\\[Conformance\\] \\[specialized-infra\\]"
      # we need privileged mode in order to do docker in docker
      securityContext:
        privileged: true
        capabilities:
          add: ["NET_ADMIN"]
      resources:
        requests:
          cpu: "4000m"
          memory: "6Gi"
        limits:
          cpu: "4000m"
          memory: "6Gi"
  annotations:
    testgrid-dashboards: cluster-api-provider-vsphere-{{ TrimPrefix $.branch "release-" }}
    testgrid-tab-name: periodic-e2e-{{ $mode }}-{{ ReplaceAll $.branch "." "-" }}
    testgrid-alert-email: sig-cluster-lifecycle-cluster-api-vsphere-alerts@kubernetes.io
    testgrid-num-failures-to-alert: "4"
    description: Runs all e2e tests
{{ if eq $mode "supervisor" -}}
{{ if not (or (eq $.branch "release-1.13") (eq $.branch "release-1.14") (eq $.branch "release-1.15")) -}}
{{ $vmOperatorVersions := list "v1.8.6-0-gde75746a" "v1.9.0-567-g93918c59" -}}
{{ range $i, $vmOperatorVersion := $vmOperatorVersions -}}
{{ $vmOperatorApiVersions := list "v1alpha2" -}}
{{ range $j, $vmOperatorApiVersion := $vmOperatorApiVersions }}
- name: periodic-cluster-api-provider-vsphere-e2e-{{ $mode }}-{{ printf "%.6s" (ReplaceAll $vmOperatorVersion "." "-") }}-{{ $vmOperatorApiVersion }}-{{ ReplaceAll $.branch "." "-" }}
  cluster: k8s-infra-prow-build
  labels:
    preset-dind-enabled: "true"
    preset-gcve-e2e-config: "true"
    preset-kind-volume-mounts: "true"
  cron: '0 7 * * *'
  decorate: true
  decoration_config:
    timeout: 180m
  rerun_auth_config:
    github_team_slugs:
    - org: kubernetes-sigs
      slug: cluster-api-provider-vsphere-maintainers
  extra_refs:
  - org: kubernetes-sigs
    repo: cluster-api-provider-vsphere
    base_ref: {{ $.branch }}
    path_alias: sigs.k8s.io/cluster-api-provider-vsphere
  - org: kubernetes
    repo: kubernetes
    base_ref: master
    path_alias: k8s.io/kubernetes
  spec:
    containers:
    - image: {{ $.config.TestImage }}
      command:
      - runner.sh
      args:
      - ./hack/e2e.sh
      env:
{{- if ne $modeFocus "" }}
      # running only PR blocking jobs because this gives enough signal for VM operator versions
      - name: GINKGO_FOCUS
        value: "{{ $modeFocus }}\\[PR-Blocking\\]"
{{- end }}
      - name: VM_OPERATOR_VERSION
        value: "{{ $vmOperatorVersion }}"
      - name: VM_OPERATOR_API_VERSION
        value: "{{ $vmOperatorApiVersion }}"
      # we need privileged mode in order to do docker in docker
      securityContext:
        privileged: true
        capabilities:
          add: ["NET_ADMIN"]
      resources:
        requests:
          cpu: "4000m"
          memory: "6Gi"
        limits:
          cpu: "4000m"
          memory: "6Gi"
  annotations:
    testgrid-dashboards: cluster-api-provider-vsphere-{{ TrimPrefix $.branch "release-" }}
    testgrid-tab-name: periodic-e2e-{{ $mode }}-{{ printf "%.6s" (ReplaceAll $vmOperatorVersion "." "-") }}-{{ $vmOperatorApiVersion }}-{{ ReplaceAll $.branch "." "-" }}
    testgrid-alert-email: sig-cluster-lifecycle-cluster-api-vsphere-alerts@kubernetes.io
    testgrid-num-failures-to-alert: "4"
    description: Runs all e2e tests
{{ end }}
{{- end }}
{{- end }}
{{- end }}
- name: periodic-cluster-api-provider-vsphere-e2e-vcsim-{{ $mode }}-{{ ReplaceAll $.branch "." "-" }}
  cluster: eks-prow-build-cluster
  labels:
    preset-dind-enabled: "true"
    preset-kind-volume-mounts: "true"
  cron: {{ $cron }}
  decorate: true
  decoration_config:
    timeout: 180m
  rerun_auth_config:
    github_team_slugs:
    - org: kubernetes-sigs
      slug: cluster-api-provider-vsphere-maintainers
  extra_refs:
  - org: kubernetes-sigs
    repo: cluster-api-provider-vsphere
    base_ref: {{ $.branch }}
    path_alias: sigs.k8s.io/cluster-api-provider-vsphere
  - org: kubernetes
    repo: kubernetes
    base_ref: master
    path_alias: k8s.io/kubernetes
  spec:
    containers:
    - image: {{ $.config.TestImage }}
      command:
      - runner.sh
      args:
      - ./hack/e2e.sh
      env:
      - name: GINKGO_FOCUS
{{- if eq $mode "supervisor" }}
        value: "\\[vcsim\\] \\[supervisor\\]"
{{- else }}
        value: "\\[vcsim\\]"
{{- end }}
      # we need privileged mode in order to do docker in docker
      securityContext:
        privileged: true
        capabilities:
          add: ["NET_ADMIN"]
      resources:
        requests:
          cpu: "4000m"
          memory: "3Gi"
        limits:
          cpu: "4000m"
          memory: "3Gi"
  annotations:
    testgrid-dashboards: cluster-api-provider-vsphere-{{ TrimPrefix $.branch "release-" }}
    testgrid-tab-name: periodic-e2e-vcsim-{{ $mode }}-{{ ReplaceAll $.branch "." "-" }}
    testgrid-alert-email: sig-cluster-lifecycle-cluster-api-vsphere-alerts@kubernetes.io
    testgrid-num-failures-to-alert: "4"
    description: Runs all e2e tests
- name: periodic-cluster-api-provider-vsphere-e2e-{{ $mode }}-conformance-{{ ReplaceAll $.branch "." "-" }}
  cluster: k8s-infra-prow-build
  labels:
    preset-dind-enabled: "true"
    preset-gcve-e2e-config: "true"
    preset-kind-volume-mounts: "true"
  cron: {{ $cron }}
  decorate: true
  decoration_config:
    timeout: 120m
  rerun_auth_config:
    github_team_slugs:
    - org: kubernetes-sigs
      slug: cluster-api-provider-vsphere-maintainers
  extra_refs:
  - org: kubernetes-sigs
    repo: cluster-api-provider-vsphere
    base_ref: {{ $.branch }}
    path_alias: sigs.k8s.io/cluster-api-provider-vsphere
  - org: kubernetes
    repo: kubernetes
    base_ref: master
    path_alias: k8s.io/kubernetes
  spec:
    containers:
    - image: {{ $.config.TestImage }}
      command:
      - runner.sh
      args:
      - ./hack/e2e.sh
      env:
      - name: GINKGO_FOCUS
        value: "{{ $modeFocus }}\\[Conformance\\] \\[K8s-Install\\]"
      # we need privileged mode in order to do docker in docker
      securityContext:
        privileged: true
        capabilities:
          add: ["NET_ADMIN"]
      resources:
        requests:
          cpu: "4000m"
          memory: "6Gi"
        limits:
          cpu: "4000m"
          memory: "6Gi"
  annotations:
    testgrid-dashboards: cluster-api-provider-vsphere-{{ TrimPrefix $.branch "release-" }}
    testgrid-tab-name: periodic-e2e-{{ $mode }}-conformance-{{ ReplaceAll $.branch "." "-" }}
    testgrid-alert-email: sig-cluster-lifecycle-cluster-api-vsphere-alerts@kubernetes.io
    testgrid-num-failures-to-alert: "4"
    description: Runs conformance tests for CAPV
- name: periodic-cluster-api-provider-vsphere-e2e-{{ $mode }}-conformance-ci-latest-{{ ReplaceAll $.branch "." "-" }}
  cluster: k8s-infra-prow-build
  labels:
    preset-dind-enabled: "true"
    preset-gcve-e2e-config: "true"
    preset-kind-volume-mounts: "true"
  cron: {{ $cron }}
  decorate: true
  decoration_config:
    timeout: 120m
  rerun_auth_config:
    github_team_slugs:
    - org: kubernetes-sigs
      slug: cluster-api-provider-vsphere-maintainers
  extra_refs:
  - org: kubernetes-sigs
    repo: cluster-api-provider-vsphere
    base_ref: {{ $.branch }}
    path_alias: sigs.k8s.io/cluster-api-provider-vsphere
  - org: kubernetes
    repo: kubernetes
    base_ref: master
    path_alias: k8s.io/kubernetes
  spec:
    containers:
    - image: {{ $.config.TestImage }}
      command:
      - runner.sh
      args:
      - ./hack/e2e.sh
      env:
      - name: GINKGO_FOCUS
        value: "{{ $modeFocus }}\\[Conformance\\] \\[K8s-Install-ci-latest\\]"
      # we need privileged mode in order to do docker in docker
      securityContext:
        privileged: true
        capabilities:
          add: ["NET_ADMIN"]
      resources:
        requests:
          cpu: "4000m"
          memory: "6Gi"
        limits:
          cpu: "4000m"
          memory: "6Gi"
  annotations:
    testgrid-dashboards: cluster-api-provider-vsphere-{{ TrimPrefix $.branch "release-" }}
    testgrid-tab-name: periodic-e2e-{{ $mode }}-conformance-ci-latest-{{ ReplaceAll $.branch "." "-" }}
    testgrid-alert-email: sig-cluster-lifecycle-cluster-api-vsphere-alerts@kubernetes.io
    testgrid-num-failures-to-alert: "4"
    description: Runs conformance tests with K8S ci latest for CAPV
{{ end -}}
{{- if eq $.branch "main" }}
- name: periodic-cluster-api-provider-vsphere-coverage-{{ ReplaceAll $.branch "." "-" }}
  cluster: eks-prow-build-cluster
  interval: {{ $.config.Interval }}
  decorate: true
  decoration_config:
    timeout: 120m
  rerun_auth_config:
    github_team_slugs:
    - org: kubernetes-sigs
      slug: cluster-api-provider-vsphere-maintainers
  extra_refs:
  - org: kubernetes-sigs
    repo: cluster-api-provider-vsphere
    base_ref: {{ $.branch }}
    path_alias: sigs.k8s.io/cluster-api-provider-vsphere
  - org: kubernetes
    repo: test-infra
    base_ref: master
    path_alias: k8s.io/test-infra
  spec:
    containers:
    - image: {{ $.config.TestImage }}
      command:
      - runner.sh
      - bash
      args:
      - -c
      - |
        result=0
        ./hack/ci-test-coverage.sh || result=$?
        cp coverage.* ${ARTIFACTS}
        cd ../../k8s.io/test-infra/gopherage
        GO111MODULE=on go build .
        ./gopherage filter --exclude-path="zz_generated,generated\.go" "${ARTIFACTS}/coverage.out" > "${ARTIFACTS}/filtered.cov" || result=$?
        ./gopherage html "${ARTIFACTS}/filtered.cov" > "${ARTIFACTS}/coverage.html" || result=$?
        ./gopherage junit --threshold 0 "${ARTIFACTS}/filtered.cov" > "${ARTIFACTS}/junit_coverage.xml" || result=$?
        exit $result
      securityContext:
        privileged: true
        capabilities:
          add: ["NET_ADMIN"]
      resources:
        requests:
          cpu: "4000m"
          memory: "4Gi"
        limits:
          cpu: "4000m"
          memory: "4Gi"
  annotations:
    testgrid-dashboards: cluster-api-provider-vsphere-{{ TrimPrefix $.branch "release-" }}
    testgrid-tab-name: periodic-test-coverage-{{ ReplaceAll $.branch "." "-" }}
    testgrid-alert-email: sig-cluster-lifecycle-cluster-api-vsphere-alerts@kubernetes.io
    testgrid-num-failures-to-alert: "4"
    description: Shows test coverage for CAPV

- name: periodic-cluster-api-provider-vsphere-janitor
  cluster: k8s-infra-prow-build
  labels:
    preset-dind-enabled: "true"
    preset-gcve-e2e-config: "true"
  interval: 12h
  decorate: true
  decoration_config:
    timeout: 120m
  rerun_auth_config:
    github_team_slugs:
    - org: kubernetes-sigs
      slug: cluster-api-provider-vsphere-maintainers
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
      - ./hack/clean-ci.sh
      # we need privileged mode in order to do docker in docker
      securityContext:
        privileged: true
        capabilities:
          add: ["NET_ADMIN"]
      resources:
        requests:
          cpu: "2000m"
          memory: "4Gi"
        limits:
          cpu: "2000m"
          memory: "4Gi"
  annotations:
    testgrid-dashboards: cluster-api-provider-vsphere-{{ TrimPrefix $.branch "release-" }}
    testgrid-tab-name: periodic-e2e-janitor
    testgrid-alert-email: sig-cluster-lifecycle-cluster-api-vsphere-alerts@kubernetes.io
    testgrid-num-failures-to-alert: "4"
    description: Runs the janitor to cleanup orphaned objects in CI
{{ end -}}
