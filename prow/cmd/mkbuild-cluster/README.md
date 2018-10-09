# MkBuild-Cluster 

The `mkbuild-cluster` program helps create `cluster.yaml` files that [plank] accepts via the `--build-cluster` flag.

This allows prow to run jobs in different clusters than the one where [plank] runs.

See the [getting started] guide for general info about how to configure jobs that target these clusters.

## Usage

Create a new `cluster.yaml` to send to [plank] via `--build-cluster`:

```sh
# Create initial entry
bazel run //prow/cmd/mkbuild-cluster -- \
  --project=P --zone=Z --cluster=C --alias=default > cluster.yaml
# Write secret with this entry
kubectl create secret generic build-cluster --from-file=cluster.yaml
```

Now update plank to mount this secret in the container and use the `--build-cluster` flag:

```yaml
spec:
  containers:
  - name: plank
    args:
    - --build-cluster=/etc/cluster/cluster.yaml
    volumeMounts:
    - mountPath: /etc/cluster
      name: cluster
      readOnly: true
  volumes:
  - name: cluster
    secret:
      defaultMode: 420
      secretName: build-cluster
```
Note: restart plank to see the `--build-cluster` flag.

Append additional entries to `cluster.yaml`:

```sh
# Get current values:
kubectl get secrets/build-cluster -o yaml > ~/old.yaml
# Add new value
cat ~/old.yaml | bazel run //prow/cmd/mkbuild-cluster -- \
  --project=P --zone=Z --cluster=C --alias=NEW_CLUSTER \
  > ~/updated.yaml
diff ~/old.yaml ~/updated.yaml
kubectl apply -f ~/updated.yaml
```

Note: restart plank to see the updated values.

## More options:

```sh
# Full list of flags like --account, --print-entry, --get-client-cert, etc.
bazel run //prow/cmd/mkbulid-cluster -- --help
```


[getting started]: /prow/getting_started.md
[plank]: /prow/cmd/plank
