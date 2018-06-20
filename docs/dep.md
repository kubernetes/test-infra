# kubernetes/test-infra dependency management

test-infra uses [`dep`] for Go dependency
management. Usage requires [bazel], which can be accessed
through [`planter`] if not locally installed.

## Usage

Run [`hack/update-deps.sh`] whenever vendored dependencies change. This will
take around 5m to complete.

### Advanced usage

* [add] - Run `hack/update-deps.sh --add cloud.google.com/go/storage@v0.17.0 [...]` to pin a dependency at a particular version
* [update] - Run `hack/update-deps.sh --update golang.org/x/net [...]` to update an existing dependency
* [remove] - Edit `Gopkg.toml` and run `hack/update-deps.sh` to remove or unpin dependencies

## Tips

If `dep ensure` doesn't come back and freezes, please make sure `hg` command is
installed on your environment. `dep ensure` requires `hg` command for getting
bitbucket.org/ww/goautoneg , but `dep ensure` doesn't output such error message
and just [freezes].

[add]: https://golang.github.io/dep/docs/daily-dep.html#adding-a-new-dependency
[bazel]: https://bazel.build/
[`dep`]: https://github.com/golang/dep
[freezes]: https://github.com/kubernetes/test-infra/issues/5987
[`hack/update-deps.sh`]: /hack/update-deps.sh
[`planter`]: /planter
[remove]: https://github.com/golang/dep/blob/master/docs/daily-dep.md#rule-changes-in-gopkgtoml
[update]: https://golang.github.io/dep/docs/daily-dep.html#updating-dependencies
