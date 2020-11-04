# Deploying Prow

This document will walk you through deploying your own Prow instance to a new Kubernetes cluster. If you encounter difficulties, please open an issue so that we can make this process easier.

Prow runs in any kubernetes cluster. Our `tackle` utility helps deploy it correctly, or you can perform each of the steps manually.

Both of these are focused on [Kubernetes Engine](https://cloud.google.com/kubernetes-engine/) but should work on any kubernetes distro with no/minimal changes.

## GitHub bot account

Before using `tackle` or deploying prow manually, ensure you have created a
GitHub account for prow to use.  Prow will ignore most GitHub events generated
by this account, so it is important this account be separate from any users or
automation you wish to interact with prow. For example, you still need to do
this even if you'd just setting up a prow instance to work against your own
personal repos.

1. Ensure the bot user has the following permissions
    - Write access to the repos you plan on handling
    - Owner access (and org membership) for the orgs you plan on handling (note
      it is possible to handle specific repos in an org without this)
1. Create a [personal access token][1] for the GitHub bot account, adding the
   following scopes (more details [here][8])
    - Must have the `public_repo` and `repo:status` scopes
    - Add the `repo` scope if you plan on handing private repos
    - Add the `admin:org_hook` scope if you plan on handling a github org
1. Set this token aside for later (we'll assume you wrote it to a file on your
   workstation at `/path/to/github/token`)

## Tackle deployment

Prow's `tackle` utility walks you through deploying a new instance of prow in a couple minutes, try it out!

You need a few things:

1. [`bazel`](https://bazel.build/) build tool installed and working
1. The prow `tackle` utility. It is recommended to use it by running `bazel run //prow/cmd/tackle` from `test-infra` directory, alternatively you can install it by running `go get -u k8s.io/test-infra/prow/cmd/tackle` (in that case you would also need go installed and working).
**Note**: Creating the `tackle` utility assumes you have the `gcloud` application in your `$PATH`,
if you are doing this on another cloud skip to the **Manual deployment** below.
1. Optionally, credentials to a Kubernetes cluster (otherwise, `tackle` will help you create on GCP)

To install prow run the following from the `test-infra` directory and follow the on-screen instructions:

```sh
# Ideally use https://bazel.build, alternatively try:
#   go get -u k8s.io/test-infra/prow/cmd/tackle && tackle
$ bazel run //prow/cmd/tackle
```

This will help you through the following steps:

* Choosing a kubectl context (and creating a cluster / getting its credentials if necessary)
* Deploying prow into that cluster
* Configuring GitHub to send prow webhooks for your repos. This is where you'll provide the absolute `/path/to/github/token`

See the [Next Steps](#next-steps) section after running this utility.

## Manual deployment

If you do not want to use the `tackle` utility above, here are the manual set of commands tackle will run.

Prow runs in a kubernetes cluster, so first figure out which cluster you want to deploy prow into.
If you already have a cluster created you can skip to the **Create cluster role bindings** step.

### Create the cluster

You can use the [GCP cloud console](https://console.cloud.google.com/) to set up a project and [create a new Kubernetes Engine cluster](https://console.cloud.google.com/kubernetes).

I'm assuming that `PROJECT` and `ZONE` environment variables are set, if you are using
GCP. Skip this step if you are using another service to host your Kubernetes cluster.

```sh
$ export PROJECT=your-project
$ export ZONE=us-west1-a
```

Run the following to create the cluster. This will also set up `kubectl` to
point to the new cluster on GCP.

```sh
$ gcloud container --project "${PROJECT}" clusters create prow \
  --zone "${ZONE}" --machine-type n1-standard-4 --num-nodes 2
```

### Create cluster role bindings

As of 1.8 Kubernetes uses [Role-Based Access Control (“RBAC”)](https://kubernetes.io/docs/admin/authorization/rbac/) to drive authorization decisions, allowing `cluster-admin` to dynamically configure policies.
To create cluster resources you need to grant a user `cluster-admin` role in all namespaces for the cluster.

For Prow on GCP, you can use the following command.

```sh
$ kubectl create clusterrolebinding cluster-admin-binding \
  --clusterrole cluster-admin --user $(gcloud config get-value account)
```

For Prow on other platforms, the following command will likely work.

```sh
$ kubectl create clusterrolebinding cluster-admin-binding-"${USER}" \
  --clusterrole=cluster-admin --user="${USER}"
```

On some platforms the `USER` variable may not map correctly to the user
in-cluster. If you see an error of the following form, this is likely the case.

```sh
Error from server (Forbidden): error when creating
"config/prow/cluster/starter-gcs.yaml": roles.rbac.authorization.k8s.io "<account>" is
forbidden: attempt to grant extra privileges:
[PolicyRule{Resources:["pods/log"], APIGroups:[""], Verbs:["get"]}
PolicyRule{Resources:["prowjobs"], APIGroups:["prow.k8s.io"], Verbs:["get"]}
APIGroups:["prow.k8s.io"], Verbs:["list"]}] user=&{<CLUSTER_USER>
[system:authenticated] map[]}...
```

Run the previous command substituting `USER` with `CLUSTER_USER` from the error
message above to solve this issue.

```sh
$ kubectl create clusterrolebinding cluster-admin-binding-"<CLUSTER_USER>" \
  --clusterrole=cluster-admin --user="<CLUSTER_USER>"
```

There are [relevant docs on Kubernetes Authentication](https://kubernetes.io/docs/reference/access-authn-authz/authentication/#authentication-strategies) that may help if neither of the above work.

### Create the GitHub secrets

You will need two secrets to talk to GitHub. The `hmac-token` is the token that
you give to GitHub for validating webhooks. Generate it using any reasonable
randomness-generator, eg `openssl rand -hex 20`.

```sh
$ openssl rand -hex 20 > /path/to/hook/secret
$ kubectl create secret generic hmac-token --from-file=hmac=/path/to/hook/secret
```

The `github-token` is the *Personal access token* you created above for the [GitHub bot account].
If you need to create one, go to <https://github.com/settings/tokens>.

```sh
kubectl create secret generic github-token --from-file=token=/path/to/github/token
```

### Update the sample manifest

There are two sample manifests to get you started:
* [`starter-s3.yaml`](/config/prow/cluster/starter-s3.yaml) sets up a minio as blob storage for logs and is particularly well suited to quickly get something working
* [`starter-gcs.yaml`](/config/prow/cluster/starter-gcs.yaml) uses GCS as blob storage and requires additional configuration to set up the bucket and ServiceAccounts. See [this](#Configure a GCS bucket) for details.

Regardless of which object storage you choose, the below adjustments are always needed:

* The github token by replacing the `<<insert-token-here>>` string
* The hmac token by replacing the `<< insert-hmac-token-here >>` string
* The domain by replacing the `<< your-domain.com >>` string
* Optionally, you can update the `cert-manager.io/cluster-issuer:` annotation if you use cert-manager
* Your github organization(s) by replacing the `<< your_github_org >>` string

### Add the prow components to the cluster

Apply the manifest you edited above by executing one of the following two commands:


* `kubectl apply -f config/prow/cluster/starter-s3.yaml`
* `kubectl apply -f config/prow/cluster/starter-gcs.yaml`

After a moment, the cluster components will be running.

```sh
$ kubectl get pods
NAME                                       READY   STATUS    RESTARTS   AGE
crier-69b6bd8f48-6sg24                     1/1     Running   0          9m54s
deck-7f6867c46c-j7nnh                      1/1     Running   0          2m5s
deck-7f6867c46c-mkxzk                      1/1     Running   0          2m5s
ghproxy-fdd45dfb6-582fh                    1/1     Running   0          9m54s
hook-7cc4df66f7-r2qpl                      1/1     Running   1          9m53s
hook-7cc4df66f7-shnjq                      1/1     Running   1          9m53s
horologium-7976c7f597-ss86t                1/1     Running   0          9m53s
minio-d756b6477-d4w4k                      1/1     Running   0          9m53s
prow-controller-manager-657767bb69-5qzhp   1/1     Running   0          9m53s
sinker-8b645d469-jjw8r                     1/1     Running   0          9m53s
statusreconciler-669697d466-zqfsj          1/1     Running   0          3m11s
tide-65489c49b8-rpnn2                      1/1     Running   0          3m2s
```

#### Get ingress IP address

Find out your external address. It might take a couple minutes for the IP to
show up.

```sh
kubectl get ingress prow
NAME   CLASS    HOSTS                     ADDRESS               	PORTS     AGE
prow   <none>   prow.<<yourdomain.com>>   an.ip.addr.ess          80, 443   22d
```

Go to that address in a web browser and verify that the "echo-test" job has a
green check-mark next to it. At this point you have a prow cluster that is ready
to start receiving GitHub events!

## Add the webhook to GitHub

### Add your first webhook

You have two options to do this:

1. You can do this with the `update-hook` utility:

```sh
# Note /path/to/hook/secret and /path/to/github/token from earlier secrets step
# Note the an.ip.addr.ess from previous ingress step

# Ideally use https://bazel.build, alternatively try:
#   go get -u k8s.io/test-infra/experiment/update-hook && update-hook
$ bazel run //experiment/update-hook -- \
  --hmac-path=/path/to/hook/secret \
  --github-token-path=/path/to/github/token \
  --hook-url http://an.ip.addr.ess/hook \
  --repo my-org/my-repo \
  --repo my-whole-org \
  --confirm=false  # Remove =false to actually add hook
```

Look for the `http://an.ip.addr.ess/hook` you added above.
A green check mark (for a ping event, if you click edit and view the details of the event) suggests everything is working!

2. If you do not want to use the `update-hook` utility, you can go the GitHub web page and add the hook manually:

- Go to your org or repo and click `Settings -> Webhooks`, and click `Add webhook`.
- Change the `Payload URL` to `http://an.ip.addr.ess/hook` you are planning to add.
- Change the `Content type` to `application/json`, and change your `Secret` to the `hmac-path` secret you created above.
- Change the trigger to `Send me **everything**`.
- Click `Add webhook`.

### Use `hmac` tool to manage webhooks and hmac tokens (recommended)

If you need to configure webhooks for multiple orgs or repos, the manual process does not work that well as it
can be error-prone, and it'll be painful when you want to replace the hmac token if it is accidentally leaked.

In such case, it's recommended to use the [hmac](/prow/cmd/hmac/README.md) tool to automatically manage the webhooks
and hmac tokens for you via declarative configuration.

## Next Steps

You now have a working Prow cluster (Woohoo!), but it isn't doing anything interesting yet.
This section will help you complete any additional setup that your instance may need.

### Configure a GCS bucket

> If you want to persist logs and output in GCS, you need to follow the steps below.

When configuring Prow jobs to use the [Pod utilities](./pod-utilities.md)
with `decorate: true`, job metdata, logs, and artifacts will be uploaded
to a GCS bucket in order to persist results from tests and allow for the
job overview page to load those results at a later point. In order to run
these jobs, it is required to set up a GCS bucket for job outputs. If your
Prow deployment is targeted at an open source community, it is strongly
suggested to make this bucket world-readable.

In order to configure the bucket, follow the following steps:

1. [provision](https://cloud.google.com/iam/docs/creating-managing-service-accounts) a new service account for interaction with the bucket
1. [create](https://cloud.google.com/storage/docs/creating-buckets) the bucket
1. (optionally) [expose](https://cloud.google.com/storage/docs/access-control/making-data-public) the bucket contents to the world
1. [grant access](https://cloud.google.com/storage/docs/access-control/using-iam-permissions) to admin the bucket for the service account
1. [serialize](https://cloud.google.com/iam/docs/creating-managing-service-account-keys) a key for the service account
1. upload the key to a `Secret` under the `service-account.json` key
1. edit the `plank` configuration for `default_decoration_configs['*'].gcs_credentials_secret` to point to the `Secret` above

After [downloading](https://cloud.google.com/sdk/gcloud/) the `gcloud` tool and authenticating,
the following collection of commands will execute the above steps for you:

```sh
$ gcloud iam service-accounts create prow-gcs-publisher
identifier="$(  gcloud iam service-accounts list --filter 'name:prow-gcs-publisher' --format 'value(email)' )"
$ gsutil mb gs://prow-artifacts/ # step 2
$ gsutil iam ch allUsers:objectViewer gs://prow-artifacts # step 3
$ gsutil iam ch "serviceAccount:${identifier}:objectAdmin" gs://prow-artifacts # step 4
$ gcloud iam service-accounts keys create --iam-account "${identifier}" service-account.json # step 5
$ kubectl -n test-pods create secret generic gcs-credentials --from-file=service-account.json # step 6
```

#### Configure the version of plank's utility images

Before we can update plank's `default_decoration_configs['*']` we'll need to retrieve the version of plank using the following:

```sh
$ kubectl get pod -lapp=plank -o jsonpath='{.items[0].spec.containers[0].image}' | cut -d: -f2
v20191108-08fbf64ac
```
Then, we can use that tag to retrieve the corresponding utility images in `default_decoration_configs['*']` in `config.yaml`:

For more information on how the pod utility images for prow are versioned see [autobump](/prow/cmd/autobump/README.md)

```yaml
plank:
  default_decoration_configs:
    '*':
      utility_images: # using the tag we identified above
        clonerefs: "gcr.io/k8s-prow/clonerefs:v20191108-08fbf64ac"
        initupload: "gcr.io/k8s-prow/initupload:v20191108-08fbf64ac"
        entrypoint: "gcr.io/k8s-prow/entrypoint:v20191108-08fbf64ac"
        sidecar: "gcr.io/k8s-prow/sidecar:v20191108-08fbf64ac"
      gcs_configuration:
        bucket: prow-artifacts # the bucket we just made
        path_strategy: explicit
      gcs_credentials_secret: gcs-credentials # the secret we just made
```

### Adding more jobs

There are two ways to configure jobs:
* Using the [inrepoconfig](/prow/inrepoconfig.md) feature to configure jobs inside the repo under test
* Using the static config by editing the `config` configmap, some samples below:

Add the following to `config.yaml`:

```yaml
periodics:
- interval: 10m
  name: echo-test
  decorate: true
  spec:
    containers:
    - image: alpine
      command: ["/bin/date"]
postsubmits:
  YOUR_ORG/YOUR_REPO:
  - name: test-postsubmit
    decorate: true
    spec:
      containers:
      - image: alpine
        command: ["/bin/printenv"]
presubmits:
  YOUR_ORG/YOUR_REPO:
  - name: test-presubmit
    decorate: true
    always_run: true
    skip_report: true
    spec:
      containers:
      - image: alpine
        command: ["/bin/printenv"]
```

Again, run the following to test the files, replacing the paths as necessary:

```sh
$ bazel run //prow/cmd/checkconfig -- --plugin-config=path/to/plugins.yaml --config-path=path/to/config.yaml
```

Now run the following to update the configmap.

```sh
$ kubectl create configmap config \
  --from-file=config.yaml=path/to/config.yaml --dry-run=server -o yaml | kubectl replace configmap config -f -
```

We create a `make` rule:

```Make
update-config: get-cluster-credentials
    kubectl create configmap config --from-file=config.yaml=config.yaml --dry-run=server -o yaml | kubectl replace configmap config -f -
```

Presubmits and postsubmits are triggered by the `trigger` plugin. Be sure to
enable that plugin by adding it to the list you created in the last section.

Now when you open a PR it will automatically run the presubmit that you added
to this file. You can see it on your prow dashboard. Once you are happy that it
is stable, switch `skip_report` in the above `config.yaml` to `false`. Then, it will post a status on the
PR. When you make a change to the config and push it with `make update-config`,
you do not need to redeploy any of your cluster components. They will pick up
the change within a few minutes.

When you push or merge a new change to the git repo, the postsubmit job will run.

For more information on the job environment, see [`jobs.md`](/prow/jobs.md)

### Run test pods in different clusters

You may choose to run test pods in a separate cluster entirely. This is a good practice to keep testing isolated from Prow's service components and secrets. It can also be used to furcate job execution to different clusters.
One can use a Kubernetes [`kubeconfig`](https://kubernetes.io/docs/concepts/configuration/organize-cluster-access-kubeconfig/) file (i.e. `Config` object) to instruct Prow components to use the *build* cluster(s).
All contexts in `kubeconfig` are used as *build* clusters and the [`InClusterConfig`](https://kubernetes.io/docs/tasks/access-application-cluster/access-cluster/#accessing-the-api-from-a-pod) (or `current-context`) is the *default*.

Create a secret containing a `kubeconfig` like this:

```yaml
apiVersion: v1
clusters:
- name: default
  cluster:
    certificate-authority-data: fake-ca-data-default
    server: https://1.2.3.4
- name: other
  cluster:
    certificate-authority-data: fake-ca-data-other
    server: https://5.6.7.8
contexts:
- name: default
  context:
    cluster: default
    user: default
- name: other
  context:
    cluster: other
    user: other
current-context: default
kind: Config
preferences: {}
users:
- name: default
  user:
    token: fake-token-default
- name: other
  user:
    token: fake-token-other
```

Use [gencred][5] to create the `kubeconfig` file (and credentials) for accessing the cluster(s):

> **NOTE:** `gencred` will merge new entries to the specified `output` file on successive invocations by *default* .

Create a *default* cluster context (if one does not already exist):

> **NOTE:** If executing `gencred` with `bazel` like below, ensure `--output` is an *absolute* path.

```sh
$ bazel run //gencred -- \
  --context=<kube-context> \
  --name=default \
  --output=/tmp/kubeconfig.yaml \
  --serviceaccount
```

Create one or more *build* cluster contexts:

> **NOTE:** the `current-context` of the *existing* `kubeconfig` will be preserved.

```sh
$ bazel run //gencred -- \
  --context=<kube-context> \
  --name=other \
  --output=/tmp/kubeconfig.yaml \
  --serviceaccount
```

Create a secret containing the `kubeconfig.yaml` in the cluster:

```sh
$ kubectl --context=<kube-context> create secret generic kubeconfig --from-file=config=/tmp/kubeconfig.yaml
```

Mount this secret into the prow components that need it (at minimum: `plank`,
`sinker` and `deck`) and set the `--kubeconfig` flag to the location you mount it at. For
instance, you will need to merge the following into the plank deployment:

```yaml
spec:
  containers:
  - name: plank
    args:
    - --kubeconfig=/etc/kubeconfig/config # basename matches --from-file key
    volumeMounts:
    - name: kubeconfig
      mountPath: /etc/kubeconfig
      readOnly: true
  volumes:
  - name: kubeconfig
    secret:
      defaultMode: 0644
      secretName: kubeconfig # example above contains a `config` key
```

Configure jobs to use the non-default cluster with the `cluster:` field.
The above example `kubeconfig.yaml` defines two clusters: `default` and `other` to schedule jobs, which we can use as follows:

```yaml
periodics:
- name: cluster-unspecified
  # cluster:
  interval: 10m
  decorate: true
  spec:
    containers:
    - image: alpine
      command: ["/bin/date"]
- name: cluster-default
  cluster: default
  interval: 10m
  decorate: true
  spec:
    containers:
    - image: alpine
      command: ["/bin/date"]
- name: cluster-other
  cluster: other
  interval: 10m
  decorate: true
  spec:
    containers:
    - image: alpine
      command: ["/bin/date"]
```

This results in:

* The `cluster-unspecified` and `cluster-default` jobs run in the `default` cluster.
* The `cluster-other` job runs in the `other` cluster.

See [gencred][5] for more details about how to create/update `kubeconfig.yaml`.

### Enable merge automation using Tide

PRs satisfying a set of predefined criteria can be configured to be
automatically merged by [Tide][6].

Tide can be enabled by modifying `config.yaml`.
See [how to configure tide][7] for more details.

#### Set up GitHub OAuth
GitHub Oauth is required for [PR Status](https://prow.k8s.io/pr)
and for the rerun button on [Prow Status](https://prow.k8s.io).
To enable these features, follow the
instructions in [`github_oauth_setup.md`](https://github.com/kubernetes/test-infra/blob/master/prow/cmd/deck/github_oauth_setup.md).

### Configure SSL

Use [cert-manager][3] for automatic LetsEncrypt integration. If you
already have a cert then follow the [official docs][4] to set up HTTPS
termination. Promote your ingress IP to static IP. On GKE, run:

```sh
$ gcloud compute addresses create [ADDRESS_NAME] --addresses [IP_ADDRESS] --region [REGION]
```

Point the DNS record for your domain to point at that ingress IP. The convention
for naming is `prow.org.io`, but of course that's not a requirement.

Then, install cert-manager as described in its readme. You don't need to run it in
a separate namespace.

## Further reading

* [Developing for Prow](/prow/getting_started_develop.md)
* [Getting more out of Prow](/prow/more_prow.md)

[1]: https://github.com/settings/tokens
[2]: /prow/jobs.md#How-to-configure-new-jobs
[3]: https://github.com/jetstack/cert-manager
[4]: https://kubernetes.io/docs/concepts/services-networking/ingress/#tls
[5]: /gencred/
[6]: /prow/cmd/tide/README.md
[7]: /prow/cmd/tide/config.md
[8]: https://github.com/kubernetes/test-infra/blob/master/prow/scaling.md#working-around-githubs-limited-acls
