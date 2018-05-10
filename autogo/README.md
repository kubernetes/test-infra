# Automatically generate bazel rules for go packages

This uses [gazelle] to generate and update [bazel] rules for golang packages without
adding new `BUILD.bazel` files to the repository.

## Usage

See options for [installing bazel].

```
# Install bazel
brew install bazel # See https://docs.bazel.build/versions/master/install.html

# Create a bazel WORKSPACE file in your repo root
echo >WORKSPACE <<END
git_repository(
  name = "fejta_autogo",
  remote = "https://github.com/fejta/test-infra.git",
  commit = "f478925cc6179f1abf6245698aaf514d873cfcc9",
)
load("@fejta_autogo//autogo:deps.bzl", "autogo_dependencies")
autogo_dependencies()
load("@fejta_autogo//autogo:def.bzl", "autogo_generate")
autogo_generate(
    name = "autogo",
    prefix = "github.com/golang/dep", # change to your go get path
)
END

# Create an empty BUILD.bazel file (needed by bazel)
touch BUILD.bazel

# Use bazel with an @autogo prefix to access the auto-generated repo
bazel query @autogo//...
bazel run @autogo//path/to/my/cmd/binary
```


## Demo

Add bazel support to [dep]:

```
git clone https://github.com/fejta/dep  # golang/dep + a WORKSPACE file
cd dep && ls WORKSPACE
bazel run @autogo//cmd/dep -- help
```

See the [concrete] `WORKSPACE` that enables this.

[bazel]: https://bazel.build
[concrete]: https://github.com/fejta/dep/blob/master/WORKSPACE
[dep]: http://github.com/golang/dep
[gazelle]: https://github.com/bazelbuild/bazel-gazelle
[installing bazel]: https://docs.bazel.build/versions/master/install.html
