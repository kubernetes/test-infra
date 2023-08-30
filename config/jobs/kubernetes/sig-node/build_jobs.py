# Copyright 2023 The Kubernetes Authors.
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

import math
import json
import re
import yaml
import jinja2  # pylint: disable=import-error


from helpers import (  # pylint: disable=import-error, no-name-in-module
    build_cron,
)

# These are job tab names of unsupported grid combinations
skip_jobs = []

image = "gcr.io/k8s-staging-test-infra/kubekins-e2e:v20230727-ea685f8747-master"

loader = jinja2.FileSystemLoader(searchpath="./templates")

##############
# Build Test #
##############


# Returns a string representing the periodic prow job and the number of job invocations per week
def build_test(
    cloud="ec2",
    distro="u2204",
    cri="containerd",
    name_override=None,
    feature_flags=(),
    extra_dashboards=None,
    test_parallelism=8,
    test_timeout_minutes=60,
    skip_regex="",
    arch="",
    test_args="",
    instance_type="",
    user_data_file="",
    image_config_file="",
    test_type="node-e2e",
    scenario_name=None,
    focus_regex=None,
    runs_per_day=0,
    scenario=None,
    env=None,
):
    # pylint: disable=too-many-statements,too-many-branches,too-many-arguments
    validation_wait = "20m" if distro in ("flatcar", "flatcararm64") else None

    suffix = ""
    if cri:
        suffix += cri
    if distro:
        suffix += "-" + distro.replace('-arm64','')
    if arch:
        suffix += "-" + arch
    if cloud:
        suffix += "-" + cloud

    tab = name_override or (f"ci-node-e2e-{suffix}-{scenario_name}-release-master")
    #TODO amend this when the generator configures older releases

    if tab in skip_jobs:
        return None

    cron, runs_per_week = build_cron(tab, runs_per_day)

    # Scenario-specific parameters
    if env is None:
        env = {}

    tmpl_file = "periodic.yaml.jinja"

    tmpl = jinja2.Environment(loader=loader).get_template(tmpl_file)
    job = tmpl.render(
        job_name=tab,
        cloud=cloud,
        cron=cron,
        distro=distro,
        test_parallelism=str(test_parallelism),
        job_timeout=str(test_timeout_minutes + 10) + "m",
        test_timeout=str(test_timeout_minutes) + "m",
        skip_regex=skip_regex,
        kops_feature_flags=",".join(feature_flags),
        focus_regex=focus_regex,
        validation_wait=validation_wait,
        test_args=test_args,
        user_data_file=user_data_file,
        image_config_file=image_config_file,
        instance_type=instance_type,
        image=image,
        scenario=scenario,
        env=env,
        arch=arch,
    )

    spec = {
        "cloud": cloud,
        "distro": distro,
        "cri": cri,
        "scenario": scenario_name,
    }
    jsonspec = json.dumps(spec, sort_keys=True)

    dashboards = [
        f"sig-node-{cri}",
        f"sig-node-{distro.removesuffix('-arm64')}",
    ]
    if cloud == "ec2":
        dashboards.extend(["sig-node-ec2","amazon-ec2-node"])
    if cloud == "gce":
        dashboards.extend(["sig-node-gce"])

    if extra_dashboards:
        dashboards.extend(extra_dashboards)

    days_of_results = 90
    if runs_per_week * days_of_results > 2000:
        # testgrid has a limit on number of test runs to show for a job
        days_of_results = math.floor(2000 / runs_per_week)
    annotations = {
        "testgrid-dashboards": ", ".join(sorted(dashboards)),
        "testgrid-days-of-results": str(days_of_results),
        "testgrid-tab-name": tab,
    }
    for k, v in spec.items():
        annotations[f"node.k8s.io/{k}"] = v or ""

    extra = yaml.dump(
        {"annotations": annotations}, width=9999, default_flow_style=False
    )

    output = f"\n# {jsonspec}\n{job.strip()}\n"
    for line in extra.splitlines():
        output += f"  {line}\n"
    return output, runs_per_week

####################
# Grid Definitions #
####################
clouds = [
    "ec2",
    "gce",
]

cri_options = [
    "containerd",
    # 'cri-o',
]

gce_distro_options = [
    "cos",
    "ubuntu2204-gke",
    "ubuntu2204-gke-arm64",
]

ec2_distro_options = [
    # "amazonlinux2",
    "al2023",
    "al2023-arm64",
    "ubuntu2204",
    "ubuntu2204-arm64",
]

image_config_files = {
    "cos": "../test-infra/jobs/e2e_node/containerd/image-config.yaml",
    "ubuntu2204-gke": "../test-infra/jobs/e2e_node/containerd/image-config-ubuntu2204.yaml",
    "ubuntu2204-gke-arm64": "../test-infra/jobs/e2e_node/containerd/image-config-ubuntu2204-gke-arm64.yaml",
}

user_data_files = {
    # "amazonlinux2": "amazonlinux2.yaml", # stored in k-sigs/aws-provider-test-infra repo
    # "amazonlinux2-arm64": "amazonlinux2-arm64.yaml", # stored in k-sigs/aws-provider-test-infra repo
    "ubuntu2204": "config/ubuntu2204.yaml", # stored in k-sigs/aws-provider-test-infra repo
    "ubuntu2204-arm64": "config/ubuntu2204.yaml", # stored in k-sigs/aws-provider-test-infra repo
    "al2023": "config/al2023-6.1.yaml", # stored in k-sigs/aws-provider-test-infra repoo
    "al2023-arm64": "config/al2023-6.1.yaml", # stored in k-sigs/aws-provider-test-infra repo
}

default_test_args = '--kubelet-flags="--cgroup-driver=systemd"'

test_scenarios = [
    {
        "name": "conformance",
        "skip_regex": r"\[Slow\]|\[Serial\]|\[Flaky\]",
        "focus_regex": r"\[NodeConformance\]",
        "cloud": "all",
        "test_args": default_test_args,
        "release_informing": ["cos-gce", "ubuntu2204-ec2"],
    },
    {
        "name": "serial",
        "skip_regex": r"\[Flaky\]|\[Benchmark\]|\[NodeSpecialFeature:.+\]|\[NodeSpecialFeature\]|\[NodeAlphaFeature:.+\]|\[NodeAlphaFeature\]|\[NodeFeature:Eviction\]",
        "focus_regex": r"\[Serial\]",
        "parallelism": 1,
        "timeout": 240,
        "cloud": "all",
        "test_args": default_test_args,
        "release_informing": ["cos-gce", "ubuntu2204-ec2"],
    },
    {
        "name": "features",
        "skip_regex": r"\[Flaky\]|\[Serial\]",
        "focus_regex": r"\[NodeFeature:.+\]|\[NodeFeature\]",
        "cloud": "all",
        "test_args": default_test_args,
    },
    {
        "name": "cgroupv1",
        "skip_regex": r"\[Flaky\]|\[Serial\]",
        "focus_regex": r"\[NodeFeature:.+\]|\[NodeFeature\]",
        "cloud": "all",
        "test_args": '--kubelet-flags="--cgroup-driver=cgroupfs"',
    },
    {
        "name": "swap",
        "skip_regex": r"\[Flaky\]|\[Serial\]",
        "focus_regex": r"\[NodeFeature:.+\]|\[NodeFeature\]",
        "cloud": "all",
        "timeout": 200,
        "test_args": '--feature-gates=NodeSwap=true --kubelet-flags=" --fail-swap-on=false"',
    },
    {
        "name": "standalone-all",
        "skip_regex": r"\[Flaky\]|\[Serial\]",
        "focus_regex": r"\[Feature:StandaloneMode\]",
        "cloud": "all",
        "timeout": 200,
        "test_args": '--standalone-mode=true --feature-gates=AllAlpha=true --kubelet-flags="--cgroup-driver=systemd"',
    }
]

# cri_versions = [
#     "containerd/main",
#     "containerd/1.7",
#     "containerd/1.6",
# ]

############################
# kops-periodics-grid.yaml #
############################
def generate_grid():
    results = []
    # pylint: disable=too-many-nested-blocks
    for cri in cri_options:
        for test_scenario in test_scenarios:
            for cloud in clouds:
                if cloud != test_scenario["cloud"] and test_scenario["cloud"] != "all":
                    continue
                for distro in eval(f"{cloud}_distro_options"):
                    distro_short = distro.replace('ubuntu', 'u').replace('debian', 'deb').replace('amazonlinux', 'amzn') # pylint: disable=line-too-long
                    arch = instance_type = user_data_file = image_config_file = ""
                    if cloud == "ec2":
                        instance_type = "m6a.large"
                        user_data_file = user_data_files[distro]
                    else:
                        instance_type = "e2-standard-2"
                        image_config_file = image_config_files[distro]
                    if 'arm64' in distro:
                        arch = "arm64"
                        if cloud == "ec2":
                            instance_type = "m6g.large"
                            user_data_file = user_data_files[distro]
                        else:
                            instance_type = "t2a-standard-2"
                            image_config_file = image_config_files[distro]
                    else:
                        arch = "amd64"
                    extra_dashboards = ["sig-node-grid"]
                    if "release_blocking" in test_scenario:
                        for v in test_scenario["release_blocking"]:
                            if re.match(fr"{distro}-{cloud}", v):
                                extra_dashboards.extend(['sig-release-master-blocking', 'sig-node-release-blocking'])
                    if "release_informing" in test_scenario:
                        for v in test_scenario["release_informing"]:
                            if re.match(fr"{distro}-{cloud}", v):
                                extra_dashboards.extend(['sig-release-master-informing'])
                    results.append(
                        build_test(
                            cloud=cloud,
                            distro=distro_short,
                            test_parallelism=test_scenario.get("parallelism", 8),
                            test_timeout_minutes=test_scenario.get("timeout", 60),
                            runs_per_day=3,
                            extra_dashboards=extra_dashboards,
                            scenario_name=test_scenario["name"],
                            cri=cri,
                            arch=arch,
                            instance_type=instance_type,
                            user_data_file=user_data_file,
                            image_config_file=image_config_file,
                            test_args=test_scenario["test_args"],
                            skip_regex=test_scenario["skip_regex"],
                            focus_regex=test_scenario["focus_regex"],
                        )
                    )

    return filter(None, results)

########################
# YAML File Generation #
########################
periodics_files = {
    "sig-node-grid.yaml": generate_grid,
}


def main():
    for filename, generate_func in periodics_files.items():
        print(f"Generating {filename}")
        output = []
        runs_per_week = 0
        job_count = 0
        for res in generate_func():
            output.append(res[0])
            runs_per_week += res[1]
            job_count += 1

        output.insert(
            0, "# yamllint disable rule:trailing-spaces\n"
        )
        output.insert(
            1, "# Test jobs generated by build_jobs.py (do not manually edit)\n"
        )
        output.insert(
            2, f"# {job_count} jobs, total of {runs_per_week} runs per week\n"
        )
        output.insert(2, "periodics:\n")
        with open(filename, "w") as fd:
            fd.write("".join(output))


if __name__ == "__main__":
    main()
