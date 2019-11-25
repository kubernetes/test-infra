JSONNET_FMT := jsonnet fmt -n 2 --max-blank-lines 2 --string-style s --comment-style s

all: fmt prometheus_alerts.yaml prometheus_rules.yaml dashboards_out lint test

fmt:
	find . -name 'vendor' -prune -o -name '*.libsonnet' -print -o -name '*.jsonnet' -print | \
		xargs -n 1 -- $(JSONNET_FMT) -i

prometheus_alerts.yaml: mixin.libsonnet lib/alerts.jsonnet alerts/*.libsonnet
	jsonnet -S lib/alerts.jsonnet > $@

prometheus_rules.yaml: mixin.libsonnet lib/rules.jsonnet rules/*.libsonnet
	jsonnet -S lib/rules.jsonnet > $@

dashboards_out: mixin.libsonnet lib/dashboards.jsonnet dashboards/*.libsonnet
	@mkdir -p dashboards_out
	jsonnet -J vendor -m dashboards_out lib/dashboards.jsonnet

lint: prometheus_alerts.yaml prometheus_rules.yaml
	find . -name 'vendor' -prune -o -name '*.libsonnet' -print -o -name '*.jsonnet' -print | \
		while read f; do \
			$(JSONNET_FMT) "$$f" | diff -u "$$f" -; \
		done

	promtool check rules prometheus_rules.yaml
	promtool check rules prometheus_alerts.yaml

clean:
	rm -rf dashboards_out prometheus_alerts.yaml prometheus_rules.yaml

test: prometheus_alerts.yaml prometheus_rules.yaml
	promtool test rules tests.yaml
