Deploying the fetcher
=====================

First time only
---------------

Create the github-tokens secret:
```
kubectl create secret generic github-tokens --from-literal=token_login_1=${your_1st_token} --from-literal=token_login_2=${your_2nd_token}
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
