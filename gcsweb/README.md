# gcsweb

gcsweb is a web frontend to Google Cloud Storage that uses the GCS API.

To access private buckets, the user has to specify a valid OAuth token with 
the `--oauth-token-file` flag.

You can build a docker image of it by running the following command:

```
bazel run //gcsweb/cmd/gcsweb:image
```
