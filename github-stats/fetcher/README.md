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
curl -O https://curl.haxx.se/ca/cacert.pem
kubectl create configmap certificates --from-file=ca-certificates.crt=cacert.pem
```

Deployment
----------

Run the deploy script:
```
make deploy IMG=gcr.io/your-repo/fetcher
```
