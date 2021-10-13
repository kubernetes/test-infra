
# Deck

## Running Deck locally

Deck can be run locally by executing `./prow/cmd/deck/runlocal`. The scripts starts Deck via
Bazel using:

* pre-generated data (extracted from a running Prow instance)
* the local `config.yaml`
* the local static files, template files and lenses

Open your browser and go to: <http://localhost:8080>

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

## Rerun Prow Job via Prow UI

Rerun prow job can be done by visiting prow UI, locate prow job and rerun job by clicking on the ↻ button then clicking `Rerun` button. For prow on github, the permission is controlled by github membership, and configured as part of deck configuration, see [`rerun_auth_configs`](https://github.com/kubernetes/test-infra/blob/0dfe42533307f9733f22d4a6abf08e1df2229fcb/config/prow/config.yaml#L92) for k8s prow.

See example below:
![Example](./rerun_button.png)

This is also available for non github prow if the frontend is secured and [`allow_anyone`](https://github.com/kubernetes/test-infra/blob/95cc9f4b68d0ce5702c3b3e009221de0fe0a482a/prow/apis/prowjobs/v1/types.go#L190-L191) is set to true for the job.
