local alerts = (import 'prometheus.libsonnet').prometheusAlerts;

{
	"apiVersion": "monitoring.coreos.com/v1",
	"kind": "PrometheusRule",
	"metadata": {
		"labels": {
			"prometheus": "prow",
			"role": "alert-rules"
		},
		"name": "prometheus-prow-rules",
		"namespace": "prow-monitoring"
	},
	"spec": alerts
}
