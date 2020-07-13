
# Deck

## Running Deck locally

Deck can be run locally by executing `./runlocal`. The scripts starts Deck via 
Bazel using:
* pre-generated data (extracted from a running Prow instance)
* the local `config.yaml`
* the local static files, template files and lenses


## Debugging via Intellij / VSCode

This section describes how to debug Deck locally by running it inside 
VSCode or Intellij.

```bash
TEST_INFRA_DIR=${GOPATH}/src/k8s.io/test-infra

# Prepare assets
cd ${TEST_INFRA_DIR}
bazel build //prow/cmd/deck:image.tar
mkdir -p /tmp/deck
tar -xvf ./bazel-bin/prow/cmd/deck/asset-base-layer.tar -C /tmp/deck 
tar -xvf ./bazel-bin/prow/cmd/deck/spyglass-lenses-layer.tar -C /tmp/deck

# Start Deck via go or in your IDE with the following arguments:
--config-path=${TEST_INFRA_DIR}/config/prow/config.yaml
--job-config-path=${TEST_INFRA_DIR}/config/jobs
--hook-url=http://prow.k8s.io
--spyglass
--template-files-location=/tmp/deck/template
--static-files-location=/tmp/deck/static
--spyglass-files-location=/tmp/deck/lenses
```
