# kubernetes/test-infra dependency management

test-infra uses [go modules] for Go dependency management.

## Usage

Run [`make update-go-deps`] whenever vendored dependencies change.

### Updating dependencies

New dependencies causes golang to recompute the minor version used for each major version of each dependency. And
golang automatically removes dependencies that nothing imports any more.

See golang's [go.mod] and [dependency upgrade] docs for more details.

### No `replace` directives!

Please DO NOT add any "replace" directives to go.mod files in this repo.
This is problematic for published packages, specifically preventing the use of `go install` or `go get` to 
use package fom this repo. See [this comment](https://github.com/golang/go/issues/44840#issuecomment-1651863470)
for a complete explanation.

### Tips

Use `hack/make-rules/go-run/arbitrary.sh whatever` rather than `go whatever` from your `$PATH` to ensure the correct version of golang is selected.

Note that using this path does not otherwise require golang to be installed on your workstation.

[dependency upgrade]: https://github.com/golang/go/wiki/Modules#how-to-upgrade-and-downgrade-dependencies
[go.mod]: https://github.com/golang/go/wiki/Modules#gomod
[go modules]: https://github.com/golang/go/wiki/Modules
