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
randomness-generator. I like [random.org][1]. The `oauth-token` is an OAuth2 token
that has read and write access to the bot account. Generate it from the
[account's settings -> Personal access tokens -> Generate new token][2].

```sh
kubectl create secret generic hmac-token --from-file=hmac=/path/to/hook/secret
kubectl create secret generic oauth-token --from-file=oauth=/path/to/oauth/secret
```

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
bazel run //prow/cmd/config -- --plugin-path=path/to/plugins.yaml
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

## TODO Add more jobs by modifying config.yaml

## TODO Run test pods in a different namespace or a different cluster

## TODO Configure SSL

[1]: https://www.random.org/strings/?num=1&len=16&digits=on&upperalpha=on&loweralpha=on&unique=on&format=html&rnd=new
[2]: https://github.com/settings/tokens
