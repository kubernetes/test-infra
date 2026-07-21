#!/usr/bin/env python3

# Copyright The Kubernetes Authors.
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

"""Generates the agent-sandbox prow job configs from an agent-sandbox checkout.

Jobs are derived from the scripts in the checkout:
    dev/ci/presubmits/<name>  -> presubmit job
    dev/ci/periodics/<name>   -> periodic job

Each job gets one of two profiles:
  - e2e:   privileged, docker-in-docker, run via runner.sh. Chosen when the
           script name contains "e2e" or "benchmark". Periodics are always
           e2e. Scripts whose name contains "gcp" additionally get the
           GCP flavor (boskos service account, ssh keys, and the
           k8s-infra-prow-build cluster); other e2e jobs are kind-based.
  - local: plain unprivileged container (unit tests, linters, codegen).

Bespoke per-job settings (legacy job names, run_if_changed patterns,
resources, periodic intervals) live in the OVERRIDES tables below.

Usage:
    ./generate_jobs.py --agent-sandbox-dir=<path to an agent-sandbox checkout>
"""

import argparse
import pathlib
import re
import sys

import yaml

IMAGE = "us-central1-docker.pkg.dev/k8s-staging-test-infra/images/kubekins-e2e:v20260712-9b391474f6-master"  # pylint: disable=line-too-long
DASHBOARD = "sig-apps-agent-sandbox"
REPO_ORG = "kubernetes-sigs"
REPO_NAME = "agent-sandbox"

# Skip presubmits when only docs/site/metadata changed.
SKIP_DOCS = (
    r"^(docs|site|\.agents)/|\.md$"
    r"|^(\.gitignore|LICENSE|netlify\.toml|OWNERS|\.coderabbit\.yaml|SECURITY_CONTACTS|cloudbuild\.yaml)$"  # pylint: disable=line-too-long
    r"|^\.github/"
)
# Variant that still runs for site/ changes (used by the autogen check).
SKIP_DOCS_ALLOW_SITE = (
    r"^(docs|\.agents)/|\.md$"
    r"|^(\.gitignore|LICENSE|netlify\.toml|OWNERS|\.coderabbit\.yaml|SECURITY_CONTACTS|cloudbuild\.yaml)$"  # pylint: disable=line-too-long
    r"|^\.github/"
)

# Helpers to build/modify job properties.

def set_run_if_changed(job, pattern):
    job.pop("skip_if_only_changed", None)
    job["run_if_changed"] = pattern


def rename_job(job, name, tab=None):
    job["name"] = name
    if "annotations" in job:
        if "testgrid-tab-name" in job["annotations"] or tab:
            job["annotations"]["testgrid-tab-name"] = tab or name


def add_label(job, key, value):
    if "labels" not in job:
        job["labels"] = {}
    job["labels"][key] = value


# Per-job override functions to mutate the job configuration.

def presubmit_test_autogen_up_to_date(job):
    rename_job(job, "presubmit-test-autogen-up-to-date")
    job["skip_if_only_changed"] = SKIP_DOCS_ALLOW_SITE
    add_label(job, "preset-service-account", "true")
    job["annotations"].update({
        "description": "presubmit-test-autogen-up-to-date",
        "testgrid-num-columns-recent": "30",
    })


# The renames below pin job names that predate the generator: renaming a
# presubmit churns branch-protection contexts (open PRs need /retest) and
# restarts testgrid tab history, so any move to the uniform
# presubmit-<script> naming should be its own deliberate change.

def presubmit_test_unit(_job):
    pass


def presubmit_lint_go(job):
    set_run_if_changed(job, r"^.*\.go$|^go\.(mod|sum)$|^dev/ci/presubmits/lint-go$")


def presubmit_lint_api(job):
    set_run_if_changed(job, r"^(extensions/)?api/|^dev/(ci/presubmits|tools)/lint-api|^dev/tools/(build-kal|\.golangci-kal\.yml|\.custom-gcl\.yaml)$")  # pylint: disable=line-too-long


def presubmit_test_e2e(_job):
    pass


def manual_presubmit(job):
    """Never runs automatically and never blocks merge; invoke with /test."""
    job["always_run"] = False
    job["optional"] = True


def optional_presubmit(job):
    """Runs automatically but does not block merge."""
    job["optional"] = True


def presubmit_benchmarks_kops_gcp_claims(job):
    """Optional, and auto-runs only on PRs that can affect warm-pool claim
    adoption -- the extensions controllers/APIs plus the controller binary,
    shared controllers, and internal packages the adoption path runs through
    -- or the benchmark itself; anything else can still trigger it with
    /test."""
    optional_presubmit(job)
    set_run_if_changed(job, r"^extensions/|^cmd/|^controllers/|^internal/|^test/stress/|^test/benchmarks/|^dev/ci/(presubmits|periodics)/benchmarks-kops-gcp")  # pylint: disable=line-too-long


PRESUBMIT_OVERRIDES = {
    "test-autogen-up-to-date": presubmit_test_autogen_up_to_date,
    "test-unit": presubmit_test_unit,
    "lint-go": presubmit_lint_go,
    "lint-api": presubmit_lint_api,
    "test-e2e": presubmit_test_e2e,
    # Expensive full-cluster benchmarks.
    "benchmarks-kops-gcp-cilium": optional_presubmit,
    "benchmarks-kops-gcp-claims": presubmit_benchmarks_kops_gcp_claims,
    "benchmarks-kops-gcp-kindnet": optional_presubmit,
    # Not wired up in prow before this generator existed; keep manual until
    # its credential requirements are sorted out.
    "test-skill-eval": manual_presubmit,
}


def periodic_test_load_test(job):
    rename_job(job, "periodic-agent-sandbox-perf-load-test")
    job["interval"] = "6h"


def periodic_test_migration(job):
    rename_job(job, "periodic-agent-sandbox-migration-test")
    job["interval"] = "1h"
    job["decoration_config"] = {"timeout": "30m"}


def periodic_24h(job):
    job["interval"] = "24h"


PERIODIC_OVERRIDES = {
    "test-load-test": periodic_test_load_test,
    "test-migration": periodic_test_migration,
    "benchmarks-kops-gcp-cilium": periodic_24h,
    "benchmarks-kops-gcp-claims": periodic_24h,
    "benchmarks-kops-gcp-kindnet": periodic_24h,
}

DEFAULT_PERIODIC_INTERVAL = "24h"


def is_e2e(script_name, periodic):
    # Periodics always need a cluster; presubmits are e2e when the name
    # says so.
    return periodic or re.search(r"e2e|benchmark", script_name) is not None


def is_gcp(script_name):
    return "gcp" in script_name


# Scripts that predate the generator, in the order the hand-written config
# listed them. Purely cosmetic: keeps the generated files ordered like the
# originals (new scripts append after, alphabetically) so diffs stay small.
LEGACY_PRESUBMIT_ORDER = ["test-autogen-up-to-date", "test-unit", "lint-go", "lint-api", "test-e2e"]
LEGACY_PERIODIC_ORDER = ["test-load-test", "test-migration"]


def discover(srcdir, legacy_order):
    if not srcdir.is_dir():
        return []
    names = [
        p.name for p in srcdir.iterdir()
        if p.is_file() and p.stat().st_mode & 0o111
    ]
    def sort_key(name):
        if name in legacy_order:
            return (0, legacy_order.index(name))
        return (1, name)
    return sorted(names, key=sort_key)


def container(script_path, e2e, resources):
    c = {
        "image": IMAGE,
        "command": ["runner.sh", script_path] if e2e else [script_path],
    }
    if e2e:
        c["securityContext"] = {"privileged": True}
    c["resources"] = resources
    return c

def resources_for_e2e():
    return {
        "requests": {"cpu": 7, "memory": "14Gi"},
        "limits": {"cpu": 7, "memory": "14Gi"},
    }

def resources_for_local_test():
    return {
        "requests": {"cpu": 4, "memory": "8Gi"},
        "limits": {"cpu": 4, "memory": "8Gi"},
    }

def build_presubmit(script_name, override_func=None):
    e2e = is_e2e(script_name, periodic=False)
    gcp = is_gcp(script_name)
    name = f"presubmit-agent-sandbox-{script_name}"

    if e2e:
        labels = {"preset-dind-enabled": "true"}
        if gcp:
            labels["preset-k8s-ssh"] = "true"  # node access
        else:
            labels["preset-kind-volume-mounts"] = "true"
        resources = resources_for_e2e()
    else:
        labels = {}
        resources = resources_for_local_test()

    job = {
        "name": name,
        "cluster": "k8s-infra-prow-build" if gcp else "eks-prow-build-cluster",
        "skip_if_only_changed": SKIP_DOCS,
        "decorate": True,
    }
    if labels:
        job["labels"] = labels

    c = container(f"dev/ci/presubmits/{script_name}", e2e, resources)
    if gcp:
        c["env"] = [{"name": "BOSKOS_HOST", "value": "boskos.test-pods.svc.cluster.local"}]
        # GCP credentials via workload identity on k8s-infra-prow-build
        # (replaces the key-based preset-service-account preset).
        job["spec"] = {"serviceAccountName": "prow-build", "containers": [c]}
    else:
        job["spec"] = {"containers": [c]}

    tab_name = f"presubmit-{script_name}"
    job["annotations"] = {
        "testgrid-dashboards": DASHBOARD,
        "testgrid-tab-name": tab_name,
    }

    if override_func:
        override_func(job)

    return ordered(job, PRESUBMIT_KEY_ORDER)


def build_periodic(script_name, override_func=None):
    gcp = is_gcp(script_name)
    name = f"periodic-{script_name}"

    labels = {"preset-dind-enabled": "true"}
    if gcp:
        labels["preset-k8s-ssh"] = "true"
    else:
        labels["preset-kind-volume-mounts"] = "true"

    job = {
        "name": name,
        "cluster": "k8s-infra-prow-build" if gcp else "eks-prow-build-cluster",
        "interval": DEFAULT_PERIODIC_INTERVAL,
        "decorate": True,
        "annotations": {"testgrid-dashboards": DASHBOARD},
        "extra_refs": [{"org": REPO_ORG, "repo": REPO_NAME, "base_ref": "main"}],
        "labels": labels,
    }

    c = container(f"dev/ci/periodics/{script_name}", e2e=True, resources=resources_for_e2e())
    if gcp:
        c["env"] = [{"name": "BOSKOS_HOST", "value": "boskos.test-pods.svc.cluster.local"}]
        # GCP credentials via workload identity on k8s-infra-prow-build
        # (replaces the key-based preset-service-account preset).
        job["spec"] = {"serviceAccountName": "prow-build", "containers": [c]}
    else:
        job["spec"] = {"containers": [c]}

    if override_func:
        override_func(job)

    return ordered(job, PERIODIC_KEY_ORDER)


# Emit job keys in the order the hand-written config used, no matter when an
# override set them (e.g. decoration_config belongs next to decorate).
PRESUBMIT_KEY_ORDER = [
    "name", "cluster", "skip_if_only_changed", "run_if_changed",
    "decorate", "decoration_config", "always_run", "optional",
    "labels", "spec", "annotations",
]
PERIODIC_KEY_ORDER = [
    "name", "cluster", "interval", "decorate", "decoration_config",
    "annotations", "extra_refs", "labels", "spec",
]


def ordered(job, key_order):
    out = {k: job[k] for k in key_order if k in job}
    out.update({k: v for k, v in job.items() if k not in key_order})
    return out


HEADER = """\
# AUTOGENERATED by generate_jobs.py; do not edit directly.
# Regenerate with:
#   ./generate_jobs.py --agent-sandbox-dir=<path to agent-sandbox checkout>
# Jobs are derived from the scripts in dev/ci/presubmits and
# dev/ci/periodics in the agent-sandbox repository.
"""


class JobDumper(yaml.SafeDumper):
    pass


def _represent_str(dumper, value):
    # Use double-quoted scalars wherever quoting is needed -- strings that
    # would otherwise parse as another type (e.g. "true") and regex-ish
    # strings containing backslashes -- matching the style of the previous
    # hand-written config so diffs stay small. Everything else stays plain.
    style = None
    if "\\" in value or dumper.resolve(yaml.ScalarNode, value, (True, False)) != "tag:yaml.org,2002:str":  # pylint: disable=line-too-long
        style = '"'
    return dumper.represent_scalar("tag:yaml.org,2002:str", value, style)


JobDumper.add_representer(str, _represent_str)


def write_yaml(path, doc):
    with open(path, "w") as f:
        f.write(HEADER)
        # width: never wrap scalars; the config's long regexes stay on one
        # line like the hand-written originals.
        yaml.dump(doc, f, Dumper=JobDumper, sort_keys=False, default_flow_style=False, width=100000)
    print(f"wrote {path}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--agent-sandbox-dir", required=True, type=pathlib.Path,
                        help="Path to an agent-sandbox checkout")
    parser.add_argument("--output-dir", type=pathlib.Path,
                        default=pathlib.Path(__file__).parent,
                        help="Directory to write the job YAML files")
    args = parser.parse_args()

    presubmits_dir = args.agent_sandbox_dir / "dev" / "ci" / "presubmits"
    presubmit_scripts = discover(presubmits_dir, LEGACY_PRESUBMIT_ORDER)

    periodics_dir = args.agent_sandbox_dir / "dev" / "ci" / "periodics"
    periodic_scripts = discover(periodics_dir, LEGACY_PERIODIC_ORDER)
    if not presubmit_scripts and not periodic_scripts:
        print(f"error: no dev/ci scripts found under {args.agent_sandbox_dir}", file=sys.stderr)
        return 1

    presubmits = {
        "presubmits": {
            f"{REPO_ORG}/{REPO_NAME}": [
                build_presubmit(s, PRESUBMIT_OVERRIDES.get(s))
                for s in presubmit_scripts
            ]
        }
    }
    periodics = {
        "periodics": [
            build_periodic(s, PERIODIC_OVERRIDES.get(s))
            for s in periodic_scripts
        ]
    }

    write_yaml(args.output_dir / "agent-sandbox-presubmits-main.yaml", presubmits)
    write_yaml(args.output_dir / "agent-sandbox-periodics-main.yaml", periodics)
    return 0


if __name__ == "__main__":
    sys.exit(main())
