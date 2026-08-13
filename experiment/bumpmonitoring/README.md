# bumpmonitoring

This package is used for copying changes from k8s prow monitoring stacks, more
specifically mixins to other prow monitoring stacks.

This is based on the assumption that:

- Configs under
  [`mixins/prometheus`](./config/prow/cluster/monitoring/mixins/prometheus)
  are general enough that doesn't contain any k8s prow specific information
- Prow specific values are configured in
  [`mixins/lib`](./config/prow/cluster/monitoring/mixins/lib)
- Downstream prow monitoring stacks maintain the same directory layout under
  `mixins` as k8s prow

This tool will:

- Copy files(Except for `prometheus.libsonnet`, explained below) from
  [`mixins/prometheus`](./config/prow/cluster/monitoring/mixins/prometheus) to
  other prow instances, skip the files that don't exist in downstream prow
  (Downstream prow will need to manually add the new file, which will be bumped
  by this tool afterwards)
- For `mixins/prometheus/prometheus.libsonnet`, it's essentially a file
  explicitly importing other files under this directory, no change should be
  expected in this file as the tool doesn't add/remove files
- Create PR
