Image Builder
=============

This image builder is sugar on top of `gcloud builds submit`. It offers the following features:

- Automatically injecting the standard commit-based tag (e.g. `20190403-dddd315ad-dirty`) as `_GIT_TAG`
- Optionally blocking pushes of dirty builds
- Uploading the working directory to GCS once and then reusing it for several builds
- Building multiple variants simultaneously and optionally sending their output to files

A "variant" is a group of GCB substitutions grouped together to describe several ways to build a
given image. They are optionally defined in `variants.yaml` in the same folder as the `Dockerfile`
and `cloudbuild.yaml`. For example, a subset of the `kubekins-e2e` variants looks like this:

```yaml
variants:
  '1.14':
    CONFIG: '1.14'
    GO_VERSION: 1.12.1
    K8S_RELEASE: latest
    BAZEL_VERSION: 0.21.0
  '1.13':
    CONFIG: '1.13'
    GO_VERSION: 1.11.5
    K8S_RELEASE: stable-1.13
    BAZEL_VERSION: 0.18.1
```

By default, the image builder will build both the `1.13` and `1.14` groups simultaneously.
If `--log-dir` is specified, it will write the build logs for each to `1.13.log` and `1.14.log`.

Alternatively, you can use `--variant` to build only one variant, e.g. `--variant 1.13`.

If no `variants.yaml` is specified, `cloudbuild.yaml` will be run once with no extra substitutions
beyond `_GIT_TAG`.

## Usage

```
bazel run //images/builder -- [options] path/to/image-directory/
```

* `--allow-dirty`: If true, allow pushing dirty builds.
* `--log-dir`: If provided, build logs will be sent to files in this directory instead of to stdout/stderr.
* `--project`: If specified, use a non-default GCP project.
* `--scratch-bucket`: If provided, the complete GCS path for Cloud Build to store scratch files (sources, logs). Necessary for upload reuse. If omitted, `gcloud` will create or reuse a bucket of its choosing.
* `-variant`: If specified, build only the given variant. An error if no variants are defined.
