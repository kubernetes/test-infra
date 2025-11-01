# AGENTS.md

This file provides guidance to AI assistants when working with code in this repository.

## Repository Overview

This is kubernetes/test-infra, which contains tools and configuration files for testing and automation needs of the Kubernetes project. The main components are:

- **Prow Jobs**: CI/CD job configurations at prow.k8s.io (config/jobs/)
- **TestGrid**: Test result visualization at testgrid.k8s.io (testgrid/)
- **Kettle**: Extracts test results from GCS to BigQuery (kettle/)
- **Label Sync**: Manages GitHub labels across repos (label_sync/)
- **Gopherage**: Go coverage file manipulation (gopherage/)
- **Experiment**: One-off tools and scripts (experiment/)

Note: Prow source code moved to kubernetes-sigs/prow in April 2024. This repo now contains prow job configurations and test-infra specific tooling.

## Building and Testing

### Running Tests

```bash
# Run all tests (Go + Python unit tests)
make test

# Run only Go unit tests
make go-unit

# Run only Python unit tests
make py-unit

# Test a specific Go package or folder
hack/make-rules/go-test/unit.sh <folder>
# Examples:
hack/make-rules/go-test/unit.sh kettle/...
hack/make-rules/go-test/unit.sh pkg/benchmarkjunit/...
```

### Verification and Linting

```bash
# Run all verification checks
make verify

# Run specific verifications
make go-lint           # golangci-lint checks
make verify-gofmt      # Go formatting
make verify-eslint     # TypeScript/JavaScript linting
make py-lint           # Python linting
make verify-boilerplate # License header checks
make verify-yamllint   # YAML linting
make verify-spelling   # Spell checking
make verify-labels     # GitHub label validation
make verify-file-perms # File permission checks
make verify-generated-jobs # Verify generated jobs are up to date
```

### Auto-fixing Issues

```bash
# Auto-format Go code
make update-gofmt

# Update Go dependencies (after changing go.mod)
make update-go-deps

# Auto-fix spelling mistakes
make update-spelling

# Update file permissions
make update-file-perms

# Regenerate generated job configs
make generate-jobs
```

## Dependency Management

This repo uses Go modules. Key rules:

- **NEVER add `replace` directives to go.mod** - this breaks published packages
- Run `make update-go-deps` after modifying go.mod
- Use `hack/make-rules/go-run/arbitrary.sh go <command>` instead of `go <command>` to ensure correct Go version (1.25.3)
- See docs/dep.md for complete details

## Working with Prow Jobs

Job configurations live in `config/jobs/`. The directory structure is:
- `org/repo/filename.yaml` for most repos
- `kubernetes/sig-foo/filename.yaml` for kubernetes/kubernetes jobs

### Job Types

- **Presubmits**: Run on PRs before merge
- **Postsubmits**: Run after code is merged
- **Periodics**: Run on a schedule

### Adding or Updating Jobs

1. Create/edit YAML in config/jobs/ following the org/repo structure
2. Ensure an OWNERS file exists in the directory
3. Add testgrid annotations to display results:
   ```yaml
   annotations:
     testgrid-dashboards: sig-foo-bar
     testgrid-tab-name: pull-verify
   ```
4. Open PR - changes auto-deploy when merged
5. Optionally test locally: `config/mkpj.sh` or `config/pj-on-kind.sh`

See config/jobs/README.md for comprehensive job configuration guide.

### Generated Jobs

Some jobs are auto-generated and should NOT be edited directly:

- **Image validation jobs**: Edit releng/test_config.yaml then run `./hack/update-generated-tests.sh`
- **Release branch jobs**: Use releng/config-forker to fork master jobs for new release branches

Always run `make verify-generated-jobs` before submitting PRs.

## TestGrid Configuration

TestGrid displays test results at testgrid.k8s.io. Two ways to configure:

1. **Simple (recommended)**: Add annotations to prow job YAML:
   ```yaml
   annotations:
     testgrid-dashboards: sig-testing-misc
     testgrid-tab-name: pull-verify
   ```

2. **Advanced**: Edit testgrid/config.yaml directly

See testgrid/README.md and testgrid/config.md for details.

## Image Building

```bash
# Build all misc images (local)
make build-misc-images

# Build single image (local)
make build-single-image PROW_IMAGE=<image-name>

# Push all misc images to registry
make push-misc-images REGISTRY=gcr.io/k8s-staging-test-infra

# Push single image to registry
make push-single-image PROW_IMAGE=<image-name> REGISTRY=gcr.io/k8s-staging-test-infra
```

Images are configured in .test-infra-misc-images.yaml

## Repository Structure

Key directories:

- **config/**: Prow configuration and job definitions
  - **config/jobs/**: All prow job configs (org/repo structure)
  - **config/prow/**: Prow cluster configuration
  - **config/testgrids/**: TestGrid dashboard configs
- **hack/**: Build and test scripts
  - **hack/make-rules/**: Make targets for testing/verification
  - **hack/tools/**: Tool dependencies (gotestsum, etc.)
- **releng/**: Release engineering tools
  - **releng/config-forker**: Fork jobs for new release branches
  - **releng/test_config.yaml**: Image validation job config
- **pkg/**: Shared Go packages (minimal, most moved to prow repo)
- **testgrid/**: TestGrid-specific configs and docs
- **kettle/**: BigQuery data pipeline
- **label_sync/**: GitHub label management
- **experiment/**: Ad-hoc scripts and tools

## Architecture Notes

- Prow jobs run in Kubernetes pods on the prow.k8s.io cluster
- Test results stored in GCS buckets (kubernetes-ci-logs)
- Kettle extracts GCS results → BigQuery for metrics/analysis
- TestGrid reads from GCS to display historical results
- GitHub webhooks trigger presubmit jobs on PRs
- Jobs use presets (defined in config/prow/config.yaml) for common credentials/config

## Common Workflows

### Updating Job Configs
1. Edit YAML in config/jobs/org/repo/
2. Run `make verify` to check for issues
3. Commit and open PR
4. Changes auto-deploy on merge

### Adding New Release Branch Jobs
```bash
go run ./releng/config-forker \
  --job-config $(pwd)/config/jobs \
  --version 1.27 \
  --go-version 1.20.2 \
  --output $(pwd)/config/jobs/kubernetes/sig-release/release-branch-jobs/1.27.yaml
```

### Testing Locally
```bash
# Test prow job config locally
config/pj-on-kind.sh

# Create prowjob CR from local config
config/mkpj.sh
```

## CI and PR Workflow

- All PRs checked by presubmit jobs configured in config/jobs/
- Use `/test <job-name>` to trigger specific jobs
- Use `/retest` to re-run failed jobs
- Jobs automatically deployed when PRs merge to master
- Use `/hold` to prevent auto-merge, `/hold cancel` to release
- Use `/cc @person` or `/assign @person` to notify reviewers
- See https://prow.k8s.io/command-help for all bot commands

## Contact

- SIG Testing owns this repo
- Slack: #sig-testing, #testing-ops, #prow, #testgrid
- Mailing list: kubernetes-sig-testing@googlegroups.com
