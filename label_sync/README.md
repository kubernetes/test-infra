You can run it locally with repositories and labels data fetched into YAML files:

Run (with go):
`go run label_sync/main.go -help` (for help)
`go run label_sync/main.go -labels-path label_sync/labels.yaml -local -repos label_sync/repos.yaml -repo-labels label_sync/repo_labels.yaml`

Test (with go):
`go test k8s.io/test-infra/label_sync`
`make test`

Bazel test:
`bazel test //label_sync/...`

Bazel build:
`bazel build //label_sync/...`

Bazel run:
`./bazel-bin/label_sync/label_sync -labels-path bazel-test-infra/label_sync/labels.yaml -local -repos bazel-test-infra/label_sync/repos.yaml -repo-labels bazel-test-infra/label_sync/repo_labels.yaml`

Build static binary:
`make compile-static`

You can run on Your own repository set (by passing special `user:user_name` to `-org` flag) and save repository data in local files, and then run program in local mode using those saved files.
Assuming You have Your oauth github file in `/etc/github/oauth` or You can pass it with `-github-token-file /your/path/to/oauth/file`:
`go run label_sync/main.go -labels-path label_sync/labels.yaml -org user:your_github_user_name -dump-repos your_repos.yaml -dump-repo-labels your_labels.yaml`

It will save Your repositories and their labels in two YAML files, next time You can run/debug/test in `-local` mode:
`go run label_sync.main.go -labels-path label_sync/labels.yaml -local -repos your_repos.yaml -repo-labels your_labels.yaml`
This will allow debuging algorithms to sync labels.

Actual label updates cannot be done in `-dry-run` or `-local` mode because it updates real repository labels.
You can update list of labels from given repo by running:
`go run label_sync/main.go -debug -dry-run -labels-path label_sync/labels_example.yaml -org kubernetes -github-token-file /etc/github/oauth -repos label_sync/single_repo_example.yaml -dump-repo-labels label_sync/kubernetes_labels.yaml`
Parameters meaning:
- `-debug`: to see more debugging info while doing so
- `-dry-run`: not to try to update repo, but do actuall read calls on this repo
- `-org` and `-github-token-file`: to allow You to use GitHub API calls
- `-labels-path`: to provide required labels file to sync (it won't be used in `-dry-run` mode)
- `-repos`: to provide list of repository (even single) to get labels from (You can manually put only kubernetes/kubernetes here)
- `-dump-repo-labels`: to save them in the file

You can tweak required labels `label_sync/labels.yaml` file and Your `-repos` and `-repo-labels` files to make it try to update single label on Your own repo etc.
Real run should have all required files in default plases and should just go without parameters (`/etc/config/labels.yaml` for `-labels-path` and `/etc/github/oauth` for `-github-token-file`):
`go run label_sync/main.go`

If You want to build docker image manually and only locally, You need to build static binary for docker first:
`make compile-static`

You can specify to build docker image with (from `test-infra` root directory):
From `label_sync` directory:
`docker build -t label_sync .`
And then run docker image via:
`docker run label_sync` (eventuallu change entry point to `/bin/sh` and add `-ti` options for interactive shell)
Currently this is an equivalent to: `go run label_sync/main.go -help` from Your local development (see `Dockerfile`) because I don't know yet how to put Github OAuth secret in that image (and have no access to real GitHub OAuth token for kubernetes repo).

