# Monitoring

This folder contains the manifest files for monitoring prow resources.

## Deploy

The deployment has been
[integrated into our CI system](https://github.com/kubernetes/test-infra/blob/201c7788b244ab2fc3efae7249fb939223ef6e1e/prow/deploy.sh#L91-L92),
except `secret` objects.
Cluster admins need to create `secret`s  manually.

```
### replace the sensitive inforamtion in the files before executing:
$ kubectl create -f grafana_secret.yaml
$ kubectl create -f alertmanager-prow_secret.yaml

```

The grafana `Ingress` in [grafana_expose.yaml](grafana_expose.yaml) has
GCE specific annotations. It can be modified/removed if [other ways](https://cloud.google.com/kubernetes-engine/docs/how-to/exposing-apps)
of exposing a service are preferred.

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
    $ bazel build //config/prow/cluster/monitoring/mixins/grafana_dashboards:hook
    $ cat bazel-bin/config/prow/cluster/monitoring/mixins/grafana_dashboards/hook.json
    ```

* Add `bazel` target to [dashboards_out/BUILD.bazel](grafana_dashboards/BUILD.bazel) for generating the configMap with the json file above.

    ```
    ### if you want to apply the configMaps
    $ bazel run //prow/cluster/monitoring/mixins/dashboards_out:grafana-configmaps.apply
    ```

* Use the configMap above in [grafana_deployment.yaml](grafana_deployment.yaml).

As an alternative to `bazel`, the Makefile in [mixin](mixins/Makefile) folder can be used to generate the yaml/json
files from `jsonnet` for debugging locally. As prerequisites, [`jsonnet`](https://github.com/google/jsonnet)
and [`gojsontoyaml`](https://github.com/brancz/gojsontoyaml) should be included in `${PATH}`.

## Access components' Web page

* For `grafana`, visit [monitoring.prow.k8s.io](https://monitoring.prow.k8s.io). Anonymous users are with read-only mode.
Use `adm` and [password](https://github.com/kubernetes/test-infra/blob/master/config/prow/cluster/monitoring/grafana_deployment.yaml#L39-L45) to become admin.

* For `prometheus` and `alertmanager`, there is no public domain configured based on the security
concerns (no authorization out of the box).
Cluster admins can use [k8s port-forward](https://kubernetes.io/docs/tasks/access-application-cluster/port-forward-access-application-cluster/) to
access the web.

    ```
    $ kubectl -n prow-monitoring port-forward $( kubectl -n prow-monitoring get pods --selector app=prometheus -o jsonpath={.items[0].metadata.name} ) 9090
    $ kubectl -n prow-monitoring port-forward $( kubectl -n prow-monitoring get pods --selector app=alertmanager -o jsonpath={.items[0].metadata.name} ) 9093
    ```

    Then, visit [127.0.0.1:9090](http://127.0.0.1:9090) for the `prometheus` pod and [127.0.0.1:9093](http://127.0.0.1:9093) for the `alertmanager` pod.

    As a result of no public domain for those two components, some of the links on the UI do not work, eg, the links on the slack alerts.
