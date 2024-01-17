periodics:
- name: periodic-cluster-api-test-{{ ReplaceAll $.branch "." "-" }}
  cluster: eks-prow-build-cluster
  interval: {{ $.config.Interval }}
  decorate: true
  rerun_auth_config:
    github_team_slugs:
    - org: kubernetes-sigs
      slug: cluster-api-maintainers
  extra_refs:
  - org: kubernetes-sigs
    repo: cluster-api
    base_ref: {{ $.branch }}
    path_alias: sigs.k8s.io/cluster-api
  spec:
    containers:
    - image: {{ $.config.TestImage }}
      command:
      - runner.sh
      - ./scripts/ci-test.sh
      resources:
        requests:
          cpu: 7300m
          memory: 9Gi
        limits:
          cpu: 7300m
          memory: 9Gi
  annotations:
    testgrid-dashboards: sig-cluster-lifecycle-cluster-api{{ if eq $.branch "main" | not -}}{{ TrimPrefix $.branch "release" }}{{- end }}
    testgrid-tab-name: capi-test-{{ ReplaceAll $.branch "." "-" }}
    testgrid-alert-email: sig-cluster-lifecycle-cluster-api-alerts@kubernetes.io
    testgrid-num-failures-to-alert: "4"
- name: periodic-cluster-api-test-mink8s-{{ ReplaceAll $.branch "." "-" }}
  cluster: eks-prow-build-cluster
  interval: {{ $.config.Interval }}
  decorate: true
  rerun_auth_config:
    github_team_slugs:
    - org: kubernetes-sigs
      slug: cluster-api-maintainers
  extra_refs:
  - org: kubernetes-sigs
    repo: cluster-api
    base_ref: {{ $.branch }}
    path_alias: sigs.k8s.io/cluster-api
  spec:
    containers:
    - image: {{ $.config.TestImage }}
      command:
      - runner.sh
      - ./scripts/ci-test.sh
      env:
      # This value determines the minimum Kubernetes
      # supported version for Cluster API management cluster
      # and can be found by referring to [Supported Kubernetes Version](https://cluster-api.sigs.k8s.io/reference/versions.html#supported-kubernetes-versions)
      # docs (look for minimum supported k8s version for management cluster, i.e N-3).
      #
      # To check the latest available envtest in Kubebuilder for the minor version we determined above, please
      # refer to https://github.com/kubernetes-sigs/kubebuilder/tree/tools-releases.
      - name: KUBEBUILDER_ENVTEST_KUBERNETES_VERSION
        value: "{{ $.config.KubebuilderEnvtestKubernetesVersion }}"
      resources:
        requests:
          cpu: 7300m
          memory: 9Gi
        limits:
          cpu: 7300m
          memory: 9Gi
  annotations:
    testgrid-dashboards: sig-cluster-lifecycle-cluster-api{{ if eq $.branch "main" | not -}}{{ TrimPrefix $.branch "release" }}{{- end }}
    testgrid-tab-name: capi-test-mink8s-{{ ReplaceAll $.branch "." "-" }}
    testgrid-alert-email: sig-cluster-lifecycle-cluster-api-alerts@kubernetes.io
    testgrid-num-failures-to-alert: "4"
- name: periodic-cluster-api-e2e-{{ ReplaceAll $.branch "." "-" }}
  cluster: eks-prow-build-cluster
  interval: {{ $.config.Interval }}
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
          - name: GINKGO_SKIP
            value: "\\[Conformance\\] \\[K8s-Upgrade\\]|\\[IPv6\\]"
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
    testgrid-tab-name: capi-e2e-{{ ReplaceAll $.branch "." "-" }}
    testgrid-alert-email: sig-cluster-lifecycle-cluster-api-alerts@kubernetes.io
    testgrid-num-failures-to-alert: "4"
{{ if eq $.branch "release-1.4" | not -}}
- name: periodic-cluster-api-e2e-dualstack-and-ipv6-{{ ReplaceAll $.branch "." "-" }}
  cluster: eks-prow-build-cluster
  interval: {{ $.config.Interval }}
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
          # enable IPV6 in bootstrap image
          - name: "DOCKER_IN_DOCKER_IPV6_ENABLED"
            value: "true"
          - name: GINKGO_SKIP
            value: "\\[Conformance\\] \\[K8s-Upgrade\\]"
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
    testgrid-tab-name: capi-e2e-dualstack-and-ipv6-{{ ReplaceAll $.branch "." "-" }}
    testgrid-alert-email: sig-cluster-lifecycle-cluster-api-alerts@kubernetes.io
    testgrid-num-failures-to-alert: "4"
{{ end -}}
- name: periodic-cluster-api-e2e-mink8s-{{ ReplaceAll $.branch "." "-" }}
  cluster: eks-prow-build-cluster
  interval: {{ $.config.Interval }}
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
      - name: GINKGO_SKIP
        value: "\\[Conformance\\] \\[K8s-Upgrade\\]|\\[IPv6\\]"
      # This value determines the minimum Kubernetes
      # supported version for Cluster API management cluster
      # and can be found by referring to [Supported Kubernetes Version](https://cluster-api.sigs.k8s.io/reference/versions.html#supported-kubernetes-versions)
      # docs (look for minimum supported k8s version for management cluster, i.e N-3).
      # Please also make sure to refer a version where a kindest/node image exists
      # for (see https://github.com/kubernetes-sigs/kind/releases/)
      - name: KUBERNETES_VERSION_MANAGEMENT
        value: "{{ $.config.KubernetesVersionManagement }}"
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
    testgrid-tab-name: capi-e2e-mink8s-{{ ReplaceAll $.branch "." "-" }}
    testgrid-alert-email: sig-cluster-lifecycle-cluster-api-alerts@kubernetes.io
    testgrid-num-failures-to-alert: "4"
