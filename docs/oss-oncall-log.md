# OSS oncall log

"Oh yeah, I remember having to deal with that a while back... what was it again?"

See also: [post-mortems](./post-mortems.md)

- Issue: bigquery metrics dashboard in velodrome.k8s.io is out of date ([#8703](https://github.com/kubernetes/test-infra/issues/8703))
  - Symptoms:
    - velodrome alert fires when [BigQuery Ingress Rate](http://velodrome.k8s.io/dashboard/db/bigquery-metrics?orgId=1&panelId=12&fullscreen) falls to 0 for 6 hours
    - velodrome.k8s.io is out of date
    - go.k8s.io/triage is out of date
    - k8s-gubernator BigQuery table is out of date
  - Remediation:
    - Kettle froze and needed to be restarted
    - See [Kettle's README](/kettle/README.md)
