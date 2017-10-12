You can run it locally with repositories and labels data fetched into YAML files:

## Run (with go):
`go run label_sync/main.go -help` (for help including parameter usgage and
descriptions)

`go run label_sync/main.go -labels-path label_sync/labels.yaml -local -repos label_sync/repos.yaml -repo-labels label_sync/repo_labels.yaml`

## Parameters meaning:
- `-debug`: to see more debugging info while doing so
- `-dry-run`: Actual read calls on this repo(s), but do not actually write any
  new labels
- `-org` and `-github-token-file`: to allow You to use GitHub API calls
- `-labels-path`: to provide required labels file to sync (it won't be used in `-dry-run` mode)
- `-repos`: to provide list of repository (even single) to get labels from (You can manually put only kubernetes/kubernetes here)
- `-dump-repo-labels`: a yaml file where github repository labels will be stored

## Test (with go):
`go test k8s.io/test-infra/label_sync`

`make test`

## Bazel test:
`bazel test //label_sync/...`

## Bazel build:
`bazel build //label_sync/...`

## Bazel run:
`./bazel-bin/label_sync/label_sync -labels-path bazel-test-infra/label_sync/labels.yaml -local -repos bazel-test-infra/label_sync/repos.yaml -repo-labels bazel-test-infra/label_sync/repo_labels.yaml`

## Build static binary:
`make compile-static`

## Dry Runs and Testing Against Your Own Repos
You can run on your own repository set (by passing special `user:user_name` to `-org` flag) 
and save repository data in local files, and then run program in local mode using those saved files.

Your should either put your oauth github file:
  - `/etc/github/oauth` 
  - or you can pass it with `-github-token-file=/your/path/to/oauth/file`:

For example: 

> `go run label_sync/main.go -labels-path label_sync/labels.yaml -org user:your_github_user_name -dump-repos your_repos.yaml -dump-repo-labels your_labels.yaml`

will save your repositories to a yaml filed called `your_repos.yaml` and
each repository's labels in a file called `your_labels.yaml`. 

Next time, you can run/debug/test in `-local` mode:

> `go run label_sync.main.go -labels-path label_sync/labels.yaml -local -repos your_repos.yaml -repo-labels your_labels.yaml`

This will allow debuging algorithms to sync labels.

*Note* Label updates cannot be done in `-dry-run` or `-local` mode.

You can tweak required labels `label_sync/labels.yaml` file and Your `-repos` and `-repo-labels` files to make it try to update single label on Your own repo etc.
Real run should have all required files in default plases and should just go without parameters (`/etc/config/labels.yaml` for `-labels-path` and `/etc/github/oauth` for `-github-token-file`):
`go run label_sync/main.go`

If You want to build docker image manually and only locally, You need to build static binary for docker first:
`make compile-static`

You can specify to build docker image with (from `test-infra` root directory):
From `label_sync` directory:
`docker build -t label_sync:<version no>.`

## Running and Deploying With K8s
There are two ways to run the docker image on K8s, with a cronjob and with a
deployment.  Both config files are available in the cluster directory and can be
created deployed to a cluster with

> `kubectl create -f <filename.yaml>`
