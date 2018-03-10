# fallbackcheck

Ensure your GCS bucket layout is what `tot` expects to use. Useful when you want to transition
from versioning your GCS buckets away from Jenkins build numbers to build numbers vended
by prow. 

`fallbackcheck` checks the existence of latest-build.txt files as per the [documented GCS layout][1].
It ignores jobs that have no GCS buckets.

## Install

```shell
go get k8s.io/test-infra/prow/cmd/tot/fallbackcheck
```

## Run

```shell
fallbackcheck -bucket GCS_BUCKET -prow-url LIVE_DECK_DEPLOYMENT
```

For example:
```shell
fallbackcheck -bucket https://gcsweb-ci.svc.ci.openshift.org/gcs/origin-ci-test/ -prow-url https://deck-ci.svc.ci.openshift.org/
```

[1]: https://github.com/kubernetes/test-infra/tree/master/gubernator#gcs-bucket-layout