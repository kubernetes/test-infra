{
  local default(obj, field, value)= {[field]: if field in obj then obj[field] else value},

  consts+:: {
    // Grafana dashboard IDs are necessary for stable links for dashboards
    grafanaDashboardIDs: {
      'boskos-http.json': 'eec46c579cbf4a518e5bbcbbf4913de9',
      'ghproxy.json': 'd72fe8d0400b2912e319b1e95d0ab1b3',
      'slo.json': 'ea313af4b7904c7c983d20d9572235a5',
    },
    // Component name constants
    components: {
      // Values should be lowercase for use with prometheus 'job' label.
      crier: 'crier',
      deck: 'deck',
      ghproxy: 'ghproxy',
      hook: 'hook',
      horologium: 'horologium',
      monitoring: 'monitoring', // Aggregate of prometheus, alertmanager, and grafana.
      plank: 'plank', // Mutually exclusive with prowControllerManager
      prowControllerManager: 'prow-controller-manager',
      sinker: 'sinker',
      tide: 'tide',
    },
  },

  defaultConfig(config): (
    self.consts +
    config +
    // Add defaulting logic to the struct below by using '+:' to deeply override fields.
    {
      instance+: default(config.instance, 'botName', '(Prow bot name)')
        + {
          monitoringLink(path, text): (
            if 'monitoringURL' in config.instance then
              '<%s%s|%s>' % [config.instance.monitoringURL, path, text]
            else
              '%s (Requires <https://github.com/kubernetes/test-infra/tree/master/config/prow/cluster/monitoring#access-components-web-page|port-forwarding the grafana service> and accessing path "%s")' % [text, path]
          ),
        },
    }
    + default(config, 'prowImageStaleByDays', 7)
  ),
}
