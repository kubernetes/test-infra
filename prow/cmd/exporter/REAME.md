# Exporter

The prow-exporter exposes metrics about prow jobs while the
metrics are not directly related to a specific prow-component.

## Metrics

| Metric name          | Metric type | Labels/tags                                                                                                                                                                                           |
|----------------------|-------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| prow_job_labels      | Gauge       | `prow_job`=&lt;prow_job-name&gt; <br> `namespace`=&lt;prow_job-namespace&gt; <br> `prow_job_agent`=&lt;prow_job-agent&gt; <br> `label_PROW_JOB_LABEL_KEY`=&lt;PROW_JOB_LABEL_VALUE&gt;                |
| prow_job_annotations | Gauge       | `prow_job`=&lt;prow_job-name&gt; <br> `namespace`=&lt;prow_job-namespace&gt; <br> `prow_job_agent`=&lt;prow_job-agent&gt; <br> `annotation_PROW_JOB_ANNOTATION_KEY`=&lt;PROW_JOB_ANNOTATION_VALUE&gt; |

For example, the metric `prow_job_labels` is similar to `kube_pod_labels` defined
in [kubernetes/kube-state-metrics](https://github.com/kubernetes/kube-state-metrics/blob/master/docs/pod-metrics.md).
A typical usage of `prow_job_labels` is to [join](https://github.com/kubernetes/kube-state-metrics/tree/master/docs#join-metrics)
it with other metrics using a [Prometheus matching operator](https://prometheus.io/docs/prometheus/latest/querying/operators/#vector-matching).
