# Monitoring

This folder contains the manifest files for monitoring prow resources.

## Deploy

The deployment has been integrated into our CI system, except `secret` objects.
Cluster admins need to create `secret`s  manually.

```
### replace the sensitive inforamtion in the files before executing:
$ kubectl create -f grafana_secret.yaml
$ kubectl create -f alertmanager-prow_secret.yaml

```

A successful deploy will spawn a stack of monitoring for prow in namespace `prow-monitoring`: _prometheus_, _alertmanager_, and _grafana_.

_Add more dashboards_:

Suppose that there is an App running as a pod that exposes Prometheus metrics on port `n` and we want to include it into our prow-monitoring stack.
First step is to create a k8s-service to proxy port `n` if you have not done it yet.

### Add the service as target in Prometheus

Add a new `servicemonitors.monitoring.coreos.com` which proxies the targeting service into [prow_servicemonitors.yaml](./prow_servicemonitors.yaml), eg,
`servicemonitor` for `ghproxy`,

```
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  labels:
    app: ghproxy
  name: ghproxy
  namespace: prow-monitoring
spec:
  endpoints:
    - interval: 30s
      port: metrics
      scheme: http
  namespaceSelector:
    matchNames:
      - default
  selector:
    matchLabels:
      app: ghproxy

```

The `svc` should be available on prometheus web UI: `Status` &rarr; `Targets`.

_Note_ that the `servicemonitor` has to have label `app` as key (value could be an arbitrary string).

### Add a new grafana dashboard

We use [jsonnet](https://jsonnet.org) to generate the json files for grafana dashboards and [jsonnet-bundler](https://github.com/jsonnet-bundler/jsonnet-bundler) to manage the jsonnet libs.
Developing a new dashboard can be achieved by

* Create a new file `<dashhoard_name>.jsonnet` in folder [grafana_dashboards](grafana_dashboards).
* Add `bazel` target to [grafana_dashboards/BUILD.bazel](grafana_dashboards/BUILD.bazel) for generating the corresponding json file `<dashhoard_name>.json`.

    ```
    ### if you want to take a look at some json file, eg, hook.json
    $ bazel build //prow/monitoring/mixins/grafana_dashboards:hook
    $ cat bazel-bin/prow/monitoring/mixins/grafana_dashboards/hook.json
    ```

* Add `bazel` target to [dashboards_out/BUILD.bazel](grafana_dashboards/BUILD.bazel) for generating the configMap with the json file above.

    ```
    ### if you want to apply the configMaps
    $ bazel run //prow/monitoring/mixins/dashboards_out:grafana-configmaps.apply
    ```

* Use the configMap above in [grafana_deployment.yaml](grafana_deployment.yaml).