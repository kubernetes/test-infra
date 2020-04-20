# gcsweb

gcsweb is a web frontend to Google Cloud Storage that uses the public,
no-login-required API. Obviously this means it can only browse public buckets.

You can build a docker image of it by running the following command:

```
bazel run //gcsweb/cmd/gcsweb:image
```
