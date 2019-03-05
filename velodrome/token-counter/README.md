Overview
========

Token-counter is a program that polls github to see how much of a token's
ratelimit has been used.

GitHub doesn't report how many requests you've used, and instead reports
how many request you have left before the rate-limit resets.
In order to do that, we poll more and more aggressively as we get near to the reset
time. Once the reset happens, we compute the difference between `Limit` and
`Remaining` giving us how much we've used in the last hour.

Deploying
=========

token-counter is fairly easy to deploy, and can keep track of multiple keys at
the same time. The result will be pushed to an InfluxDB.

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
