# ghProxy

ghProxy is a reverse proxy HTTP cache optimized for use with the GitHub API (https://api.github.com).
It is essentially just a reverse proxy wrapper around [ghCache](/ghproxy/ghcache) with Prometheus instrumentation to monitor disk usage.

ghProxy is designed to reduce API token usage by allowing many components to
share a single ghCache.

## with Prow

While ghProxy can be used with any GitHub API client, it was designed for Prow.
Prow's GitHub client request throttling is optimized for use with ghProxy and
doesn't count requests that can be fulfilled with a cached response against the
throttling limit.

Many Prow features (and soon components) require ghProxy in order to avoid
rapidly consuming the API rate limit. Direct your Prow components that use the
GitHub API (anything that requires the GH token secret) to use ghProxy and fall
back to using the upstream API by adding the following flags:

```yaml
--github-endpoint=http://ghproxy  # Replace this as needed to point to your ghProxy instance.
--github-endpoint=https://api.github.com
```

## Deploying

A new container image is automatically built and published to
[gcr.io/k8s-prow/ghproxy](https://gcr.io/k8s-prow/ghproxy) whenever this
directory is changed on the master branch. You can find a recent stable image
tag and an example of how to deploy ghProxy to Kubernetes by checking out
[Prow's ghProxy deployment](/config/prow/cluster/ghproxy.yaml).
