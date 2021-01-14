# KETTLE -- Kubernetes Extract Tests/Transform/Load Engine

This collects test results scattered across a variety of GCS buckets,
stores them in a local SQLite database, and outputs newline-delimited
JSON files for import into BigQuery. *See [overview](./overview.md) for more details.*

Results are stored in the [k8s-gubernator:build BigQuery dataset][Big Query Tables],
which is publicly accessible.

# Deploying

Kettle runs as a pod in the `k8s-gubernator/g8r` cluster. To drop into it's context, run `<root>$ make -C kettle get-cluster-credentials`

If you change:

- `buckets.yaml`: do nothing, it's automatically fetched from GitHub
- `deployment.yaml`: deploy with `make push deploy`
- any code: **Run from root** deploy with `make -C kettle push update`, revert with `make -C kettle rollback` if it fails
    - `push` builds the continer image and pushes it to the image registry
    - `update` sets the image of the existing kettle *Pod* which triggers a restart cycle
    - this will build the image to [Google Container Registry](https://console.cloud.google.com/gcr/images/k8s-gubernator/GLOBAL/kettle?project=k8s-gubernator&organizationId=433637338589&gcrImageListsize=30)
    - See [Makefile](Makefile) for details

#### Note:
 - If you make local changes in the branch prior to `make push/update` the image will be uploaded with `-dirty` in the tag. Keep this in mind when seeting the image. If you see a Pod in a `ImagePullBackOff` loop, there is likely an issue when `kubectl image set` was run, where the image does not exist in the specified location.

# Restarting

#### Find out when the build started failing

eg: by looking at the logs

```sh
make get-cluster-credentials
kubectl logs -l app=kettle

# ...

==== 2018-07-06 08:19:05 PDT ========================================
PULLED 174
ACK irrelevant 172
EXTEND-ACK  2
gs://kubernetes-jenkins/pr-logs/pull/kubeflow_kubeflow/1136/kubeflow-presubmit/2385 True True 2018-07-06 07:51:49 PDT FAILED
gs://kubernetes-jenkins/logs/ci-cri-containerd-e2e-ubuntu-gce/5742 True True 2018-07-06 07:44:17 PDT FAILURE
ACK "finished.json" 2
Downloading JUnit artifacts.
```

Alternatively, navigate to [Gubernator BigQuery page][Big Query All] (click on Details) and you can see a table showing last date/time the metrics were collected.

#### Replace pods

```sh
kubectl delete pod -l app=kettle
kubectl rollout status deployment/kettle # monitor pod restart status
kubectl get pod -l app=kettle # should show a new pod name
```

#### Verify functionality

You can watch the pod startup and collect data from various GCS buckets by looking at its logs via:

```sh
kubectl logs -f $(kubectl get pod -l app=kettle -oname)
```
or access [log history](https://console.cloud.google.com/logs/query?project=k8s-gubernator) with the Query: `resource.labels.container_name="kettle"`.

It might take a couple of hours to be fully functional and start updating BigQuery. You can always go back to the [Gubernator BigQuery page][Big Query All] and check to see if data collection has resumed.  Backfill should happen automatically.

#### Kettle Staging

`Kettle Staging` uses a similar deployment to `Kettle` with the following differences
- [100G SSD](https://console.cloud.google.com/compute/disksDetail/zones/us-west1-b/disks/kettle-data-staging?folder=&organizationId=&project=k8s-gubernator) vs 1001G in production
- Limit option for number of builds to pull from each job bucket (Default 1000 each). Set via BUILD_LIMIT env in [deployment-staging.yaml](./deployment-staging.yaml).
- writes to [build.staging](https://console.cloud.google.com/bigquery?project=k8s-gubernator&page=table&t=all&d=build&p=k8s-gubernator&redirect_from_classic=true) table only. This differs from production that writes to three tables `build.all`, `build.day`, and `build.week`.


It can be deployed with `make -C kettle deploy-staging`. If already deployed, you may just run `make -C kettle update-staging`.

#### Adding Fields

To add fields to the BQ table, Visit the [k8s-gubernator:build BigQuery dataset][Big Query Tables] and Select the table (Ex. Build > All). Schema -> Edit Schema -> Add field. As well as update [schema.json](./schema.json)

## Adding Buckets

To add a new GCS bucket to Kettle, simply update [buckets.yaml](./buckets.yaml) in `master`, it will be auto pulled by Kettle on the next cycle.

```yaml
gs://<bucket path>: #bucket url
  contact: "username" #Git Hub Username
  prefix: "abc:" #the identifier prefixed to jobs from this bucket (ends in :).
  sequential: (bool) #an optional boolean that indicates whether test runs in this
  #                  bucket are numbered sequentially
  exclude_jobs: # list of jobs to explicitly exclude from kettle data collection
    - job_name1
    - job_name2
```

# CI

A [postsubmit job](https://github.com/kubernetes/test-infra/blob/master/config/jobs/kubernetes/test-infra/test-infra-trusted.yaml#L203-L210) runs that pushes Kettle on changes.

# PubSub

Kettle `stream.py` leverages Google Cloud [PubSub] to alert on GCS changes. These events are tied to the `gcs-changes` Topic in the `kubernetes-jenkins` project where Prow job artifacts are collated. Each time an artifact is finalized, a PubSub event is triggered and Kettle collects job information when it sees a resource uploaded called `finished.json` (indicating the build completed).

[Subscriptions] are in Kuberenetes Jenkins Build - PubSub.
- kettle
- kettle-staging

They are split so that the staging instance does not consume events aimed at production.

### Auth
For kettle to have permisions, kettle's user needs access. When updating or changing a [Subscription] make sure to add `kettle@k8s-gubernator.iam.gserviceaccount.com` as a `PubSub Editor`.

# Known Issues

- Occasionally data from Kettle stops updating, we suspect this is due to a transient hang when contacting GCS ([#8800](https://github.com/kubernetes/test-infra/issues/8800)). If this happens, [restart kettle](#restarting)

[Big Query Tables]: https://console.cloud.google.com/bigquery?utm_source=bqui&utm_medium=link&utm_campaign=classic&project=k8s-gubernator
[Big Query All]: https://console.cloud.google.com/bigquery?project=k8s-gubernator&page=table&t=all&d=build&p=k8s-gubernator
[Big Query Staging]: https://console.cloud.google.com/bigquery?project=k8s-gubernator&page=table&t=staging&d=build&p=k8s-gubernator
[PubSub]: https://cloud.google.com/pubsub/docs
[Subscriptions]: https://console.cloud.google.com/cloudpubsub/subscription/list?project=kubernetes-jenkins
