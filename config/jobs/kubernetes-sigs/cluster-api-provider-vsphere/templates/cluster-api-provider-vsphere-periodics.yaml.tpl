periodics:
- name: periodic-cluster-api-provider-vsphere-test-{{ ReplaceAll $.branch "." "-" }}
  cluster: eks-prow-build-cluster
  interval: 1h
  decorate: true
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
    testgrid-dashboards: vmware-cluster-api-provider-vsphere, sig-cluster-lifecycle-cluster-api-provider-vsphere
    testgrid-tab-name: periodic-test-{{ ReplaceAll $.branch "." "-" }}
    testgrid-alert-email: sig-cluster-lifecycle-cluster-api-vsphere-alerts@kubernetes.io
    testgrid-num-failures-to-alert: "4"
    description: Runs unit tests

- name: periodic-cluster-api-provider-vsphere-test-integration-{{ ReplaceAll $.branch "." "-" }}
  cluster: eks-prow-build-cluster
  labels:
    preset-dind-enabled: "true"
    preset-kind-volume-mounts: "true"
  interval: 1h
  decorate: true
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
      # we need privileged mode in order to do docker in docker
      securityContext:
        privileged: true
        capabilities:
          add: ["NET_ADMIN"]
      resources:
        limits:
          cpu: 4
          memory: 6Gi
        requests:
          cpu: 4
          memory: 6Gi
      command:
      - runner.sh
      args:
      - make
      - test-integration
  annotations:
    testgrid-dashboards: vmware-cluster-api-provider-vsphere, sig-cluster-lifecycle-cluster-api-provider-vsphere
    testgrid-tab-name: periodic-test-integration-{{ ReplaceAll $.branch "." "-" }}
    testgrid-alert-email: sig-cluster-lifecycle-cluster-api-vsphere-alerts@kubernetes.io
    testgrid-num-failures-to-alert: "4"
    description: Runs integration tests

- name: periodic-cluster-api-provider-vsphere-e2e-{{ ReplaceAll $.branch "." "-" }}
  labels:
    preset-dind-enabled: "true"
    preset-cluster-api-provider-vsphere-e2e-config: "true"
    preset-cluster-api-provider-vsphere-gcs-creds: "true"
    preset-kind-volume-mounts: "true"
  interval: {{ $.config.Interval }}
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
  spec:
    containers:
    - image: {{ $.config.TestImage }}
      command:
      - runner.sh
      args:
      - ./hack/e2e.sh
      env:
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
  annotations:
    testgrid-dashboards: vmware-cluster-api-provider-vsphere, sig-cluster-lifecycle-cluster-api-provider-vsphere
    testgrid-tab-name: periodic-e2e-{{ ReplaceAll $.branch "." "-" }}
    testgrid-alert-email: sig-cluster-lifecycle-cluster-api-vsphere-alerts@kubernetes.io
    testgrid-num-failures-to-alert: "4"
    description: Runs all e2e tests

- name: periodic-cluster-api-provider-vsphere-e2e-conformance-{{ ReplaceAll $.branch "." "-" }}
  labels:
    preset-dind-enabled: "true"
    preset-cluster-api-provider-vsphere-e2e-config: "true"
    preset-cluster-api-provider-vsphere-gcs-creds: "true"
    preset-kind-volume-mounts: "true"
  interval: {{ $.config.Interval }}
  decorate: true
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
      - ./hack/e2e.sh
      env:
      - name: GINKGO_FOCUS
{{- if eq $.branch "release-1.5" "release-1.6" "release-1.7" "release-1.8" }}
        value: "\\[Conformance\\]"
{{- else }}
        value: "\\[Conformance\\] \\[K8s-Install\\]"
{{- end }}
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
    testgrid-tab-name: periodic-e2e-conformance-{{ ReplaceAll $.branch "." "-" }}
    testgrid-alert-email: sig-cluster-lifecycle-cluster-api-vsphere-alerts@kubernetes.io
    testgrid-num-failures-to-alert: "4"
    description: Runs conformance tests for CAPV
{{ if eq $.branch "release-1.5" "release-1.6" "release-1.7" "release-1.8" | not }}
- name: periodic-cluster-api-provider-vsphere-e2e-conformance-ci-latest-{{ ReplaceAll $.branch "." "-" }}
  labels:
    preset-dind-enabled: "true"
    preset-cluster-api-provider-vsphere-e2e-config: "true"
    preset-cluster-api-provider-vsphere-gcs-creds: "true"
    preset-kind-volume-mounts: "true"
  interval: {{ $.config.Interval }}
  decorate: true
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
      - ./hack/e2e.sh
      env:
      - name: GINKGO_FOCUS
        value: "\\[Conformance\\] \\[K8s-Install-ci-latest\\]"
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
    testgrid-tab-name: periodic-e2e-conformance-ci-latest-{{ ReplaceAll $.branch "." "-" }}
    testgrid-alert-email: sig-cluster-lifecycle-cluster-api-vsphere-alerts@kubernetes.io
    testgrid-num-failures-to-alert: "4"
    description: Runs conformance tests with K8S ci latest for CAPV
{{ end -}}
{{- if eq $.branch "main" }}
- name: periodic-cluster-api-provider-vsphere-coverage-{{ ReplaceAll $.branch "." "-" }}
  cluster: eks-prow-build-cluster
  interval: {{ $.config.Interval }}
  decorate: true
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
          memory: "6Gi"
        limits:
          cpu: "4000m"
          memory: "6Gi"
  annotations:
    testgrid-dashboards: vmware-cluster-api-provider-vsphere, sig-cluster-lifecycle-cluster-api-provider-vsphere
    testgrid-tab-name: periodic-test-coverage-{{ ReplaceAll $.branch "." "-" }}
    testgrid-alert-email: sig-cluster-lifecycle-cluster-api-vsphere-alerts@kubernetes.io
    testgrid-num-failures-to-alert: "4"
    description: Shows test coverage for CAPV
{{ end -}}
