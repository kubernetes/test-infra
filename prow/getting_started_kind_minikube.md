# Simplified kind/minikube deployment 

This document is targeting developers, testers, and all the other interested parties who would like to learn or test prow, but do not have resources other than a localhost workstation. It describes a simplified minikube/kind deployment of crucial prow components:

* [`crier`](prow/cmd/crier)
* [`deck`](prow/cmd/deck)
* [`hook`](prow/cmd/hook)
* [`horologium`](prow/cmd/horologium) 
* [`prow-controller-manager`](prow/cmd/prow-controller-manager) 
* [`sinker`](prow/cmd/sinker) 
* [`tide`](prow/cmd/tide)
* [`status-reconciler`](prow/cmd/status-reconciler)
* [`ghproxy`](ghproxy)

Additionally, besides prow components, minio is set up as blob storage for logs.

More information about each component function can be found [here](prow/cmd/). Other components can be easily added as well, depending on user needs.

## Prerequisites

* Create a new GitHub repository or choose an existing one that will be connected to your localhost prow.
* Please follow [**GitHub App**](prow/getting_started_deploy.md) section from **Deploying Prow** tutorial. During configuration please be aware, that fields like ``Homepage URL`` or ``Callback URL`` don't matter that much in the test deployment. You can skip them, and if not possible, an invalid URL also should work. The most important field for this tutorial ``Webhook URL`` should be filled by following the instruction from the next paragraph.
* Install configured application into your project (``Settings -> Developer settings -> GitHub Apps -> YourAppName -> Edit -> Install App``)
* Install docker and kind or minikube. 

## Filling Webhook URL

The deployment will work on your localhost machine, but it needs to connect to the GitHub application configured in the previous section. There are several methods to do this with limited access to the public IP. The simplest would be to use dedicated services webhook services. A good example of such a service could be [smee.io](https://smee.io/). Such services should be used only for testing and development purposes. Please visit [smee.io/new](https://smee.io/new) to generate a new channel. Follow the instruction to install the tool (using ``npm``). Paste the generated link in the ``Webhook URL`` config field in your application configuration.

The ``Webhook URL`` should be configured with a secret. To generate a secret please run in the console:
```
$ openssl rand -hex 20
```
Save the result locally, and paste it in the field ``Webhook secret (optional)`` during GitHub App configuration.

## Deployment on kind or minikube

### kind preparation
Start a new kind cluster using:
```
kind create cluster
```
After a proper startup, you need IP address assigned to kind. To get IP assigned to kind you can execute the following command:
```
$ kubectl --context kind-kind  describe service kubernetes
```
IP is listed in `Endpoints:` section. If there is any port, ignore it. Finally, execute `smee` command. Taking `172.18.0.2` as an example IP run:
```
$ smee -u https://smee.io/YourWebhookHashHere --target 172.18.0.2:30003/hook
```
Apply prow CRDs using:
```
$ kubectl apply --context kind-kind --server-side=true -f https://raw.githubusercontent.com/kubernetes/test-infra/master/config/prow/cluster/prowjob-crd/prowjob_customresourcedefinition.yaml
```
### minikube preparation
Alternatively, of you use minikube:
```
$ minikube start
```
Due to a deticated `minikube ip` command, you do not need to inspect IP address assigned to minikube. Just run the following command:
```
$ smee -u https://smee.io/YourWebhookHashHere --target $(minikube ip):30003/hook
```
Apply prow CRDs using:
```
$ kubectl apply --server-side=true -f https://raw.githubusercontent.com/kubernetes/test-infra/master/config/prow/cluster/prowjob-crd/prowjob_customresourcedefinition.yaml
```

### Update the sample manifest
The sample manifest you need to use is [starter-s3-kind.yaml](config/prow/cluster/starter/starter-s3-kind.yaml).
There are several fields that has to be configured before applying the manifest:

* The github app cert by replacing the `<<insert-downloaded-cert-here>>` string
* The github app id by replacing the `<<insert-the-app-id-here>>` string
* The hmac token by replacing the `<< insert-hmac-token-here >>` string
* The repository by replacing the `<< your_github_repo >>` string (format: `user/repo`)
* The IP address by replacing the `<< your-minikube/kind IP >>` string
* The minio's root account by replacing the `<<CHANGE_ME_MINIO_ROOT_USER>>` string in 3 places
* The minio's secret by replacing the `<<CHANGE_ME_MINIO_ROOT_USER>>` string in 3 places

After filling all the fields, you need to apply the manifest. It can be done using:
```
$ kubectl apply -f starter-s3-kind.yaml 
```
A few minutes later, all the components should be running (some restarts are possible due to a slow minio init):
```
$ kubectl -- get pods -n prow
NAME                                       READY   STATUS    RESTARTS       AGE
crier-5db74d86c4-zkf4h                     1/1     Running   0              3m38s
deck-85d8978769-bvbvz                      1/1     Running   0              3m39s
ghproxy-5c5df8f459-kgxp7                   1/1     Running   0              3m38s
hook-7c9bf9b48b-vpkx7                      1/1     Running   0              3m38s
horologium-696596964b-gjwkn                1/1     Running   0              3m38s
minio-69fb9fff98-j59dq                     1/1     Running   0              3m37s
prow-controller-manager-7d8988546c-tc4dz   1/1     Running   0              3m38s
sinker-69f69c4f85-ssl8h                    1/1     Running   0              3m38s
statusreconciler-64f5c88b7-hfnxb           1/1     Running   0              3m38s
tide-74f555b5b5-65tsq                      1/1     Running   5 (115s ago)   3m38s
```

### Static ports for hook, deck and minio console
You can access ``hook`` using port ``30001``, ``deck`` UI is listening on port ``30002`` and ``minio`` UI on port ``30003``. Use ``minikube ip`` or the method described previously for kind to get the IP address. Taking `192.168.49.1` as an example IP address, go to the web browser and type `192.168.49.1:30002` to access `deck`.

### Test your deployment
The simplest way to test the deployment to see if the webhook is passing data and the components are interacting with the GitHub is to open a PR in your repository. At the bottom jobs should be visible if the CI is properly configured. You can also post some simple comments (like ``/meow`` or ``/bark``) to see if messages get the hook component.
There could be several problems, the list below will give some hints how to debug them:
* Check if your GitHub App is installed in your repository
* Check if webhook is still active. If it expired, paste the new one in the ``Webhook URL`` field and use it locally re-running ``smee``
* Check correctness of your IP, it can change after cluster recreation
