presubmits:
  kubernetes-sigs/cluster-api-provider-aws:
  - name: pull-cluster-api-provider-aws-test{{ if ne $.branch "main" }}-{{ ReplaceAll $.branch "." "-" }}{{ end }}
    cluster: eks-prow-build-cluster
    branches:
    - ^{{ $.branch }}$
    always_run: true
    optional: false
    decorate: true
    rerun_auth_config:
      github_team_slugs:
        - org: kubernetes-sigs
          slug: cluster-api-provider-aws-maintainers
    path_alias: "sigs.k8s.io/cluster-api-provider-aws"
    spec:
      containers:
      - image: {{ $.config.TestImage }}
        command:
        - "./scripts/ci-test.sh"
        resources:
          requests:
            cpu: "7"
            memory: "16Gi"
          limits:
            cpu: "7"
            memory: "16Gi"
    annotations:
      testgrid-dashboards: sig-cluster-lifecycle-cluster-api-provider-aws
      testgrid-tab-name: pr-test{{ if ne $.branch "main" }}-{{ ReplaceAll $.branch "." "-" }}{{ end }}
  - name: pull-cluster-api-provider-aws-apidiff-{{ ReplaceAll $.branch "." "-" }}
    cluster: eks-prow-build-cluster
    decorate: true
    rerun_auth_config:
      github_team_slugs:
        - org: kubernetes-sigs
          slug: cluster-api-provider-aws-maintainers
    path_alias: sigs.k8s.io/cluster-api-provider-aws
    always_run: true
    optional: true
    labels:
      preset-service-account: "true"
    branches:
    - ^{{ $.branch }}$
    spec:
      containers:
      - command:
        - runner.sh
        - ./scripts/ci-apidiff.sh
        image: {{ $.config.TestImage }}
        resources:
          requests:
            cpu: 7
            memory: 9Gi
          limits:
            cpu: 7
            memory: 9Gi
    annotations:
      testgrid-dashboards: sig-cluster-lifecycle-cluster-api-provider-aws
      testgrid-tab-name: pr-apidiff-{{ ReplaceAll $.branch "." "-" }}
  - name: pull-cluster-api-provider-aws-build{{ if ne $.branch "main" }}-{{ ReplaceAll $.branch "." "-" }}{{ end }}
    cluster: eks-prow-build-cluster
    always_run: true
    optional: false
    decorate: true
    path_alias: "sigs.k8s.io/cluster-api-provider-aws"
    branches:
    - ^{{ $.branch }}$
    spec:
      containers:
      - image: {{ $.config.TestImage }}
        command:
        - "./scripts/ci-build.sh"
        resources:
          requests:
            cpu: "4"
            memory: "8Gi"
          limits:
            cpu: "4"
            memory: "8Gi"
    annotations:
      testgrid-dashboards: sig-cluster-lifecycle-cluster-api-provider-aws
      testgrid-tab-name: pr-build{{ if ne $.branch "main" }}-{{ ReplaceAll $.branch "." "-" }}{{ end }}
  - name: pull-cluster-api-provider-aws-build-docker{{ if ne $.branch "main" }}-{{ ReplaceAll $.branch "." "-" }}{{ end }}
    cluster: eks-prow-build-cluster
    labels:
      preset-dind-enabled: "true"
      preset-kind-volume-mounts: "true"
    always_run: true
    optional: false
    decorate: true
    rerun_auth_config:
      github_team_slugs:
        - org: kubernetes-sigs
          slug: cluster-api-provider-aws-maintainers
    path_alias: "sigs.k8s.io/cluster-api-provider-aws"
    branches:
    - ^{{ $.branch }}$
    spec:
      containers:
      - image: {{ $.config.TestImage }}
        command:
        - runner.sh
        args:
        - ./scripts/ci-docker-build.sh
        resources:
          requests:
            cpu: "4"
            memory: "8Gi"
          limits:
            cpu: "4"
            memory: "8Gi"
        # docker-in-docker needs privileged mode
        securityContext:
          privileged: true
    annotations:
      testgrid-dashboards: sig-cluster-lifecycle-cluster-api-provider-aws
      testgrid-tab-name: pr-build-docker{{ if ne $.branch "main" }}-{{ ReplaceAll $.branch "." "-" }}{{ end }}
  - name: pull-cluster-api-provider-aws-verify{{ if ne $.branch "main" }}-{{ ReplaceAll $.branch "." "-" }}{{ end }}
    cluster: eks-prow-build-cluster
    always_run: true
    branches:
    - ^{{ $.branch }}$
    optional: false
    decorate: true
    path_alias: "sigs.k8s.io/cluster-api-provider-aws"
    spec:
      containers:
      - image: {{ $.config.TestImage }}
        command:
        - "runner.sh"
        - "make"
        - "verify"
        resources:
          requests:
            cpu: "4"
            memory: "8Gi"
          limits:
            cpu: "4"
            memory: "8Gi"
        # docker-in-docker needs privileged mode
        securityContext:
            privileged: true
    annotations:
      testgrid-dashboards: sig-cluster-lifecycle-cluster-api-provider-aws
      testgrid-tab-name: pr-verify{{ if ne $.branch "main" }}-{{ ReplaceAll $.branch "." "-" }}{{ end }}
    labels:
      preset-dind-enabled: "true"
  # conformance test
  - name: pull-cluster-api-provider-aws-e2e-conformance{{ if ne $.branch "main" }}-{{ ReplaceAll $.branch "." "-" }}{{ end }}
    cluster: eks-prow-build-cluster
    branches:
    - ^{{ $.branch }}$
    labels:
      preset-dind-enabled: "true"
      preset-kind-volume-mounts: "true"
      preset-service-account: "true"
      preset-aws-ssh: "true"
      preset-aws-credential: "true"
    path_alias: "sigs.k8s.io/cluster-api-provider-aws"
    always_run: false
    optional: true
    decorate: true
    decoration_config:
      timeout: 5h
    rerun_auth_config:
      github_team_slugs:
        - org: kubernetes-sigs
          slug: cluster-api-provider-aws-maintainers
    max_concurrency: 1
    extra_refs:
    - org: kubernetes-sigs
      repo: image-builder
      base_ref: main
      path_alias: "sigs.k8s.io/image-builder"
    - org: kubernetes
      repo: kubernetes
      base_ref: master
      path_alias: k8s.io/kubernetes
    spec:
      containers:
        - image: {{ $.config.TestImage }}
          command:
            - "runner.sh"
            - "./scripts/ci-conformance.sh"
          env:
            - name: BOSKOS_HOST
              value: "boskos.test-pods.svc.cluster.local"
            - name: AWS_REGION
              value: "us-west-2"
          # we need privileged mode in order to do docker in docker
          securityContext:
            privileged: true
          resources:
            requests:
              # these are both a bit below peak usage during build
              # this is mostly for building kubernetes
              memory: "9Gi"
              cpu: 2
            limits:
              cpu: 2
              memory: "9Gi"
    annotations:
      testgrid-dashboards: sig-cluster-lifecycle-cluster-api-provider-aws
      testgrid-tab-name: pr-conformance{{ if ne $.branch "main" }}-{{ ReplaceAll $.branch "." "-" }}{{ end }}
      testgrid-num-columns-recent: '20'
  {{- if ne $.branch "release-2.7" }}
  # conformance test against kubernetes main branch with `kind` + cluster-api-provider-aws
  - name: pull-cluster-api-provider-aws-e2e-conformance-with-ci-artifacts{{ if ne $.branch "main" }}-{{ ReplaceAll $.branch "." "-" }}{{ end }}
    cluster: eks-prow-build-cluster
    branches:
    - ^{{ $.branch }}$
    path_alias: "sigs.k8s.io/cluster-api-provider-aws"
    always_run: false
    optional: true
    decorate: true
    decoration_config:
      timeout: 5h
    rerun_auth_config:
      github_team_slugs:
        - org: kubernetes-sigs
          slug: cluster-api-provider-aws-maintainers
    max_concurrency: 1
    labels:
      preset-dind-enabled: "true"
      preset-kind-volume-mounts: "true"
      preset-service-account: "true"
      preset-aws-ssh: "true"
      preset-aws-credential: "true"
    spec:
      containers:
        - image: {{ $.config.TestImage }}
          command:
            - "runner.sh"
            - "./scripts/ci-conformance.sh"
          env:
            - name: BOSKOS_HOST
              value: "boskos.test-pods.svc.cluster.local"
            - name: AWS_REGION
              value: "us-west-2"
            - name: E2E_ARGS
              value: "-kubetest.use-ci-artifacts"
          securityContext:
            privileged: true
          resources:
            requests:
              cpu: 2
              memory: "9Gi"
            limits:
              cpu: 2
              memory: "9Gi"
    annotations:
      testgrid-dashboards: sig-cluster-lifecycle-cluster-api-provider-aws
      testgrid-tab-name: pr-conformance-{{ ReplaceAll $.branch "." "-" }}-k8s-main
      testgrid-num-columns-recent: '20'
{{- end }}
  - name: pull-cluster-api-provider-aws-e2e-blocking{{ if ne $.branch "main" }}-{{ ReplaceAll $.branch "." "-" }}{{ end }}
    cluster: eks-prow-build-cluster
    branches:
    - ^{{ $.branch }}$
    path_alias: "sigs.k8s.io/cluster-api-provider-aws"
    #run_if_changed: '^((api|bootstrap|cmd|config|controllers|controlplane|exp|feature|hack|pkg|test|util)/|main\.go|go\.mod|go\.sum|Dockerfile|Makefile)'
    always_run: {{ eq $.branch "main" }}
    optional: {{ not (eq $.branch "main") }}
    decorate: true
    decoration_config:
      timeout: {{ if has $.branch (list "release-2.8" "release-2.7") }}5h{{ else }}2h{{ end }}
    rerun_auth_config:
      github_team_slugs:
        - org: kubernetes-sigs
          slug: cluster-api-provider-aws-maintainers
    max_concurrency: {{ if has $.branch (list "release-2.8" "release-2.7") }}1{{ else }}3{{ end }}
    labels:
      preset-dind-enabled: "true"
      preset-kind-volume-mounts: "true"
      preset-service-account: "true"
      preset-aws-ssh: "true"
      preset-aws-credential: "true"
    spec:
      containers:
        - image: {{ $.config.TestImage }}
          command:
            - "runner.sh"
            - "./scripts/ci-e2e.sh"
          env:
            - name: GINKGO_FOCUS
              value: "\\[PR-Blocking\\]"
            - name: BOSKOS_HOST
              value: "boskos.test-pods.svc.cluster.local"
            - name: AWS_REGION
              value: "us-west-2"
            # Parallelize tests
            - name: GINKGO_ARGS
              value: "-nodes 20"
          securityContext:
            privileged: true
          resources:
            requests:
              cpu: 2
              memory: "9Gi"
            limits:
              cpu: 2
              memory: "9Gi"
    annotations:
      testgrid-dashboards: sig-cluster-lifecycle-cluster-api-provider-aws
      testgrid-tab-name: pr-quick-e2e-{{ ReplaceAll $.branch "." "-" }}
      testgrid-num-columns-recent: '20'
  - name: pull-cluster-api-provider-aws-e2e{{ if ne $.branch "main" }}-{{ ReplaceAll $.branch "." "-" }}{{ end }}
    cluster: eks-prow-build-cluster
    branches:
    - ^{{ $.branch }}$
    path_alias: "sigs.k8s.io/cluster-api-provider-aws"
    always_run: false
    optional: true
    decorate: true
    decoration_config:
      timeout: 5h
    rerun_auth_config:
      github_team_slugs:
        - org: kubernetes-sigs
          slug: cluster-api-provider-aws-maintainers
    max_concurrency: {{ if has $.branch (list "release-2.8" "release-2.7") }}1{{ else }}2{{ end }}
    labels:
      preset-dind-enabled: "true"
      preset-kind-volume-mounts: "true"
      preset-service-account: "true"
      preset-aws-ssh: "true"
      preset-aws-credential: "true"
    spec:
      containers:
        - image: {{ $.config.TestImage }}
          command:
            - "runner.sh"
            - "./scripts/ci-e2e.sh"
          env:
            - name: BOSKOS_HOST
              value: "boskos.test-pods.svc.cluster.local"
            - name: AWS_REGION
              value: "us-west-2"
          securityContext:
            privileged: true
          resources:
            requests:
              cpu: 2
              memory: "9Gi"
            limits:
              cpu: 2
              memory: "9Gi"
    annotations:
      testgrid-dashboards: sig-cluster-lifecycle-cluster-api-provider-aws
      testgrid-tab-name: pr-e2e-{{ ReplaceAll $.branch "." "-" }}
      testgrid-num-columns-recent: '20'
  - name: pull-cluster-api-provider-aws-e2e-eks{{ if ne $.branch "main" }}-{{ ReplaceAll $.branch "." "-" }}{{ end }}
    cluster: eks-prow-build-cluster
    branches:
    - ^{{ $.branch }}$
    path_alias: "sigs.k8s.io/cluster-api-provider-aws"
    always_run: false
    optional: true
    decorate: true
    decoration_config:
      timeout: 5h
    rerun_auth_config:
      github_team_slugs:
        - org: kubernetes-sigs
          slug: cluster-api-provider-aws-maintainers
    max_concurrency: {{ if has $.branch (list "release-2.8" "release-2.7") }}1{{ else }}2{{ end }}
    labels:
      preset-dind-enabled: "true"
      preset-kind-volume-mounts: "true"
      preset-service-account: "true"
      preset-aws-ssh: "true"
      preset-aws-credential: "true"
    spec:
      containers:
        - image: {{ $.config.TestImage }}
          command:
            - "runner.sh"
            - "./scripts/ci-e2e-eks.sh"
          env:
            - name: BOSKOS_HOST
              value: "boskos.test-pods.svc.cluster.local"
            - name: AWS_REGION
              value: "us-west-2"
          securityContext:
            privileged: true
          resources:
            requests:
              cpu: 2
              memory: "9Gi"
            limits:
              cpu: 2
              memory: "9Gi"
    annotations:
      testgrid-dashboards: sig-cluster-lifecycle-cluster-api-provider-aws
      testgrid-tab-name: pr-e2e-eks-{{ ReplaceAll $.branch "." "-" }}
      testgrid-num-columns-recent: '20'
{{- $onlyInBranch := list "release-2.7" "release-2.8" }}
{{- if has $.branch $onlyInBranch  }}
  - name: pull-cluster-api-provider-aws-e2e-eks-gc{{ if ne $.branch "main" }}-{{ ReplaceAll $.branch "." "-" }}{{ end }}
    cluster: eks-prow-build-cluster
    branches:
    - ^{{ $.branch }}$
    path_alias: "sigs.k8s.io/cluster-api-provider-aws"
    always_run: false
    optional: true
    decorate: true
    decoration_config:
      timeout: 5h
    rerun_auth_config:
      github_team_slugs:
        - org: kubernetes-sigs
          slug: cluster-api-provider-aws-maintainers
    max_concurrency: 1
    labels:
      preset-dind-enabled: "true"
      preset-kind-volume-mounts: "true"
      preset-service-account: "true"
      preset-aws-ssh: "true"
      preset-aws-credential: "true"
    spec:
      containers:
        - image: {{ $.config.TestImage }}
          command:
            - "runner.sh"
            - "./scripts/ci-e2e-eks-gc.sh"
          env:
            - name: BOSKOS_HOST
              value: "boskos.test-pods.svc.cluster.local"
            - name: AWS_REGION
              value: "us-west-2"
          securityContext:
            privileged: true
          resources:
            limits:
              cpu: 2
              memory: "9Gi"
            requests:
              cpu: 2
              memory: "9Gi"
    annotations:
      testgrid-dashboards: sig-cluster-lifecycle-cluster-api-provider-aws
      testgrid-tab-name: pr-e2e-eks-gc-{{ ReplaceAll $.branch "." "-" }}
      testgrid-num-columns-recent: '20'
{{- end }}
  - name: pull-cluster-api-provider-aws-e2e-eks-testing{{ if ne $.branch "main" }}-{{ ReplaceAll $.branch "." "-" }}{{ end }}
    cluster: eks-prow-build-cluster
    branches:
    - ^{{ $.branch }}$
    path_alias: "sigs.k8s.io/cluster-api-provider-aws"
    always_run: false
    optional: true
    decorate: true
    decoration_config:
      timeout: 5h
    rerun_auth_config:
      github_team_slugs:
        - org: kubernetes-sigs
          slug: cluster-api-provider-aws-maintainers
    max_concurrency: 1
    labels:
      preset-dind-enabled: "true"
      preset-kind-volume-mounts: "true"
      preset-service-account: "true"
      preset-aws-ssh: "true"
      preset-aws-credential: "true"
    spec:
      containers:
        - image: {{ $.config.TestImage }}
          command:
            - "runner.sh"
            - "./scripts/ci-e2e-eks.sh"
          env:
            - name: BOSKOS_HOST
              value: "boskos.test-pods.svc.cluster.local"
            - name: AWS_REGION
              value: "us-west-2"
            - name: GINKGO_ARGS
              value: "-nodes 2"
          securityContext:
            privileged: true
          resources:
            requests:
              cpu: 2
              memory: "9Gi"
            limits:
              cpu: 2
              memory: "9Gi"
    annotations:
      testgrid-dashboards: sig-cluster-lifecycle-cluster-api-provider-aws
      testgrid-tab-name: pr-e2e-eks-{{ ReplaceAll $.branch "." "-" }}-testing
      testgrid-num-columns-recent: '20'
