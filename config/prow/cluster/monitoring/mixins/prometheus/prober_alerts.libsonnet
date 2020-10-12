{
  prometheusAlerts+:: {
    local comps = $._config.components,
    groups+: [
      {
        name: 'Blackbox Prober',
        rules: [
          {
            alert: 'Site unavailable: %s' % target.url,
            expr: |||
              min(probe_success{instance="%s"}) == 0
            ||| % target.url,
            'for': '2m', # I think this needs to be at least the scrape_interval and 2*evaluation_interval (which both default to 1m) in order to ignore individual probe failures.
            labels: {
              severity: 'critical',
            } + target.labels,
            annotations: {
              message: 'The blackbox_exporter HTTP probe has detected that the following site has been unhealthy (not 2xx HTTP response) for at least 2 minutes: <%s|%s>.' % [target.url, target.url],
            },
          }
          for target in [
          # ATTENTION: Keep this in sync with the list in ../../additional-scrape-configs_secret.yaml
            {url: 'https://prow.k8s.io', labels: {slo: comps.deck}},
            {url: 'https://monitoring.prow.k8s.io', labels: {slo: comps.monitoring}},
            {url: 'https://testgrid.k8s.io', labels: {}},
            {url: 'https://gubernator.k8s.io', labels: {}},
            {url: 'https://gubernator.k8s.io/pr/fejta', labels: {}}, # Deep health check of someone's PR dashboard.
            {url: 'https://storage.googleapis.com/k8s-gubernator/triage/index.html', labels: {}},
            {url: 'https://storage.googleapis.com/test-infra-oncall/oncall.html', labels: {}},
          ]
        ],
      },
    ],
  },
}
