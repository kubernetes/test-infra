Deploying the fetcher
=====================

First time only
---------------

Create the github-token secret:
```
kubectl create secret generic github-token --from-literal=token=${your_token}
```

And install the certificate if you haven't done it yet:
```
kubectl create configmap certificates --from-file=/etc/ssl/certs/ca-certificates.crt
```

Deployment
----------

Run the deploy script:
```
make deploy IMG=gcr.io/your-repo/fetcher
```
