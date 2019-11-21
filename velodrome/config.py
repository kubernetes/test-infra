#!/usr/bin/env python3

# Copyright 2017 The Kubernetes Authors.
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

import os
import string
import sys

import ruamel.yaml as yaml

CONFIG = "config.yaml"

DEPLOYMENTS = {
    "fetcher": "fetcher/deployment.yaml",
    "transform": "transform/deployment.yaml",
    "influxdb": "grafana-stack/influxdb.yaml",
    "prometheus": "grafana-stack/prometheus.yaml",
    "prometheus-config": "grafana-stack/prometheus-config.yaml",
    "prober": "prober/blackbox.yaml",
    "grafana": "grafana-stack/grafana.yaml",
    "grafana-config": "grafana-stack/grafana-config.yaml",
    "nginx": "grafana-stack/nginx.yaml",
    "sqlproxy": "mysql/sqlproxy.yaml",
}


def main():
    if len(sys.argv) != 1:
        print("Too many arguments.", file=sys.stderr)
        sys.exit(128)

    with open(get_absolute_path(CONFIG)) as config_file:
        config = yaml.safe_load(config_file)
        print_deployments(["sqlproxy", "prober"], {})
        for project_name, project in config['projects'].items():
            public_ip = project.get('nginx', {}).get('public-ip', '') or ''
            print_deployments(["influxdb", "grafana", "nginx"], {
                "PROJECT": project_name,
                "NGINX_PUBLIC_IP": public_ip,
            })
            if 'prometheus' in project:
                print_deployments(["prometheus"], {
                    "PROJECT": project_name,
                })
                patch_configuration("prometheus-config",
                                    project['prometheus'],
                                    {"PROJECT": project_name})
            if 'grafana' in project:
                patch_configuration("grafana-config",
                                    project['grafana'],
                                    {"PROJECT": project_name})
            for repository, transforms in project['repositories'].items():
                print_deployments(["fetcher"], {
                    "GH_ORGANIZATION": repository.split("/")[0],
                    "GH_REPOSITORY": repository.split("/")[1],
                    "PROJECT": project_name,
                })
                for metric, transformer in (transforms or {}).items():
                    plugin = metric
                    if "plugin" in transformer:
                        plugin = transformer["plugin"]
                    args = []
                    if "args" in transformer:
                        args = transformer["args"]
                    apply_transform(args, {
                        "GH_ORGANIZATION": repository.split("/")[0],
                        "GH_REPOSITORY": repository.split("/")[1],
                        "PROJECT": project_name,
                        "TRANSFORM_PLUGIN": plugin,
                        "TRANSFORM_METRIC": metric,
                    })


def apply_transform(new_args, env):
    with open(get_absolute_path(DEPLOYMENTS["transform"])) as fp:
        config = yaml.safe_load(fp)
        config['spec']['template']['spec']['containers'][0]['args'] += new_args
    print_deployment(yaml.dump(config, default_flow_style=False), env)


def patch_configuration(component, values, env):
    with open(get_absolute_path(DEPLOYMENTS[component])) as fp:
        config = yaml.safe_load(fp)
        # We want to fail if we have unknown keys in values
        unknown_keys = set(values) - set(config['data'])
        if unknown_keys:
            raise ValueError("Unknown keys in config:", unknown_keys)
        config['data'] = values
    print_deployment(yaml.dump(config, default_flow_style=False), env)


def get_absolute_path(path):
    return os.path.join(os.path.dirname(__file__), path)


def print_deployments(components, env):
    for component in components:
        with open(get_absolute_path(DEPLOYMENTS[component])) as fp:
            print_deployment(fp.read(), env)


def print_deployment(deployment, env):
    print(string.Template(deployment).safe_substitute(**env), end='')
    print('---')

if __name__ == '__main__':
    main()
