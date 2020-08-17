# Prow Autobump

The autobump image is composed of two scripts that help automate Prow component verion upgrades:
- [`bump.sh`](/prow/cmd/autobump/bump.sh) determines the version tag to upgrade to (upstream version, latest version, or a specific version) and updates all image references to the new tag.
- [`autobump.sh`](/prow/cmd/autobump/autobump.sh) wraps bump.sh with additional logic to commit and push to a fork and then create or update a PR using the [pr-creator](/robots/pr-creator).

The autobump image is designed to be run as a periodic ProwJob that maintains an open PR that proposes updating referenced Prow component images to the upstream (prow.k8s.io) version. The autobump job should be paired with a post-submit autodeploy job that applies the Prow deployment files whenever they are updated. Using both of these jobs together allows admins to upgrade to the latest upstream Prow version by just approving the autobump PR.

### Requirements
- `autobump.sh` requires a GitHub personal access token that has been granted the `repo` access scope.
- The GitHub user associated with the GitHub access token must already have a fork of the repo that that will be PRed. If the fork repo has a different name than the source repo, the name of the fork must be specified via the `FORK_GH_REPO` environment variable.
- If the container is not automatically provided with GCP credentials it may be necessary to provide a JSON service account key file via the `GOOGLE_APPLICATION_CREDENTIALS` environment variable. Any service account will work because the `gcr.io/k8s-prow/*` images are publicly readable.

## Usage/Configuration
For an explanation of the config that each script requires, see the comments in the example ProwJob or the those at the start of the scripts.

### Example ProwJob
Here is an example periodic autobump job that tries to create or update a PR every hour, bumping to the upstream Prow version. It includes comments that explain how to change the fields to make the job create autobump PRs for your Prow instance. [`example-periodic.yaml`](/prow/cmd/autobump/example-periodic.yaml)

## Building
The gcr.io/k8s-prow/autobump image is built and published automatically by [`post-test-infra-push-prow`](https://github.com/kubernetes/test-infra/blob/9a939de10fa72af415eb1e628345b7d16c1f0be0/config/jobs/kubernetes/test-infra/test-infra-trusted.yaml#L118-L143) with the rest of the Prow components.

You can build the image locally with `bazel run //prow/cmd/autobump:autobump` (note `bazel run` not `bazel build`). Publish to a remote repository after building with `docker push` or build and push all Prow images at once with [`prow/push.sh`](/prow/push.sh).
