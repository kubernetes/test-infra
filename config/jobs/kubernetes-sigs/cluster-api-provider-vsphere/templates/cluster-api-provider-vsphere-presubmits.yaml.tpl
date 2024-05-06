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
            memory: 4Gi
          requests:
            cpu: 2
            memory: 4Gi
    annotations:
      testgrid-dashboards: vmware-cluster-api-provider-vsphere, sig-cluster-lifecycle-cluster-api-provider-vsphere
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
            memory: 4Gi
          requests:
            cpu: 2
            memory: 4Gi
    annotations:
      testgrid-dashboards: vmware-cluster-api-provider-vsphere, sig-cluster-lifecycle-cluster-api-provider-vsphere
      testgrid-tab-name: pr-verify-{{ ReplaceAll $.branch "." "-" }}

  - name: pull-cluster-api-provider-vsphere-test-{{ ReplaceAll $.branch "." "-" }}
    cluster: eks-prow-build-cluster
    branches:
    - ^{{ $.branch }}$
    always_run: false
    # Run if go files, scripts or configuration changed (we use the same for all jobs for simplicity).
    run_if_changed: '^((apis|config|controllers|feature|hack|packaging|pkg|test|webhooks)/|Dockerfile|go\.mod|go\.sum|main\.go|Makefile)'
    decorate: true
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
      testgrid-tab-name: pr-test-{{ ReplaceAll $.branch "." "-" }}
      description: Runs unit tests
{{ $testInBranch := list "release-1.7" "release-1.8" "release-1.9" -}}
{{ if has $.branch $testInBranch }}
  - name: pull-cluster-api-provider-vsphere-test-integration-{{ ReplaceAll $.branch "." "-" }}
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
      testgrid-tab-name: pr-test-integration-{{ ReplaceAll $.branch "." "-" }}
      description: Runs integration tests
{{ end -}}
{{ $modes := list "govmomi" "supervisor" -}}
{{ range $i, $mode := $modes -}}
{{ $modeFocus := "" -}}
{{ if eq $mode "supervisor" }}{{ $modeFocus = "\\\\[supervisor\\\\] " }}{{ end -}}
{{/* e2e blocking for supervisor mode has been introduced with release-1.10 */ -}}
{{ $skipInBranch := list -}}
{{ if eq $mode "supervisor" }}{{ $skipInBranch = list "release-1.7" "release-1.8" "release-1.9" }}{{ end -}}
{{ if has $.branch $skipInBranch | not }}
  - name: pull-cluster-api-provider-vsphere-e2e-{{ $mode }}-blocking-{{ ReplaceAll $.branch "." "-" }}
    branches:
    - ^{{ $.branch }}$
    labels:
      preset-dind-enabled: "true"
      preset-cluster-api-provider-vsphere-e2e-config: "true"
      preset-cluster-api-provider-vsphere-gcs-creds: "true"
      preset-kind-volume-mounts: "true"
    always_run: false
    # Run if go files, scripts or configuration changed (we use the same for all jobs for simplicity).
    run_if_changed: '^((apis|config|controllers|feature|hack|packaging|pkg|test|webhooks)/|Dockerfile|go\.mod|go\.sum|main\.go|Makefile)'
    decorate: true
    path_alias: sigs.k8s.io/cluster-api-provider-vsphere
    max_concurrency: 3
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
    annotations:
      testgrid-dashboards: vmware-cluster-api-provider-vsphere, sig-cluster-lifecycle-cluster-api-provider-vsphere
      testgrid-tab-name: pr-e2e-{{ $mode }}-blocking-{{ ReplaceAll $.branch "." "-" }}
      description: Runs only PR Blocking e2e tests
{{ end -}}
{{/* e2e full for supervisor mode has been introduced with release-1.10 */ -}}
{{ $skipInBranch = list -}}
{{ if eq $mode "supervisor" }}{{ $skipInBranch = list "release-1.7" "release-1.8" "release-1.9" }}{{ end -}}
{{ if has $.branch $skipInBranch | not }}
  - name: pull-cluster-api-provider-vsphere-e2e-{{ $mode }}-{{ ReplaceAll $.branch "." "-" }}
    branches:
    - ^{{ $.branch }}$
    labels:
      preset-dind-enabled: "true"
      preset-cluster-api-provider-vsphere-e2e-config: "true"
      preset-cluster-api-provider-vsphere-gcs-creds: "true"
      preset-kind-volume-mounts: "true"
    always_run: false
    decorate: true
    decoration_config:
      timeout: 180m
    path_alias: sigs.k8s.io/cluster-api-provider-vsphere
    max_concurrency: 3
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
    annotations:
      testgrid-dashboards: vmware-cluster-api-provider-vsphere, sig-cluster-lifecycle-cluster-api-provider-vsphere
      testgrid-tab-name: pr-e2e-{{ $mode }}-{{ ReplaceAll $.branch "." "-" }}
      description: Runs all e2e tests
{{ end -}}
{{/* e2e with vcsim has been introduced with release-1.10 */ -}}
{{ $skipInBranch = list "release-1.7" "release-1.8" "release-1.9" -}}
{{ if has $.branch $skipInBranch | not }}
  - name: pull-cluster-api-provider-vsphere-e2e-vcsim-{{ $mode }}-{{ ReplaceAll $.branch "." "-" }}
    cluster: eks-prow-build-cluster
    branches:
    - ^{{ $.branch }}$
    labels:
      preset-dind-enabled: "true"
      preset-kind-volume-mounts: "true"
    always_run: false
    decorate: true
    decoration_config:
      timeout: 180m
    path_alias: sigs.k8s.io/cluster-api-provider-vsphere
    max_concurrency: 3
    spec:
      containers:
      - image: {{ $.config.TestImage }}
        command:
        - runner.sh
        args:
        - ./hack/e2e.sh
        env:
        - name: GINKGO_FOCUS
          value: "{{ $modeFocus }}\\[vcsim\\]"
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
      testgrid-dashboards: vmware-cluster-api-provider-vsphere, sig-cluster-lifecycle-cluster-api-provider-vsphere
      testgrid-tab-name: pr-e2e-vcsim-{{ $mode }}-{{ ReplaceAll $.branch "." "-" }}
      description: Runs e2e tests with vcsim / govmomi mode
{{ end -}}
{{/* e2e upgrade has been introduced in release-1.9 */ -}}
{{/* e2e upgrade in supervisor mode has been introduced in release-1.10 */ -}}
{{ $skipInBranch = list "release-1.7" "release-1.8" -}}
{{ if eq $mode "supervisor" }}{{ $skipInBranch = list "release-1.7" "release-1.8" "release-1.9" }}{{ end -}}
{{ if has $.branch $skipInBranch | not }}
  - name: pull-cluster-api-provider-vsphere-e2e-{{ $mode }}-upgrade-{{ ReplaceAll (last $.config.Upgrades).From "." "-" }}-{{ ReplaceAll (last $.config.Upgrades).To "." "-" }}-{{ ReplaceAll $.branch "." "-" }}
    labels:
      preset-dind-enabled: "true"
      preset-cluster-api-provider-vsphere-e2e-config: "true"
      preset-cluster-api-provider-vsphere-gcs-creds: "true"
      preset-kind-volume-mounts: "true"
    decorate: true
    always_run: false
    branches:
    # The script this job runs is not in all branches.
    - ^{{ $.branch }}$
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
    annotations:
      testgrid-dashboards: vmware-cluster-api-provider-vsphere, sig-cluster-lifecycle-cluster-api-provider-vsphere
      testgrid-tab-name: pr-e2e-{{ $mode }}-{{ ReplaceAll $.branch "." "-" }}-{{ ReplaceAll (last $.config.Upgrades).From "." "-" }}-{{ ReplaceAll (last $.config.Upgrades).To "." "-" }}
{{ end -}}
{{/* e2e conformance with supervisor mode has been introduced with release-1.10 */ -}}
{{ $skipInBranch = list -}}
{{ if eq $mode "supervisor" }}{{ $skipInBranch = list "release-1.7" "release-1.8" "release-1.9" }}{{ end -}}
{{ if has $.branch $skipInBranch | not }}
  - name: pull-cluster-api-provider-vsphere-e2e-{{ $mode }}-conformance-{{ ReplaceAll $.branch "." "-" }}
    branches:
    - ^{{ $.branch }}$
    labels:
      preset-dind-enabled: "true"
      preset-cluster-api-provider-vsphere-e2e-config: "true"
      preset-cluster-api-provider-vsphere-gcs-creds: "true"
      preset-kind-volume-mounts: "true"
    always_run: false
    decorate: true
    path_alias: sigs.k8s.io/cluster-api-provider-vsphere
    max_concurrency: 3
    spec:
      containers:
      - image: {{ $.config.TestImage }}
        command:
        - runner.sh
        args:
        - ./hack/e2e.sh
        env:
        - name: GINKGO_FOCUS
{{- if eq $.branch "release-1.7" "release-1.8" }}
          value: "{{ $modeFocus }}\\[Conformance\\]"
{{- else }}
          value: "{{ $modeFocus }}\\[Conformance\\] \\[K8s-Install\\]"
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
      testgrid-tab-name: pr-e2e-{{ $mode }}-conformance-{{ ReplaceAll $.branch "." "-" }}
      description: Runs conformance tests for CAPV
{{ end -}}
{{/* e2e conformance-ci-latest has been introduced with release-1.9 */ -}}
{{/* e2e conformance-ci-latest with supervisor mode has been introduced with release-1.10 */ -}}
{{ $skipInBranch = list "release-1.7" "release-1.8" -}}
{{ if eq $mode "supervisor" }}{{ $skipInBranch = list "release-1.7" "release-1.8" "release-1.9" }}{{ end -}}
{{ if has $.branch $skipInBranch | not }}
  - name: pull-cluster-api-provider-vsphere-e2e-{{ $mode }}-conformance-ci-latest-{{ ReplaceAll $.branch "." "-" }}
    branches:
    - ^{{ $.branch }}$
    labels:
      preset-dind-enabled: "true"
      preset-cluster-api-provider-vsphere-e2e-config: "true"
      preset-cluster-api-provider-vsphere-gcs-creds: "true"
      preset-kind-volume-mounts: "true"
    always_run: false
    decorate: true
    path_alias: sigs.k8s.io/cluster-api-provider-vsphere
    max_concurrency: 3
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
    annotations:
      testgrid-dashboards: vmware-cluster-api-provider-vsphere, sig-cluster-lifecycle-cluster-api-provider-vsphere
      testgrid-tab-name: pr-e2e-{{ $mode }}-conformance-ci-latest-{{ ReplaceAll $.branch "." "-" }}
      description: Runs conformance tests with K8S ci latest for CAPV
{{ end -}}
{{ end }}