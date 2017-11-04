# Kubernetes Contrib

[![Build Status](https://travis-ci.org/kubernetes/contrib.svg)](https://travis-ci.org/kubernetes/contrib)

This is a place for various components in the Kubernetes ecosystem
that aren't part of the Kubernetes core.

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
or `godep test` work in the root directory. Theys should be run from inside the
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

The most common dep to update is obviously going to be kuberetes proper. Updating
kubernetes and it's dependancies in the Ingress subproject for example can be done
as follows (the example assumes you Kubernetes repo is rooted at `$GOPATH/src/github.com/kubernetes`, `s/github.com\/kubernetes/k8s.io/` as required):
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
