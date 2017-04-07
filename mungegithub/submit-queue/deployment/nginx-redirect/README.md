## Nginx HTTP->HTTPS redirector

This service sits behind the main submit-queue load balancer and 
intercepts HTTP (as indicated by the `X-Forwarded-Proto` header)
and sends a 301 redirect to HTTPS.

## Deploying

### Create the config-map

```
kubectl create cm nginx-https-redirect --from-file=nginx.conf=nginx.conf
```

Note that the config is _not_ automatically updated, if you update the
`ConfigMap` you need to delete any existing `Pods` in the nginx
redirector to pick up the new config.

TODO: We should really version the ConfigMap and do this via rolling-update.

### Deploy nginx

```
kubectl create -f nginx-deploy.yaml
```

### Deploy the service
(this shouldn't really ever be needed)

```
kubectl create -f nginx-svc.yaml
```
