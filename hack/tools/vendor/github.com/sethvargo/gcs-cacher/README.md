# GCS Cacher

[![GoDoc](https://img.shields.io/badge/go-documentation-blue.svg?style=flat-square)](https://pkg.go.dev/mod/github.com/sethvargo/gcs-cacher)
[![GitHub Actions](https://img.shields.io/github/workflow/status/sethvargo/gcs-cacher/Test?style=flat-square)](https://github.com/sethvargo/gcs-cacher/actions?query=workflow%3ATest)

GCS Cacher is a small CLI and Docker container that saves and restores caches on
[Google Cloud Storage][gcs]. It is intended to be used in CI/CD systems like
[Cloud Build][gcb], but may have applications elsewhere.


## Usage

1.  [Create a new Cloud Storage bucket][create-bucket]. Alternatively, you can
    use an existing Cloud Storage bucket. To automatically clean up the cache
    after a certain period of time, set a [lifecycle policy][lifecycle-policy].

1.  Create a cache:

    ```shell
    gcs-cacher -bucket "my-bucket" -cache "go-mod" -dir "$GOPATH/pkg/mod"
    ```

    This will compress and upload the contents at `pkg/mod` to Google Cloud
    Storage at the key "go-mod".

1.  Restore a cache:

    ```shell
    gcs-cacher -bucket "my-bucket" -restore "go-mod" -dir "$GOPATH/pkg/mod"
    ```

    This will download the Google Cloud Storage object named "go-mod" and
    decompress it to `pkg/mod`.


## Installation

Choose from one of the following:

-   Download the latest version from the [releases][releases].

-   Use a pre-built Docker container:

    ```text
    us-docker.pkg.dev/vargolabs/gcs-cacher/gcs-cacher
    docker.pkg.github.com/sethvargo/gcs-cacher/gcs-cacher
    ```


## Implementation

When saving the cache, the provided directory is made into a tarball, then
gzipped, then uploaded to Google Cloud Storage. When restoring the cache, the
reverse happens.

It's strongly recommend that you use a cache key based on your dependency file,
and restore up the chain. For example:

```shell
gcs-cacher \
  -bucket "my-bucket" \
  -cache "ruby-{{ hashGlob "Gemfile.lock" }}"
```

```shell
gcs-cacher \
  -bucket "my-bucket" \
  -restore "ruby-{{ hashGlob "Gemfile.lock" }}"
  -restore "ruby-"
```

This will maximize cache hits.

**It is strongly recommended that you enable a lifecycle rule on your cache
bucket!** This will automatically purge stale entities and keep costs lower.


## Why?

The primary use case is to cache large and/or expensive dependency trees like a
Ruby vendor directory or a Go module cache as part of a CI/CD step. Downloading
a compressed, packaged archive is often much faster than a full dependency
resolution. It has an unintended benefit of also reducing dependencies on
external build systems.

**Why not just use gsutil?**<br>
That's a great question. In fact, there's already a [cloud builder][builder]
that uses `gsutil` to accomplish similar things. However, that approach has a
few drawbacks:

1.  It doesn't work with large files because containers don't package the crc
    package. If you're cache is > 500mb it will fail. GCS Cacher does not have
    this same limitation.

1.  You have to build, publish, and manage the container to your own project. We
    publish pre-compiled binaries and Docker containers from multiple
    registries. You're still free to build it yourself, but you don't have to.

1.  The container image itself is **huge**. It's nearly 1GB in size. The
    gcs-cacher container is just a few MBs. Since we're optimzing for build
    speed, container size is important.

1.  It's actually really hard to get the fallback key logic correct in bash.
    There are some subtle edge cases (like when your filename contains a `$`)
    where this approach completely fails.

1.  Despite supporting parallel uploads, that cacher is still ~3.2x slower than
    GCS Cacher.


[gcs]: https://cloud.google.com/storage
[gcb]: https://cloud.google.com/cloud-build
[releases]: releases
[builder]: https://github.com/GoogleCloudPlatform/cloud-builders-community/tree/master/cache
[create-bucket]: https://cloud.google.com/storage/docs/creating-buckets
[lifecycle-policy]: https://cloud.google.com/storage/docs/lifecycle#delete
