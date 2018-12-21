# ghProxy

ghProxy is a reverse proxy HTTP cache optimized for use with the GitHub API (https://api.github.com).
It is essentially just a reverse proxy wrapper around [ghCache](/ghproxy/ghcache) with some additional prometheus instrumentation logic to monitor disk usage and push metrics to a prometheus push gateway.

ghProxy is designed to reduce API token usage by allowing many components to share a single ghCache. Note that components must use the same API token to benefit from the cache and avoid clobbering existing cache entries for other tokens.

## How to use

To use ghProxy for your prow instance,

- Create a `StorageClass` using [`gce-ssd-retain_storageclass.yaml`](/prow/cluster/gce-ssd-retain_storageclass.yaml)
if on GCE or specify your own storage class.
- Create a `PersistentVolumeClaim` and `Deployment` using the config [here](prow/cluster/ghproxy_deployment.yaml).
- Finally, create a [`Service`](/prow/cluster/ghproxy_service.yaml).
- In the deployments for your prow components, use `--github-endpoint=http://ghproxy` as shown
[here](https://github.com/kubernetes/test-infra/blob/6cd6c2332d493b4d141822cbaa3ffe5806f64825/prow/cluster/hook_deployment.yaml#L43).
To make sure that the client is able to bypass cache if it is temporarily unavailable,
you can additionally use `--github-endpoint=https://api.github.com`. The endpoints are used in the order
they are mentioned, and if the preceeding endpoints return a conn err.
