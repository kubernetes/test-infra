# GCB Builder

This builder is sugar on top of `gcloud builds submit`. It offers the following features:

- Automatically injecting the standard commit-based tag (e.g. `20190403-dddd315ad-dirty`) as `_GIT_TAG`
- Optionally blocking pushes of dirty builds
- Uploading the working directory to GCS once and then reusing it for several builds
- Building multiple variants simultaneously and optionally sending their output to files

A "variant" is a group of GCB substitutions grouped together to describe several ways to build a
given image. They are optionally defined in `variants.yaml` in the same folder as the `Dockerfile`
and `cloudbuild.yaml`. For example, a subset of the `kubekins-e2e` variants looks like this:

```yaml
variants:
  '1.16':
    CONFIG: '1.16'
    GO_VERSION: 1.12.12
    K8S_RELEASE: stable-1.16
    BAZEL_VERSION: 0.23.2
  '1.15':
    CONFIG: '1.15'
    GO_VERSION: 1.12.12
    K8S_RELEASE: stable-1.15
    BAZEL_VERSION: 0.23.2
```

By default, the image builder will build both the `1.15` and `1.16` groups simultaneously.
If `--log-dir` is specified, it will write the build logs for each to `1.15.log` and `1.16.log`.

Alternatively, you can use `--variant` to build only one variant, e.g. `--variant 1.15`.

If no `variants.yaml` is specified, `cloudbuild.yaml` will be run once with no extra substitutions
beyond `_GIT_TAG`.

## Usage

```shell
bazel run //images/builder -- [options] path/to/build-directory/
```

- `--allow-dirty`: If true, allow pushing dirty builds.
- `--log-dir`: If provided, build logs will be sent to files in this directory instead of to stdout/stderr.
- `--project`: If specified, use a non-default GCP project.
- `--scratch-bucket`: If provided, the complete GCS path for Cloud Build to store scratch files (sources, logs). Necessary for upload reuse. If omitted, `gcloud` will create or reuse a bucket of its choosing.
- `--variant`: If specified, build only the given variant. An error if no variants are defined.
- `--env-passthrough`: Comma-separated list of specified environment variables to be passed to GCB as substitutions with an underscore (`_`) prefix. If the variable doesn't exist, the substitution will exist but be empty.
- `--build-dir`: If provided, this directory will be uploaded as the source for the Google Cloud Build run.
- `--gcb-config`: If provided, this will be used as the name of the Google Cloud Build config file.
- `--no-source`: If true, no source will be uploaded with this build.
