# Kubernetes Contrib

[![Build Status](https://travis-ci.org/kubernetes/contrib.svg)](https://travis-ci.org/kubernetes/contrib)
[![Go Report Card](https://goreportcard.com/badge/github.com/kubernetes/contrib)](https://goreportcard.com/report/github.com/kubernetes/contrib)
[![APACHEv2 License](https://img.shields.io/badge/license-APACHEv2-blue.svg)](https://github.com/kubernetes/contrib/blob/master/LICENSE)

**Do not add new projects to this repository.** We eventually want to
move all code in this repository to more appropriate repositories (see 
[#762](https://github.com/kubernetes/contrib/issues/762)). Create a new
repository in `kubernetes-incubator` instead 
([process](https://github.com/kubernetes/community/blob/master/incubator.md)).

## Getting the Code

The code must be checked out as a subdirectory of `k8s.io`, and not `github.com`.

```shell
mkdir -p $GOPATH/src/k8s.io
cd $GOPATH/src/k8s.io
# Replace "$YOUR_GITHUB_USERNAME" below with your github username
git clone https://github.com/$YOUR_GITHUB_USERNAME/contrib.git
cd contrib
```

## Updating Godeps

Godeps in contrib/ has a different layout than in kubernetes/ proper. This is because
contrib contains multiple tiny projects, each with their own dependencies. Each
in contrib/ has it's own Godeps.json. For example the Godeps.json for Ingress
is Ingress/Godeps/Godeps.json. This means that godeps commands like `godep restore`
or `godep test` do not work in the root directory. They should be run from inside the
subproject directory you want to test.

## Prerequisites for updating Godeps

Since we vendor godeps through `/vendor` vs the old style `Godeps/_workspace`, you either need a more recent install of go and godeps, or you need to set `GO15VENDOREXPERIMENT=1`. Eg:
```shell
$ godep version
godep v74 (linux/amd64/go1.6.1)
$ go version
go version go1.6.1 linux/amd64
$ godep save ./...
```

Will automatically save godeps to `vendor/` instead of `_workspace/`.
If you have an older version of go, you must run:
```shell
$ GO15VENDOREXPERIMENT=1 godep save ./...
```

If you have an older version of godep, you must update it:
```shell
$ go get github.com/tools/godep
$ cd $GOPATH/src/github.com/tools/godep
$ go build -o godep *.go
```

## Updating Godeps

The most common dep to update is obviously going to be kubernetes proper. Updating
kubernetes and it's dependancies in the Ingress subproject for example can be done
as follows (the example assumes your Kubernetes repo is rooted at `$GOPATH/src/github.com/kubernetes`, `s/github.com\/kubernetes/k8s.io/` as required):
```shell
cd $GOPATH/src/github.com/kubernetes/contrib/ingress
godep restore
go get -u github.com/kubernetes/kubernetes
cd $GOPATH/src/github.com/kubernetes/kubernetes
godep restore
cd $GOPATH/src/github/kubernetes/contrib/ingress
rm -rf Godeps
godep save ./...
git [add/remove] as needed
git commit
```

Other deps are similar, although if the dep you wish to update is included from
kubernetes we probably want to stay in sync using the above method. If the dep is not in kubernetes proper something like the following should get you a nice clean result:
```shell
cd $GOPATH/src/github/kubernetes/contrib/ingress
godep restore
go get -u $SOME_DEP
rm -rf Godeps
godep save ./...
git [add/remove] as needed
git commit
```

## Running all tests

To run all go test in all projects do this:
```shell
./hack/for-go-proj.sh test
```

## Getting PRs Merged Into Contrib

In order for your PR to get merged, it must have the both `lgtm` AND `approved` labels.  When you open a PR, the k8s-merge-bot will automatically assign a reviewer from the `OWNERS` files.  Once assigned, the reviewer can then comment `/lgtm`, which will add the `lgtm` label, or if he/she has permission, the reviewer can add the label directly.

Each file modified in the PR will also need to be approved by an approver from its `OWNERS` file or an approver in a parent directory's `OWNERS` file.  A file is approved when the approver comments `/approve`, and it is unapproved if an approver comments `/approve cancel`.  When all files have been approved, the `approved` label will automatically be added by the k8s-merge-bot and the PR will be added to the submit-queue to be merged.
