# Prow Autobump

// TODO: scripts vs image?, organize headings

// TODO: finish
The prow-autobump image is composed of two scripts that help automate Prow component verion upgrades:
- [`bump.sh`]()
- [`autobump.sh`]()
		 [pr-creator]()
The prow-autobump image is designed to be run as a periodic ProwJob that pairs with a post-submit PJ.

## Requirements
- `autobump.sh` requires a GitHub personal access token that has been granted the `repo` access scope.
- The GitHub user associated with the GitHub access token must already have a fork of the repo that that will be PRed. If the fork repo has a different name than the source repo, the name of the fork must be specified via the `FORK_GH_REPO` environment variable.
- If the container is not automatically provided with GCP credentials it may be necessary to provide a JSON service account key file via the `GOOGLE_APPLICATION_CREDENTIALS` environment variable. Any service account will work because the `gcr.io/k8s-prow/*` images are publicly readable.

## Usage/Configuration
For an explanation of the config that each script requires, see the comments in the example ProwJob or the those at the start of the scripts.

## Example ProwJob
Here is an example periodic autobump job that tries to create or update a PR every hour, bumping to the upstream Prow version. It includes comments that explain how to change the fields to make the job create autobump PRs for your Prow instance. [`example-periodic.yaml`](experiment/prow-autobump/example-periodic.yaml)

## Building
// TODO: finish
The gcr.io/k8s-prow/prow-autobump image is built and published automatically by XXXXXXXX whenever there are changes to XXXXXXXX.
You can build the image locally with either
- Bazel: `bazel run //experiment/prow-autobump:prow-autobump` (note `bazel run` not `bazel build`). Publish with `docker push` or the `//prow:release-push` bazel rule.
- Cloud Build: From the repo root run `cloud-build-local --config=experiment/prow-autobump/cloudbuild.yaml --dryrun=false .` Include `--push` to publish the image to your configured GCP project's container registry.
