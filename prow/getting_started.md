# How to turn up a new cluster

Prow should run anywhere that Kubernetes runs. Here are the steps required to
set up a basic prow cluster on [GKE](https://cloud.google.com/container-engine/).
Prow will work on any Kubernetes cluster, so feel free to turn up a cluster
some other way and skip the first step. You can set up a project on GCP using
the [cloud console](https://console.cloud.google.com/).

## Create the cluster

I'm assuming that `PROJECT` and `ZONE` environment variables are set.

```sh
export PROJECT=your-project
export ZONE=us-west1-a
```

Run the following to create the cluster. This will also set up `kubectl` to
point to the new cluster.

```sh
gcloud container --project "${PROJECT}" clusters create prow \
  --zone "${ZONE}" --machine-type n1-standard-4 --num-nodes 2
```

## Create the GitHub secrets

You will need two secrets to talk to GitHub. The `hmac-token` is the token that
you give to GitHub for validating webhooks. Generate it using any reasonable
randomness-generator, eg `openssl rand -hex 20`. The `oauth-token` is an OAuth2 token
that has read and write access to the bot account. Generate it from the
[account's settings -> Personal access tokens -> Generate new token][1].

```sh
kubectl create secret generic hmac-token --from-file=hmac=/path/to/hook/secret
kubectl create secret generic oauth-token --from-file=oauth=/path/to/oauth/secret
```

#### Bot account

The bot account used by prow must be granted owner level access to the Github
orgs that prow will operate on. Note that events triggered by this account are
ignored by some prow plugins. It is prudent to use a different bot account for
other Github automation that prow should interact with to prevent events from
being ignored unjustly.

## Run the prow components in the cluster

Run the following command to start up a basic set of prow components.

```sh
kubectl apply -f cluster/starter.yaml
```

After a moment, the cluster components will be running.

```sh
$ kubectl get deployments
NAME         DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
deck         2         2         2            2           1m
hook         2         2         2            2           1m
horologium   1         1         1            1           1m
plank        1         1         1            1           1m
sinker       1         1         1            1           1m
```

Find out your external address. It might take a couple minutes for the IP to
show up.

```sh
$ kubectl get ingress ing
NAME      HOSTS     ADDRESS          PORTS     AGE
ing       *         an.ip.addr.ess   80        3m
```

Go to that address in a web browser and verify that the "echo-test" job has a
green check-mark next to it. At this point you have a prow cluster that is ready
to start receiving GitHub events!

## Add the webhook to GitHub

On the GitHub repo you would like to use, go to Settings -> Webhooks -> Add
webhook. You can also add org-level webhooks.

Set the payload URL to `http://<IP-FROM-INGRESS>/hook`, the content type to
`application/json`, the secret to your HMAC secret, and ask it to send everything.
After you've created your webhook, GitHub will indicate that it successfully
sent an event by putting a green checkmark under "Recent Deliveries."

# Next steps

## Enable some plugins by modifying `plugins.yaml`

Create a file called `plugins.yaml` and add the following to it:

```yaml
plugins:
  YOUR_ORG/YOUR_REPO:
  - size
```

Replace `YOUR_ORG/YOUR_REPO:` with the appropriate values. If you want, you can
instead just say `YOUR_ORG:` and the plugin will run for every repo in the org.

Run the following to test the file, replacing the path as necessary:

```
bazel run //prow/cmd/config -- --plugin-config=path/to/plugins.yaml
```

There should be no errors. You can run this as a part of your presubmit testing
so that any errors are caught before you try to update.

Now run the following to update the configmap, replacing the path as necessary:

```
kubectl create configmap plugins --from-file=plugins=path/to/plugins.yaml --dry-run -o yaml | kubectl replace configmap plugins -f -
```

We added a make rule to do this for us:

```Make
get-cluster-credentials:
    gcloud container clusters get-credentials "$(CLUSTER)" --project="$(PROJECT)" --zone="$(ZONE)"

update-plugins: get-cluster-credentials
    kubectl create configmap plugins --from-file=plugins=plugins.yaml --dry-run -o yaml | kubectl replace configmap plugins -f -
```

Now when you open a PR, it will automatically be labelled with a `size/*`
label. When you make a change to the plugin config and push it with `make
update-plugins`, you do not need to redeploy any of your cluster components.
They will pick up the change within a few minutes.

## Add more jobs by modifying `config.yaml`

Create a file called `config.yaml`, and add the following to it:

```yaml
periodics:
- interval: 10m
  agent: kubernetes
  name: echo-test
  spec:
    containers:
    - image: alpine
      command: ["/bin/date"]
postsubmits:
  YOUR_ORG/YOUR_REPO:
  - name: test-postsubmit
    agent: kubernetes
    spec:
      containers:
      - image: alpine
        command: ["/bin/printenv"]
presubmits:
  YOUR_ORG/YOUR_REPO:
  - name: test-presubmit
    trigger: "(?m)^/test this"
    rerun_command: "/test this"
    context: test-presubmit
    always_run: true
    skip_report: true
    agent: kubernetes
    spec:
      containers:
      - image: alpine
        command: ["/bin/printenv"]
```

Run the following to test the file, replacing the path as necessary:

```
bazel run //prow/cmd/config -- --config-path=path/to/config.yaml
```

Now run the following to update the configmap.

```
kubectl create configmap config --from-file=config=path/to/config.yaml --dry-run -o yaml | kubectl replace configmap config -f -
```

We use a make rule:

```Make
update-config: get-cluster-credentials
    kubectl create configmap config --from-file=config=config.yaml --dry-run -o yaml | kubectl replace configmap config -f -
```

Presubmits and postsubmits are triggered by the `trigger` plugin. Be sure to
enable that plugin by adding it to the list you created in the last section.

Now when you open a PR it will automatically run the presubmit that you added
to this file. You can see it on your prow dashboard. Once you are happy that it
is stable, switch `skip_report` to `false`. Then, it will post a status on the
PR. When you make a change to the config and push it with `make update-config`,
you do not need to redeploy any of your cluster components. They will pick up
the change within a few minutes.

When you push a new change, the postsubmit job will run.

For more information on the job environment, see [How to add new jobs][2].

## Run test pods in a different namespace or a different cluster

You may choose to keep prowjobs or run tests in a different namespace. First
create the namespace by `kubectl create -f`ing this:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: prow
```

Now, in `config.yaml`, set `prowjob_namespace` or `pod_namespace` to the
name from the YAML file. You can then use RBAC roles to limit what test pods
can do.

You may choose to run test pods in a separate cluster entirely. Create a secret
containing the following:

```yaml
endpoint: https://<master-ip>
clientCertificate: <base64-encoded cert>
clientKey: <base64-encoded key>
clusterCaCertificate: <base64-encoded cert>
```

You can learn these by running `gcloud container clusters describe` on your
cluster. Then, mount this secret into the prow components that need it and set
the `--build-cluster` flag to the location you mount it at. For instance, you
will need to merge the following into the plank deployment:

```yaml
spec:
  containers:
  - name: plank
    args:
    - --build-cluster=/etc/cluster/cluster
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

## Configure SSL

I suggest using [kube-lego][3] for automatic LetsEncrypt integration. If you
already have a cert then follow the [official docs][4] to set up HTTPS
termination. Promote your ingress IP to static IP. On GKE, run:

```
gcloud compute addresses create [ADDRESS_NAME] --addresses [IP_ADDRESS] --region [REGION]
```

Point the DNS record for your domain to point at that ingress IP. The convention
for naming is `prow.org.io`, but of course that's not a requirement.

Then, install kube-lego as described in its readme. You don't need to run it in
a separate namespace.

[1]: https://github.com/settings/tokens
[2]: ./README.md##how-to-add-new-jobs
[3]: https://github.com/jetstack/kube-lego
[4]: https://kubernetes.io/docs/concepts/services-networking/ingress/#tls
