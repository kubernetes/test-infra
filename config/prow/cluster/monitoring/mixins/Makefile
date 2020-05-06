# Copyright 2016 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# The targets defined here are only for local testing and debuging
# There is no intention of using it in CI/CD pipeline for the moment

SHELL=/bin/bash -o pipefail

all: grafana-dashboards prow_prometheusrule.yaml

grafana-dashboards:
	for name in deck ghproxy hook plank prow sinker tide; do echo "Generating $${name}.json ..."; jsonnet -J vendor/ -J lib/ "grafana_dashboards/$${name}.jsonnet" > "./dashboards_out/$${name}.json"; done

prow_prometheusrule.yaml:
	@echo "Generating prow_prometheusrule.yaml ..."
	jsonnet -J lib/ ./prometheus/prow_prometheusrule.jsonnet | gojsontoyaml > prometheus_out/$@

clean-vendor:
	rm -rfv ./vendor

clean:
	rm -rfv ./dashboards_out/*.json
	rm -rfv ./prometheus_out/*.json
	rm -rfv ./prometheus_out/*.yaml

install:
	jb install
