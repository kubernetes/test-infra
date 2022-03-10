# Building, Testing, and Updating Prow

This guide is directed at Prow developers and maintainers who want to build/test individual components or deploy changes to an existing Prow cluster. [`getting_started_deploy.md`](/prow/getting_started_deploy.md) is a better reference for deploying a new Prow cluster.

## How to build and test Prow

You can build, test, and deploy Prow’s binaries, container images, and cluster resources.

Build locally with:
```shell
make -C prow build-images
```
Push to remote with
```shell
make -C prow push-images REGISTRY=<YOUR_REGISTRY>
```
Unit test with:
```shell
make test
```
Integration test with([more details](./test/integration)):
```shell
./prow/test/integration/integration-test.sh
```
Individual packages and components can be built and tested like:
```shell
go build ./prow/cmd/hook
go test ./prow/plugins/lgtm
```
(Note: `deck` depends on non-go static files, these were tested by integration
tests, and for e2e test use [`runlocal`](/prow/cmd/deck/runlocal) if desired.)

### How to test a plugin

If you are making changes to a Prow plugin you can test the new behavior by sending fake webhooks to [`hook`](/prow/cmd/hook) with [`phony`](/prow/cmd/phony#phony).

## How to update the cluster

Any modifications to prow Go code will require redeploying the affected
binaries. The process of doing so is streamlined, which is highly recommended to
all prow instances:

1. Prow code change PR merged.
1. [`post-test-infra-push-prow`](https://github.com/kubernetes/test-infra/blob/e7ff9e7ad8a395bc246c4bc38610d4d57d3b011c/config/jobs/kubernetes/test-infra/test-infra-trusted.yaml#L191)
   is automatically triggered, can be found on
   [prow.k8s.io](https://prow.k8s.io?job=post-test-infra-push-prow), which
   pushes images to [gcr.io/k8s-prow](gcr.io/k8s-prow).
1. Periodic job
   [`ci-test-infra-autobump-prow`](https://github.com/kubernetes/test-infra/blob/e7ff9e7ad8a395bc246c4bc38610d4d57d3b011c/config/jobs/kubernetes/test-infra/test-infra-trusted.yaml#L588)
   runs every hour, looking for latest image tags from
   [gcr.io/k8s-prow](gcr.io/k8s-prow), and creates a PR
   ([example](https://github.com/kubernetes/test-infra/pull/25571)) to let prow
   use the latest tag.
1. Once the periodic job is merged,
   [`post-test-infra-deploy-prow`](https://github.com/kubernetes/test-infra/blob/e7ff9e7ad8a395bc246c4bc38610d4d57d3b011c/config/jobs/kubernetes/test-infra/test-infra-trusted.yaml#L114)
   deploys the config changes from the PR above.

## How to test a ProwJob

The best way to go about testing a new ProwJob depends on the job itself. If the
job can be run locally that is typically the best way to initially test the job
because local debugging is easier and safer than debugging in CI. See
[Running a ProwJob Locally](#running-a-prowjob-locally) below.

Actually running the job on Prow by merging the job config is the next step.
Typically, new presubmit jobs are configured to `skip_report`ing to GitHub and
may not be configured to automatically run on every PR with `always_run: true`.
Once the job is stable these values can be changed to make the job run everywhere
and become visible to users by posting results to GitHub (if desired). Changes
to existing jobs can be trialed on canary jobs.

ProwJobs can also be manually triggered by generating a YAML ProwJob [CRD](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/)
with mkpj and deploying that directly to the Prow cluster, however this pattern
is generally not recommended. It requires the use of direct prod cluster access,
allows ProwJobs to run in prod without passing presubmit validation, and can result
in malformed ProwJobs CRDs that can jam some of Prow's core service components.
See [How to manually run a given job on Prow](#how-to-manually-run-a-given-job-on-prow)
below if you need to do this.

### Running a ProwJob Locally

#### Using pj-on-kind.sh
[pj-on-kind.sh] is a bash script that runs ProwJobs locally as pods in a [Kind] cluster.
The script does the following:
1. Installs [mkpj], [mkpod], and [Kind] if they are not found in the path. A [Kind]
cluster named `mkpod` is created if one does not already exist.
1. Uses [mkpj] to generate a YAML ProwJob CRD given job name, config, and git refs (if applicable).
1. Uses [mkpod] to generate a YAML Pod resource from the ProwJob CRD. This Pod will
be decorated with the pod utilities if needed and will exactly match what would be
applied in prod with two exceptions:
	1. The job logs, metadata, and artifacts will be copied to disk rather than
	uploaded to GCS. By default these files are copied to `/mnt/disks/prowjob-out/<job-name>/<build-id>/`
	on the host machine.
	1. Any volume mounts may be substituted for `emptyDir` or `hostPath` volumes at the
	interactive prompt to replace dependencies that are only available in prod.
	__NOTE!__ In order for `hostPath` volume sources to reach the host and not just the Kind "node" container,
	use paths under `/mnt/disks/kind-node` or set `$NODE_DIR` before the mkpod cluster is created.
1. Applies the Pod to the [Kind] cluster and starts watching it (interrupt whenever,
this is for convenience). At this point the Pod will start running if configured
correctly.

Once the Pod has been applied to the cluster you can wait for it to complete and output
results to the output directory, or you can interact with it using kubectl by first
running `export KUBECONFIG="$(kind get kubeconfig-path --name=mkpod)"`.

Requirements: [Go], [Docker], and [kubectl] must be installed before using this script.
The ProwJob must use `agent: kubernetes` (the default, runs ProwJobs as Pods).

##### pj-on-kind.sh for specific Prow instances
Each Prow instance can supply a preconfigured variant of pj-on-kind.sh that properly
defaults the config file locations. [Example](https://github.com/istio/test-infra/blob/01167b0dc9cb19bee40aa8dff958f526cfeeb570/prow/pj-on-kind.sh)
for [prow.istio.io](https://prow.istio.io).
To test ProwJobs for the [prow.k8s.io] instance use [`config/pj-on-kind.sh`](/config/pj-on-kind.sh).

##### Example
This command runs the ProwJob [`pull-test-infra-yamllint`](https://github.com/kubernetes/test-infra/blob/170921984a34ca40f2763f9e71d6ce6e033dec03/config/jobs/kubernetes/test-infra/test-infra-presubmits.yaml#L94-L107) locally on Kind.
```sh
./pj-on-kind.sh pull-test-infra-yamllint
```
You may also need to set the `CONFIG_PATH` and `JOB_CONFIG_PATH` environmental variables:
```sh
CONFIG_PATH=$(realpath ../config/prow/config.yaml) JOB_CONFIG_PATH=$(realpath ../config/jobs/kubernetes/test-infra/test-infra-presubmits.yaml) ...
```

##### Modifying pj-on-kind.sh for special scenarios
This tool was written in bash so that it can be easily adjusted when debugging.
In particular it should be easy to modify the main function to:
* Add additional K8s resources to the cluster before running the Pod such as
secrets, configmaps, or volumes.
* Skip applying the pod.yaml to the Kind cluster to inspect it, modify it, or apply it to
a real cluster instead of the `mkpod` Kind cluster. (Same for pj.yaml)

##### Debugging within a pj-on-kind.sh container
To point `kubectl` to the Kind cluster you will need to export the `KUBECONFIG` Env. The command to point this to the correct config is echoed in the pj-on-kind.sh logging. It will have the form:
```sh
export KUBECONFIG='/<path to user dir>/.kube/kind-config-mkpod'
```
After pointing to the correct master you will be able to drop into the container using `kubectl exec -it <pod name> <bash/sh/etc>`. **This pod will only last the lifecycle of the job, if you need more time to debug you might add a `sleep` within the job execution.

#### Using Phaino
[Phaino](/prow/cmd/phaino) lets you interactively mock and run the job locally on
your workstation in a docker container. Detailed instructions can be found in
Phaino's [Readme](/prow/cmd/phaino/README.md).

Note: Test containers designed for decorated jobs (configured with `decorate: true`)
may behave incorrectly or fail entirely without the environment the pod utilities
provide. Similarly jobs that mount volumes or use `extra_refs` likely won't work
properly.
These jobs are best run locally as decorated pods inside a [Kind] cluster [Using pj-on-kind.sh](#using-pj-on-kindsh).

### How to manually run a given job on Prow

If the normal job triggering mechanisms (`/test foo` comments, PR changes, PR
merges, cron schedule) are not sufficient for your testing you can use `mkpj` to
manually trigger new ProwJob runs.
To manually trigger any ProwJob, run the following, specifying `JOB_NAME`:

For K8S Prow, you can trigger a job by running
```shell
go run ./config:mkpj --job=JOB_NAME
```

For your own prow instance:
```shell
go run k8s.io/test-infra/prow/cmd/mkpj --job=JOB_NAME --config-path=path/to/config.yaml
```

Alternatively, if you have jobs defined in a separate `job-config`, you can
specify the config by adding the flag `--job-config-path=path/to/job/config.yaml`.

This will print the ProwJob YAML to stdout. You may pipe it into `kubectl`.
Depending on the job, you will need to specify more information such as PR
number.

NOTE: It is dangerous to create ProwJobs from handcrafted YAML. Please use `mkpj`
to generate ProwJob YAML.

[prow.k8s.io]: https://prow.k8s.io
[Go]: https://golang.org/doc/install
[Docker]: https://docs.docker.com/install/
[kubectl]: https://kubernetes.io/docs/tasks/tools/install-kubectl/
[Kind]: https://sigs.k8s.io/kind
[mkpj]: /prow/cmd/mkpj
[mkpod]: /prow/cmd/mkpod
[pj-on-kind.sh]: /prow/pj-on-kind.sh
