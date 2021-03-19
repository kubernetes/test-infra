# PROW_INSTANCE_SPECIFIC
# Contains list of alerts to be included, could be different among prow instances
(import 'config.libsonnet') +
(import 'ci_absent_alerts.libsonnet') +
(import 'prow_monitoring_absent_alerts.libsonnet') +
(import 'configmap_alerts.libsonnet') +
(import 'ghproxy_alerts.libsonnet') +
(import 'hook_alert.libsonnet') +
(import 'sinker_alerts.libsonnet') +
(import 'stale_alerts.libsonnet') +
(import 'tide_alerts.libsonnet') +
(import 'prober_alerts.libsonnet') +
(import 'boskos_alerts.libsonnet') +
(import 'plank_alerts.libsonnet') +
(import 'slo_recordrules.libsonnet') +
(import 'prow_alerts.libsonnet')
