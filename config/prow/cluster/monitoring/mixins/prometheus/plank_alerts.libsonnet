{
  prometheusAlerts+:: {
    local componentName = $._config.components.plank,
    groups+: [
      {
        name: 'Heartbeat ProwJobs',
        # To add more heartbeat PJs add entries to `heartbeatJobs` in config.libsonnet
        # NOTE: These alerts are associated with plank, but may be
        #       triggered by problems with horologium or the pod utils.
        rules: [
          {
            alert: 'No recent successful runs: `%s`' % job.name,

            # This query counts the number of PJs with the specified name that
            # transitioned to the success state in the last job.alertInterval
            # amount of time. If that number is < 1 we return a result causing
            # the alert to fire. (We use 0.5 instead of 1 because query
            # results are not precise integers due to how prometheus interpolates.)
            expr: |||
              sum(increase(prowjob_state_transitions{job_name="%s", state="success"}[%s])) < 0.5
            ||| % [job.name, job.alertInterval],
            labels: {
              severity: 'critical',
              slo: componentName,
            },
            annotations: {
              message: '@test-infra-oncall The heartbeat job `%s` has not had a successful run in the past %s (should run every %s).' % [job.name, job.alertInterval, job.interval],
            },
          }
          for job in $._config.heartbeatJobs
        ],
      },
    ],
  },
}
