# kubernetes/test-infra dependency management

test-infra uses [go modules] for Go dependency management.
Usage requires [bazel].

## Usage

Run [`hack/update-deps.sh`] whenever vendored dependencies change.
This takes a minute to complete, and
longer the more uncached repos golang needs to download.


### Updating dependencies

New dependencies causes golang to recompute the minor version used for each major version of each dependency. And
golang automatically removes dependencies that nothing imports any more.

Manually increasing the version of dependencies can be done in one of three ways:
* `bazel run //:update-patch # -- example.com/foo example.com/bar`
  - update everything (or just foo and bar) to the latest patch release
  - aka `vX.Y.latest`
* `bazel run //:update-minor # -- cloud.google.com/go/storage`
  - update everything (or just storage) to the latest minor release
  - aka `vX.latest.latest`
* Manually editing `go.mod`.

Always run `hack/update-deps.sh` after changing `go.mod` by any of these methods (or adding new imports).

See golang's [go.mod] and [dependency upgrade] docs for more details.

### Tips

Use `bazel run //:go -- whatever` rather than `go whatever` from your `$PATH` to ensure the correct version of golang is selected.

Note that using this path does not otherwise require golang to be installed on your workstation.

[bazel]: https://bazel.build/
[dependency upgrade]: https://github.com/golang/go/wiki/Modules#how-to-upgrade-and-downgrade-dependencies
[go.mod]: https://github.com/golang/go/wiki/Modules#gomod
[go modules]: https://github.com/golang/go/wiki/Modules
[`hack/update-deps.sh`]: /hack/update-deps.sh
