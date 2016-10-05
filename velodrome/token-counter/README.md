Token-counter is a small program that polls github to see how much of a token
ratelimit has been used.

Because github doesn't report how much you've used, but how much you have left
before the next reset, we need to get the value just before the reset.  In order
to do that, we poll more and more agressively as we get near to the reset
time. Once the reset happen, we can compute the difference between `Limit` and
`Remaining` and this tells us how much we used in the last hour.

Deploying
---------

token-counter is fairly easy to deploy, and can keep track of multiple keys at
the same time. The result will be pushed to an influx database.

Make a `github-tokens` secret with each of your tokens in there:
```
kubectl create secret generic github-tokens --from-file=token1=token1 ...
```

Change `deployment.yaml` to have `--token=token1` for each of your token. Also
make sure the influxdb is properly configured.

Deploy the token-counter:
```
kubectl apply -f deployment.yaml
```
