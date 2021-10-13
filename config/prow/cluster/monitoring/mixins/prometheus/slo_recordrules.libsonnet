{
  prometheusAlerts+:: {
    local components = $._config.slo.components,

    groups+: [
      {
       name: 'SLO Compliance',
       interval: '1m',
       rules: [
          {
            record: 'slo_component_ok',
            # We can't check for the absence of alerts without explicitly listing the components we are checking for. These are defined in config.
            # We want the SLO metrics to include alert-specific labels when SLO is violated. This means there may be multiple time series per component when out of SLO since multiple alerts may be firing, but all should have value 0.
            # If a component is SLO compliant there will be a single timeseries for that "slo" label: vector(1){slo="component-name"}

            local absents = std.join(
              ' or ',
              ['absent(ALERTS{alertstate="firing", slo="%s"})' % comp for comp in components],
            ),
            local allCompsRE = std.join('|', components),

            expr: |||
              min((%s) or (ALERTS{alertstate="firing", slo=~"%s"} - 1)) without (alertstate)
            ||| % [absents, allCompsRE],

            # Example compiled query for components=['tide', 'hook']
            # min((absent(ALERTS{alertstate="firing", slo="tide"}) or absent(ALERTS{alertstate="firing", slo="hook"})) or (ALERTS{alertstate="firing", slo=~"tide|hook"} - 1)) without (alertstate)
          },
          {
            record: 'slo_prow_ok',
            expr: '(vector(1) unless min(slo_component_ok == 0)) or (slo_component_ok == 0)',
          },
       ],
      },
    ],
  },
}