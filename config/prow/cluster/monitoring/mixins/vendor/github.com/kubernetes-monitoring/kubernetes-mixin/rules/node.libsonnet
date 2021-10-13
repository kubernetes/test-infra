{
  _config+:: {
    kubeStateMetricsSelector: 'job="kube-state-metrics"',
    nodeExporterSelector: 'job="node-exporter"',
    podLabel: 'pod',
  },

  prometheusRules+:: {
    groups+: [
      {
        name: 'node.rules',
        rules: [
          {
            // Number of nodes in the cluster
            // SINCE 2018-02-08
            record: ':kube_pod_info_node_count:',
            expr: |||
              sum(min(kube_pod_info{node!=""}) by (%(clusterLabel)s, node))
            ||| % $._config,
          },
          {
            // This rule results in the tuples (node, namespace, instance) => 1.
            // It is used to calculate per-node metrics, given namespace & instance.
            // We use the topk() aggregator to ensure that each (namespace,
            // instance) tuple is only associated to one node and thus avoid
            // "many-to-many matching not allowed" errors when joining with
            // other timeseries on (namespace, instance). See node:node_num_cpu:sum
            // below for instance.
            record: 'node_namespace_pod:kube_pod_info:',
            expr: |||
              topk by(namespace, %(podLabel)s) (1,
                max by (node, namespace, %(podLabel)s) (
                  label_replace(kube_pod_info{%(kubeStateMetricsSelector)s,node!=""}, "%(podLabel)s", "$1", "pod", "(.*)")
              ))
            ||| % $._config,
          },
          {
            // This rule gives the number of CPUs per node.
            record: 'node:node_num_cpu:sum',
            expr: |||
              count by (%(clusterLabel)s, node) (sum by (node, cpu) (
                node_cpu_seconds_total{%(nodeExporterSelector)s}
              * on (namespace, %(podLabel)s) group_left(node)
                node_namespace_pod:kube_pod_info:
              ))
            ||| % $._config,
          },
          // Add separate rules for Available memory, so we can aggregate across clusters in dashboards.
          {
            record: ':node_memory_MemAvailable_bytes:sum',
            expr: |||
              sum(
                node_memory_MemAvailable_bytes{%(nodeExporterSelector)s} or
                (
                  node_memory_Buffers_bytes{%(nodeExporterSelector)s} +
                  node_memory_Cached_bytes{%(nodeExporterSelector)s} +
                  node_memory_MemFree_bytes{%(nodeExporterSelector)s} +
                  node_memory_Slab_bytes{%(nodeExporterSelector)s}
                )
              ) by (%(clusterLabel)s)
            ||| % $._config,
          },
        ],
      },
    ],
  },
}
