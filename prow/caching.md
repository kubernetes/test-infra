# Caching dependencies in Prow

Caching dependencies in prow works only when the [Pod Utilities][0] are enabled. A
valid Object Storage Bucket has to be configured via `gcs_configuration` either
in the Prow jobs `decoration_config` or in [Planks][1]
`default_decoration_configs`.

[0]: /prow/pod-utilities.md
[1]: /prow/cmd/plank/README.md

Saving and restoring the cache will be done in the `clonerefs` and `sidecar` pod
utility containers, where:

- `clonerefs` tries to restore the cache after the clone has been done. Multiple
  `keys` can be specified to reference files inside the repository. Those files
  will be hashed and serve as unique identifier for the cache. If no cache is
  available remotely, then the restore will not fail. The restore can only fail
  if a cache is specified but not accessible due to permissions or network
  issues.

- `sidecar` tries to save the cache after the main test container has been
  finished successfully. The specified `keys` will be used here as well to
  calculate the target directory. If the cache is already available remotely,
  then this step will be skipped.

We store the cache in Prows global root directory of the Bucket (`/prow/cache`),
because the cache should be available between multiple Prow runs and even
between different Prow jobs.

A single cache entry can be defined in the `decoration_config` via:

```yaml
decoration_config:
  caches:
    - path: /gocache
      keys:
        - go.sum
        - version-marker.txt
      download_only: true
```

Please ensure that the `path` has to be unique because it will be used as
normalized reference for the internally used volumes and mounts. Another point
worth mentioning is that the `path` has to be absolute to be mount-able.

If `download_only` is true, then it will not upload the cache if the test
process succeeded and no cache is already available.

The `keys` contain repository-relative paths, where a SHA256 is calculated for
each of the specified files. The complete cache path on the remote bucket will
then be assembled like this:

```
/prow/cache/{{ normalized path }}-{{ sha256 keys[0] }}-{{ sha256 keys[1] }}.tar.gz
```

This means that we indirectly invalidate the cache if its `path` or any of the
`keys`, respectively their SHA256 values, change.
