presubmits:
  kubernetes-sigs/cluster-api-provider-vsphere:
  - name: pull-cluster-api-provider-vsphere-apidiff-{{ ReplaceAll $.branch "." "-" }}
    cluster: eks-prow-build-cluster
    branches:
    - ^{{ $.branch }}$
    always_run: false
    # Run if go files, scripts or configuration changed (we use the same for all jobs for simplicity).
    run_if_changed: '^((apis|config|controllers|feature|hack|packaging|pkg|test|webhooks)/|Dockerfile|go\.mod|go\.sum|main\.go|Makefile)'
    optional: true
    decorate: true
    decoration_config:
      timeout: 120m
    path_alias: sigs.k8s.io/cluster-api-provider-vsphere
    spec:
      containers:
      - image: {{ $.config.TestImage }}
        command:
        - runner.sh
        args:
        - ./hack/ci-apidiff.sh
        resources:
          limits:
            cpu: 2
            memory: 3Gi
          requests:
            cpu: 2
            memory: 3Gi
    annotations:
      testgrid-dashboards: cluster-api-provider-vsphere-{{ TrimPrefix $.branch "release-" }}
      testgrid-tab-name: pr-apidiff-{{ ReplaceAll $.branch "." "-" }}
      description: Checks for API changes in the PR

  - name: pull-cluster-api-provider-vsphere-verify-{{ ReplaceAll $.branch "." "-" }}
    cluster: eks-prow-build-cluster
    branches:
    - ^{{ $.branch }}$
    labels:
      preset-dind-enabled: "true"
    always_run: true
    decorate: true
    decoration_config:
      timeout: 120m
    path_alias: sigs.k8s.io/cluster-api-provider-vsphere
    spec:
      containers:
      - image: {{ $.config.TestImage }}
        command:
        - runner.sh
        args:
        - make
        - verify
        # we need privileged mode in order to do docker in docker
        securityContext:
          privileged: true
        resources:
          limits:
            cpu: 2
            memory: 3Gi
          requests:
            cpu: 2
            memory: 3Gi
    annotations:
      testgrid-dashboards: cluster-api-provider-vsphere-{{ TrimPrefix $.branch "release-" }}
      testgrid-tab-name: pr-verify-{{ ReplaceAll $.branch "." "-" }}

  - name: pull-cluster-api-provider-vsphere-test-{{ ReplaceAll $.branch "." "-" }}
    cluster: eks-prow-build-cluster
    branches:
    - ^{{ $.branch }}$
    always_run: false
    # Run if go files, scripts or configuration changed (we use the same for all jobs for simplicity).
    run_if_changed: '^((apis|config|controllers|feature|hack|packaging|pkg|test|webhooks)/|Dockerfile|go\.mod|go\.sum|main\.go|Makefile)'
    decorate: true
    decoration_config:
      timeout: 120m
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
      testgrid-tab-name: pr-test-{{ ReplaceAll $.branch "." "-" }}
      description: Runs unit tests
{{ $modes := list "govmomi" "supervisor" -}}
{{ range $i, $mode := $modes -}}
{{ $modeFocus := "" -}}
{{ if eq $mode "supervisor" }}{{ $modeFocus = "\\\\[supervisor\\\\] " }}{{ end }}
  - name: pull-cluster-api-provider-vsphere-e2e-{{ $mode }}-blocking-{{ ReplaceAll $.branch "." "-" }}
    cluster: k8s-infra-prow-build
    branches:
    - ^{{ $.branch }}$
    labels:
      preset-dind-enabled: "true"
      preset-gcve-e2e-config: "true"
      preset-kind-volume-mounts: "true"
    always_run: false
    # Run if go files, scripts or configuration changed (we use the same for all jobs for simplicity).
    run_if_changed: '^((apis|config|controllers|feature|hack|packaging|pkg|test|webhooks)/|Dockerfile|go\.mod|go\.sum|main\.go|Makefile)'
    decorate: true
    decoration_config:
      timeout: 120m
    path_alias: sigs.k8s.io/cluster-api-provider-vsphere
    max_concurrency: 3
    extra_refs:
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
          value: "{{ $modeFocus }}\\[PR-Blocking\\]"
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
      testgrid-tab-name: pr-e2e-{{ $mode }}-blocking-{{ ReplaceAll $.branch "." "-" }}
      description: Runs only PR Blocking e2e tests
  - name: pull-cluster-api-provider-vsphere-e2e-{{ $mode }}-{{ ReplaceAll $.branch "." "-" }}
    cluster: k8s-infra-prow-build
    branches:
    - ^{{ $.branch }}$
    labels:
      preset-dind-enabled: "true"
      preset-gcve-e2e-config: "true"
      preset-kind-volume-mounts: "true"
    always_run: false
    decorate: true
    decoration_config:
      timeout: 180m
    path_alias: sigs.k8s.io/cluster-api-provider-vsphere
    max_concurrency: 3
    extra_refs:
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
      testgrid-tab-name: pr-e2e-{{ $mode }}-{{ ReplaceAll $.branch "." "-" }}
      description: Runs all e2e tests
{{ if eq $mode "supervisor" -}}
{{ if not (or (eq $.branch "release-1.13") (eq $.branch "release-1.14") (eq $.branch "release-1.15")) -}}
{{ $vmOperatorVersions := list "v1.8.6-0-gde75746a" "v1.9.0-567-g93918c59" -}}
{{ range $i, $vmOperatorVersion := $vmOperatorVersions -}}
{{ $vmOperatorApiVersions := list "v1alpha2" -}}
{{ range $j, $vmOperatorApiVersion := $vmOperatorApiVersions }}
  - name: pull-cluster-api-provider-vsphere-e2e-{{ $mode }}-{{ printf "%.6s" (ReplaceAll $vmOperatorVersion "." "-") }}-{{ $vmOperatorApiVersion }}-{{ ReplaceAll $.branch "." "-" }}
    cluster: k8s-infra-prow-build
    branches:
    - ^{{ $.branch }}$
    labels:
      preset-dind-enabled: "true"
      preset-gcve-e2e-config: "true"
      preset-kind-volume-mounts: "true"
    always_run: false
    decorate: true
    decoration_config:
      timeout: 180m
    path_alias: sigs.k8s.io/cluster-api-provider-vsphere
    max_concurrency: 3
    extra_refs:
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
      testgrid-tab-name: pr-e2e-{{ $mode }}-{{ printf "%.6s" (ReplaceAll $vmOperatorVersion "." "-") }}-{{ $vmOperatorApiVersion }}-{{ ReplaceAll $.branch "." "-" }}
      description: Runs all e2e tests
{{- end }}
{{- end }}
{{- end }}
{{- end }}
  - name: pull-cluster-api-provider-vsphere-e2e-vcsim-{{ $mode }}-{{ ReplaceAll $.branch "." "-" }}
    cluster: eks-prow-build-cluster
    branches:
    - ^{{ $.branch }}$
    labels:
      preset-dind-enabled: "true"
      preset-kind-volume-mounts: "true"
    always_run: false
    # Run if go files, scripts or configuration changed (we use the same for all jobs for simplicity).
    run_if_changed: '^((apis|config|controllers|feature|hack|packaging|pkg|test|webhooks)/|Dockerfile|go\.mod|go\.sum|main\.go|Makefile)'
    decorate: true
    decoration_config:
      timeout: 180m
    path_alias: sigs.k8s.io/cluster-api-provider-vsphere
    max_concurrency: 3
    extra_refs:
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
      testgrid-tab-name: pr-e2e-vcsim-{{ $mode }}-{{ ReplaceAll $.branch "." "-" }}
      description: Runs e2e tests with vcsim / govmomi mode
  - name: pull-cluster-api-provider-vsphere-e2e-{{ $mode }}-upgrade-{{ ReplaceAll (last $.config.Upgrades).From "." "-" }}-{{ ReplaceAll (last $.config.Upgrades).To "." "-" }}-{{ ReplaceAll $.branch "." "-" }}
    cluster: k8s-infra-prow-build
    labels:
      preset-dind-enabled: "true"
      preset-gcve-e2e-config: "true"
      preset-kind-volume-mounts: "true"
    decorate: true
    decoration_config:
      timeout: 120m
    always_run: false
    branches:
    # The script this job runs is not in all branches.
    - ^{{ $.branch }}$
    path_alias: sigs.k8s.io/cluster-api-provider-vsphere
    extra_refs:
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
          value: "{{ $modeFocus }}\\[Conformance\\] \\[K8s-Upgrade\\]"
        - name: KUBERNETES_VERSION_UPGRADE_FROM
          value: "{{ index (index $.versions ((last $.config.Upgrades).From)) "k8sRelease" }}"
        - name: KUBERNETES_VERSION_UPGRADE_TO
          value: "{{ index (index $.versions ((last $.config.Upgrades).To)) "k8sRelease" }}"
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
      testgrid-tab-name: pr-e2e-{{ $mode }}-{{ ReplaceAll $.branch "." "-" }}-{{ ReplaceAll (last $.config.Upgrades).From "." "-" }}-{{ ReplaceAll (last $.config.Upgrades).To "." "-" }}
  - name: pull-cluster-api-provider-vsphere-e2e-{{ $mode }}-conformance-{{ ReplaceAll $.branch "." "-" }}
    cluster: k8s-infra-prow-build
    branches:
    - ^{{ $.branch }}$
    labels:
      preset-dind-enabled: "true"
      preset-gcve-e2e-config: "true"
      preset-kind-volume-mounts: "true"
    always_run: false
    decorate: true
    decoration_config:
      timeout: 120m
    path_alias: sigs.k8s.io/cluster-api-provider-vsphere
    max_concurrency: 3
    extra_refs:
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
      testgrid-tab-name: pr-e2e-{{ $mode }}-conformance-{{ ReplaceAll $.branch "." "-" }}
      description: Runs conformance tests for CAPV
  - name: pull-cluster-api-provider-vsphere-e2e-{{ $mode }}-conformance-ci-latest-{{ ReplaceAll $.branch "." "-" }}
    cluster: k8s-infra-prow-build
    branches:
    - ^{{ $.branch }}$
    labels:
      preset-dind-enabled: "true"
      preset-gcve-e2e-config: "true"
      preset-kind-volume-mounts: "true"
    always_run: false
    decorate: true
    decoration_config:
      timeout: 120m
    path_alias: sigs.k8s.io/cluster-api-provider-vsphere
    max_concurrency: 3
    extra_refs:
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
      testgrid-tab-name: pr-e2e-{{ $mode }}-conformance-ci-latest-{{ ReplaceAll $.branch "." "-" }}
      description: Runs conformance tests with K8S ci latest for CAPV
{{ end -}}
{{- if eq $.branch "main" }}
  - name: pull-cluster-api-provider-vsphere-janitor-main
    cluster: k8s-infra-prow-build
    labels:
      preset-dind-enabled: "true"
      preset-gcve-e2e-config: "true"
    always_run: false
    optional: true
    decorate: true
    decoration_config:
      timeout: 120m
    path_alias: sigs.k8s.io/cluster-api-provider-vsphere
    max_concurrency: 1
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
      testgrid-tab-name: pr-e2e-janitor-main
      description: Runs the janitor to cleanup orphaned objects in CI
{{ end -}}
