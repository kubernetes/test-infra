## Instructions to restart Kettle job
Occasionally we run into situation where there's no data in the Bigquery metrics dashboard in Velodrome.We suspect this to be due to Kettle pod running out of disk space. While we [debug the rootcause](https://github.com/kubernetes/test-infra/issues/8703), here's how you can kickstart the Kettle pod to get it going again.

* CD into k8s/test-infra/kettle
* make get-cluster-credentials 
* kubectl logs $( kubectl get pods -o jsonpath='{.items[?(@.metadata.labels.app=="kettle")].metadata.name}' )

You will see a failure message indicating when the build started failing

```
==== 2018-07-06 08:19:05 PDT ========================================
PULLED 174
ACK irrelevant 172
EXTEND-ACK  2
gs://kubernetes-jenkins/pr-logs/pull/kubeflow_kubeflow/1136/kubeflow-presubmit/2385 True True 2018-07-06 07:51:49 PDT FAILED
gs://kubernetes-jenkins/logs/ci-cri-containerd-e2e-ubuntu-gce/5742 True True 2018-07-06 07:44:17 PDT FAILURE
ACK "finished.json" 2
Downloading JUnit artifacts.
```

Alternatively, navigating to [Gubernator BigQuery page](https://bigquery.cloud.google.com/table/k8s-gubernator:build.all?pli=1&tab=details) (click on “all” on the left and “Details”) you can see a table showing last date/time the metrics were collected.

* kubectl delete pod/$( kubectl get pods -o jsonpath='{.items[?(@.metadata.labels.app=="kettle")].metadata.name}' )

* kubectl rollout status deployment/kettle // monitor pod restart status

* kubectl get pods // should show a new pod name
```
NAME                      READY     STATUS    RESTARTS   AGE
kettle-5df45c4dcb-pf5l2   1/1       Running   2          2h
```
* kubectl logs $( kubectl get pods -o jsonpath='{.items[?(@.metadata.labels.app=="kettle")].metadata.name}' ) // should indicate the pod starting up and collecting data from various GCS buckets. 

It might take a couple of hours to be fully functional and start updating BigQuery. You can always go back to Gubernator BigQuery page and check to see if data collection has resumed.

### Issues
We have a [feature request open on test-infra](https://github.com/kubernetes/test-infra/issues/8743) to setup alerts when Kettle job stops/fails.
