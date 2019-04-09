# image-bumper

`image-bumper` helps update image references to the latest tag in config files. It understands k8s test-infra
image tagging conventions, and uses this to inform what it considers the latest tag.

In particular, `image-bumper` expects images in either `vYYYYMMDD-githash` form. It also accepts
`git describe` output in place of `githash`, assuming your tags look like version numbers. In this
case, your tag might look like e.g. `v1.14.0-alpha.0-4321-gac16ac7cbe`.

`image-bumper` accepts image variants indicated by suffixes on those tags. For instance,
`v20190404-928d18687-1.11` and `v20190404-928d18687-1.12` are understood to be different, and
`image-bumper` will find the latest tag for both the `-1.11` and `-1.12` suffixes, and will not
merge them together.

## Usage

```
bazel run //experiment/image-bumper -- [options] files...
```

### Options

* `--image-regex`: only touch image references matching this regex. Note that they must still be
                   hosted on *.gcr.io even if this is specigfied

### Examples

```
bazel run //experiment/image-bumper -- --image-regex gcr.io/k8s-testimages/ config/**.yaml
```

Updates every image referencing the `k8s-testimages` project in the config directory (assuming
your shell understands `**`, e.g. fish, or bash with `globstar` enabled)

```
bazel run //experiment/image-bumper -- --image-regex gcr.io/k8s-prow/ prow/**.yaml
```

Updates prow references to the latest versions.
