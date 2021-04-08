{
  prometheusAlerts+:: {
    groups+: [
      {
        name: 'external-secret-sync',
        rules: [
          {
            # https://github.com/external-secrets/kubernetes-external-secrets/blob/master/README.md#metrics
            alert: 'Failed-syncing-external-secret',
            # Prometheus scrapes kubernetes external secrets every 30 seconds as defined in servicemonitor, so this counts failures between scrape intervals.
            # Since kubernetes secret manager runs every 10 seconds, there should be at least 2 runs in every 30s, so this will only report consecutive failures.
            expr: |||
              increase(kubernetes_external_secrets_sync_calls_count{job="kubernetes-external-secrets",status!="success"}[1m]) > 1.5
            |||,
            labels: {
              severity: 'user-warning',
            },
            annotations: {
              message: 'ExternalSecret {{ $labels.namespace }}/{{ $labels.name }} failed to be synced. does %s have `roles/secretmanager.viewer` and `roles/secretmanager.secretAccessor` permissions on the google secret manager secret used for this cluster secret?' % $._config.kubernetesExternalSecretServiceAccount,
            },
          }
        ],
      },
    ],
  },
}
