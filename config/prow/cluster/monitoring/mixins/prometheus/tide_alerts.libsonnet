{
  prometheusAlerts+:: {
    local tideName = $._config.components.tide,
    local monitoringLink = $._config.instance.monitoringLink,
    local prowURL = $._config.instance.url,
    groups+: [
      {
        name: 'Tide progress',
        rules: [
          {
            alert: 'Sync controller heartbeat',
            expr: |||
              sum(increase(tidesyncheartbeat{controller="sync"}[15m])) < 1
            |||,
            'for': '5m',
            labels: {
              severity: 'critical',
              slo: tideName,
            },
            annotations: {
              message: 'The Tide "sync" controller has not synced in 15 minutes. See the %s.' % monitoringLink('/d/d69a91f76d8110d3e72885ee5ce8038e/tide-dashboard?orgId=1&from=now-24h&to=now&fullscreen&panelId=7', 'processing time graph'),
            },
          },
          {
            alert: 'Status-update controller heartbeat',
            expr: |||
              sum(increase(tidesyncheartbeat{controller="status-update"}[30m])) < 1
            |||,
            'for': '5m',
            labels: {
              severity: 'critical',
              slo: tideName,
            },
            annotations: {
              message: 'The Tide "status-update" controller has not synced in 30 minutes. See the %s.' % monitoringLink('/d/d69a91f76d8110d3e72885ee5ce8038e/tide-dashboard?orgId=1&from=now-24h&to=now&fullscreen&panelId=7', 'processing time graph'),
            },
          },
          {
            alert: 'TidePool error rate: individual',
            expr: |||
              ((sum(increase(tidepoolerrors{org!="kubeflow"}[10m])) by (org, repo, branch)) or vector(0)) >= 3
            |||,
            'for': '5m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              message: 'At least one Tide pool encountered 3+ sync errors in a 10 minute window. If the TidePoolErrorRateMultiple alert has not fired this is likely an isolated configuration issue. See the <%s/tide-history|/tide-history> page and the %s.' % [prowURL, monitoringLink('/d/d69a91f76d8110d3e72885ee5ce8038e/tide-dashboard?orgId=1&fullscreen&panelId=6&from=now-24h&to=now', 'sync error graph')],
            },
          },
          {
            alert: 'TidePool error rate: multiple',
            expr: |||
              (count(sum(increase(tidepoolerrors[10m])) by (org, repo) >= 3) or vector(0)) >= 3
            |||,
            'for': '5m',
            labels: {
              severity: 'critical',
              slo: tideName,
            },
            annotations: {
              message: 'Tide encountered 3+ sync errors in a 10 minute window in at least 3 different repos that it handles. See the <%s/tide-history|/tide-history> page and the %s.' % [prowURL, monitoringLink('/d/d69a91f76d8110d3e72885ee5ce8038e/tide-dashboard?orgId=1&fullscreen&panelId=6&from=now-24h&to=now', 'sync error graph')],
            },
          },
        ],
      },
    ],
  },
}
