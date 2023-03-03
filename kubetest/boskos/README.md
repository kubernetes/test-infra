# Boskos client package

**If you are building something outside test-infra that relies on the Boskos client library, do not use this package! Use `sigs.k8s.io/boskos` directly instead.**

This package has been copied across from https://github.com/kubernetes-sigs/boskos to avoid a dependency cycle between this repository and the Boskos repository.
This is a temporary stop-gap measure until Boskos has moved it's `k8s.io/client-go` dependency off of the old `v11` version.

More details on this problem can be read in the issue [#20421](https://github.com/kubernetes/test-infra/issues/20421).

Once Boskos no longer requires client-go@v11, we can delete this whole directory and once again depend directly on `sigs.k8s.io/boskos/*`.
