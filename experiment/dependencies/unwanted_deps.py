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

"""Attribute new references to unwanted modules.

Diffs two `go mod graph` outputs, keeps edges newly pointing at a module in
hack/unwanted-dependencies.json, and finds which upstream release added each.
Uses the module proxy and GitHub API only, never clones.
"""

import argparse
import json
import logging
import os
import re
import subprocess
import urllib.error
import urllib.request

PROXY = os.environ.get("GOPROXY_BASE", "https://proxy.golang.org")
GITHUB_API = "https://api.github.com"
TIMEOUT = 30

# ponytail: linear scan below this many candidate versions is exact; above it we
# binary search, which assumes a require line is never removed and re-added.
LINEAR_SCAN_LIMIT = 25


_CACHE = {}


def get(url, accept="text/plain"):
    """GET a URL, returning the body as text, or None on any failure."""
    if url in _CACHE:
        return _CACHE[url]
    req = urllib.request.Request(url, headers={"Accept": accept})
    token = os.environ.get("GITHUB_TOKEN")
    if token and url.startswith(GITHUB_API):
        req.add_header("Authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(req, timeout=TIMEOUT) as resp:
            body = resp.read().decode("utf-8", "replace")
    except (urllib.error.URLError, urllib.error.HTTPError, OSError) as err:
        logging.debug("GET %s failed: %s", url, err)
        body = None
    _CACHE[url] = body
    return body


def get_json(url):
    body = get(url, accept="application/vnd.github+json")
    if not body:
        return None
    try:
        return json.loads(body)
    except json.JSONDecodeError:
        return None


def version_key(version):
    """Sort key for Go module versions. Good enough for release ordering."""
    version = version.lstrip("v").replace("+incompatible", "")
    base, _, pre = version.partition("-")
    nums = []
    for part in base.split(".")[:3]:
        nums.append(int(part) if part.isdigit() else 0)
    while len(nums) < 3:
        nums.append(0)
    # A release sorts after its own pre-releases and pseudo-versions.
    return (nums[0], nums[1], nums[2], 1 if not pre else 0, pre)


def parse_graph(path):
    """Parse `go mod graph` into edges and a module -> version map."""
    edges = set()
    versions = {}
    with open(path, encoding="utf-8") as handle:
        for line in handle:
            parts = line.split()
            if len(parts) != 2:
                continue
            src, dst = parts
            src_path, _, src_ver = src.partition("@")
            dst_path, _, dst_ver = dst.partition("@")
            edges.add((src_path, dst_path))
            if src_ver:
                versions.setdefault(src_path, set()).add(src_ver)
            if dst_ver:
                versions.setdefault(dst_path, set()).add(dst_ver)
    return edges, versions


def newest(versions, module):
    got = versions.get(module)
    if not got:
        return None
    return sorted(got, key=version_key)[-1]


def module_versions(module):
    body = get("%s/%s/@v/list" % (PROXY, module.lower()))
    if not body:
        return []
    return sorted((v for v in body.split() if v), key=version_key)


def requires(module, version, target):
    """Does module@version's go.mod require target?"""
    body = get("%s/%s/@v/%s.mod" % (PROXY, module.lower(), version))
    if not body:
        return None
    return re.search(r"^\s*%s\s+v" % re.escape(target), body, re.MULTILINE) is not None


def version_info(module, version):
    return get_json("%s/%s/@v/%s.info" % (PROXY, module.lower(), version)) or {}


def find_introducing_version(dependant, unwanted, old_version, new_version):
    """First release of dependant, after old_version, whose go.mod requires unwanted."""
    all_versions = module_versions(dependant)
    if not all_versions:
        return None, "module proxy has no version list"

    hi = version_key(new_version)
    lo = version_key(old_version) if old_version else None
    candidates = [
        v for v in all_versions
        if version_key(v) <= hi and (lo is None or version_key(v) > lo)
    ]
    if not candidates:
        return None, "no candidate versions between %s and %s" % (old_version, new_version)

    if len(candidates) <= LINEAR_SCAN_LIMIT:
        for version in candidates:
            if requires(dependant, version, unwanted):
                return version, "linear scan over %d releases" % len(candidates)
        return None, "no release in range requires it (indirect via another module?)"

    low, high, found = 0, len(candidates) - 1, None
    while low <= high:
        mid = (low + high) // 2
        if requires(dependant, candidates[mid], unwanted):
            found = candidates[mid]
            high = mid - 1
        else:
            low = mid + 1
    return found, "binary search over %d releases" % len(candidates)


def github_repo(origin_url):
    if not origin_url:
        return None
    match = re.match(r"https?://github\.com/([^/]+)/([^/.]+)", origin_url)
    return "%s/%s" % (match.group(1), match.group(2)) if match else None


def github_attribution(repo, base, head, unwanted, since=None):
    """go.mod line added, plus commits that touched go.mod. `since` bounds the
    list to the release range instead of whatever is most recent."""
    out = {}
    if not repo:
        return out

    compare = get_json("%s/repos/%s/compare/%s...%s" % (GITHUB_API, repo, base, head))
    if compare:
        out["commits_in_range"] = len(compare.get("commits") or [])
        for changed in compare.get("files") or []:
            if changed.get("filename") == "go.mod":
                for line in (changed.get("patch") or "").splitlines():
                    if line.startswith("+") and unwanted in line:
                        out["gomod_line_added"] = line.lstrip("+").strip()
                break

    url = "%s/repos/%s/commits?path=go.mod&sha=%s&per_page=10" % (GITHUB_API, repo, head)
    if since:
        url += "&since=%s" % since
    commits = get_json(url)
    if commits:
        out["gomod_commits"] = [
            {
                "sha": c.get("sha", "")[:12],
                "date": (c.get("commit", {}).get("author", {}).get("date") or "")[:10],
                "subject": (c.get("commit", {}).get("message") or "").split("\n")[0],
            }
            for c in commits[:10]
        ]
    return out


def attribute(dependant, unwanted, old_version, new_version):
    """Everything we can learn about one new unwanted reference."""
    record = {
        "unwanted": unwanted,
        "dependant": dependant,
        "dependant_was": old_version,
        "dependant_now": new_version,
    }

    introduced, how = find_introducing_version(dependant, unwanted, old_version, new_version)
    record["method"] = how
    if not introduced:
        return record

    record["introduced_in"] = introduced
    info = version_info(dependant, introduced)
    record["released"] = (info.get("Time") or "")[:10]
    origin = info.get("Origin") or {}
    record["repo_url"] = origin.get("URL")
    record["tag_commit"] = (origin.get("Hash") or "")[:12]

    repo = github_repo(origin.get("URL"))
    if repo and old_version:
        prior = None
        for version in module_versions(dependant):
            if version_key(version) < version_key(introduced):
                prior = version
        if prior:
            record["compared_from"] = prior
            since = version_info(dependant, prior).get("Time")
            record.update(github_attribution(repo, prior, introduced, unwanted, since))
    return record


def find_new_references(before_graph, after_graph, unwanted_modules):
    before_edges, before_versions = parse_graph(before_graph)
    after_edges, after_versions = parse_graph(after_graph)

    findings = []
    for src, dst in sorted(after_edges - before_edges):
        if dst not in unwanted_modules:
            continue
        findings.append((src, newest(before_versions, src), newest(after_versions, src),
                         dst, newest(after_versions, dst)))
    return findings


def group_records(records):
    """Bucket findings by the dependency update that caused them."""
    groups = {}
    for rec in records:
        key = (rec["dependant"], rec.get("dependant_was"),
               rec.get("dependant_now"), rec.get("introduced_in"))
        groups.setdefault(key, []).append(rec)
    return groups


def render_text(records):
    """Aligned table for the build log. Markdown pipes do not line up in plain text."""
    if not records:
        return "No banned modules are reintroduced by updating everything to latest."

    header = ("banned module", "version", "arrives via", "introduced upstream in")
    rows = [header]
    for rec in sorted(records, key=lambda r: (r["dependant"], r["unwanted"])):
        rows.append((
            rec["unwanted"],
            rec.get("unwanted_version") or "?",
            "%s %s -> %s" % (rec["dependant"], rec.get("dependant_was") or "(new)",
                             rec.get("dependant_now")),
            "%s (%s)" % (rec["introduced_in"], rec.get("released") or "?")
            if rec.get("introduced_in") else "unknown",
        ))

    widths = [max(len(row[i]) for row in rows) for i in range(len(header))]

    def rule(left, mid, right):
        return left + mid.join("─" * (w + 2) for w in widths) + right

    def line(row):
        return "│ " + " │ ".join(
            cell.ljust(width) for cell, width in zip(row, widths)) + " │"

    out = [rule("┌", "┬", "┐"), line(header), rule("├", "┼", "┤")]
    out += [line(row) for row in rows[1:]]
    out.append(rule("└", "┴", "┘"))

    out += ["", "why each update brings them in:", ""]
    for (dependant, was, now, introduced), group in sorted(group_records(records).items()):
        out.append("  %s %s -> %s  (%d banned module%s)" % (
            dependant, was or "(new)", now, len(group), "" if len(group) == 1 else "s"))
        first = group[0]
        if not introduced:
            out += ["    could not pin the introducing release: %s"
                    % first.get("method", "no detail"), ""]
            continue
        commits = first.get("gomod_commits") or []
        added = [r for r in group if r.get("gomod_line_added")]
        if added and commits:
            out.append("    likely cause: %s %s" % (commits[0]["sha"], commits[0]["subject"]))
            out.append("                  (adds the require lines directly; %d of %s commits "
                       "in the release touched go.mod)"
                       % (len(commits), first.get("commits_in_range", "?")))
        elif commits:
            out.append("    likely cause: unclear, arrived transitively. %d commits touched "
                       "go.mod, none add it directly." % len(commits))
            out.append("                  closest candidate: %s %s"
                       % (commits[0]["sha"], commits[0]["subject"]))
        else:
            out.append("    likely cause: no commit detail available for this repo")
        if first.get("repo_url"):
            out.append("    upstream:     %s" % first["repo_url"])
        out.append("")
    return "\n".join(out).rstrip()


def render_markdown(records, unwanted_reasons):
    """Group by what caused the reference, not by each module it dragged in."""
    lines = ["# Banned dependencies reintroduced by updating to latest", ""]
    if not records:
        lines += ["None. Updating every dependency to its latest version pulls in no module "
                  "banned by `hack/unwanted-dependencies.json`.", ""]
        return "\n".join(lines)

    groups = group_records(records)

    lines += [
        "**%d banned module(s) come back, pulled in by %d dependency update(s).**"
        % (len(records), len(groups)),
        "",
        "Each arrived because a dependency we want was updated and the new version "
        "requires the banned module. `hack/lint-dependencies.sh` fails until it is "
        "resolved, so that bump cannot merge.",
        "",
        "Options: ask upstream to drop it, hold the dependency back, or record it in "
        "`status.unwantedReferences` and bump anyway (fine when nothing is vendored).",
        "",
        "| banned module | version | arrives via | introduced upstream in |",
        "|---|---|---|---|",
    ]
    for rec in sorted(records, key=lambda r: (r["dependant"], r["unwanted"])):
        where = "`%s` (%s)" % (rec["introduced_in"], rec.get("released") or "?") \
            if rec.get("introduced_in") else "unknown"
        lines.append("| `%s` | %s | `%s` %s -> %s | %s |" % (
            rec["unwanted"], rec.get("unwanted_version") or "?", rec["dependant"],
            rec.get("dependant_was") or "(new)", rec.get("dependant_now"), where))
    lines += [
        "",
        "Attribution uses the module proxy and GitHub API, no clone, so the commits "
        "below are a lead, not a verdict.",
        "",
    ]

    for (dependant, was, now, introduced), group in sorted(groups.items()):
        first = group[0]
        lines += ["## `%s` %s -> %s" % (dependant, was or "(new)", now), ""]
        for rec in sorted(group, key=lambda r: r["unwanted"]):
            lines.append("- `%s@%s` - banned: %s" % (
                rec["unwanted"], rec.get("unwanted_version") or "?",
                unwanted_reasons.get(rec["unwanted"], "unspecified")))
        lines.append("")

        if not introduced:
            lines += ["Could not pin the release that introduced these: %s."
                      % first.get("method", "no detail"), ""]
            continue

        origin = "Introduced in `%s@%s`, released %s" % (
            dependant, introduced, first.get("released") or "an unknown date")
        if first.get("repo_url"):
            origin += ", %s @ `%s`" % (first["repo_url"], first.get("tag_commit") or "?")
        lines += [origin + ".", ""]

        added = sorted(r["gomod_line_added"] for r in group if r.get("gomod_line_added"))
        commits = first.get("gomod_commits") or []
        in_range = first.get("commits_in_range", "?")

        if added and commits:
            lines += [
                "**Likely cause:** `%s` %s" % (commits[0]["sha"], commits[0]["subject"]),
                "",
                "It added %s to that repo's own `go.mod`. %d of the %s commits in the "
                "release touched `go.mod`, and this is the first."
                % (", ".join("`%s`" % line for line in added), len(commits), in_range),
                "",
            ]
            if len(commits) > 1:
                lines += ["Others that touched `go.mod`: %s." % ", ".join(
                    "`%s` %s" % (c["sha"], c["subject"]) for c in commits[1:]), ""]
        elif commits:
            lines += [
                "**Likely cause:** unclear. %d commit(s) touched `go.mod` between `%s` and "
                "`%s` (%s commits in range), none naming a banned module directly, so it "
                "most likely arrived through one of their own dependency bumps:"
                % (len(commits), first.get("compared_from") or "?", introduced, in_range),
                "",
            ]
            lines += ["- `%s` %s %s" % (c["sha"], c["date"], c["subject"]) for c in commits]
            lines.append("")
        else:
            lines += ["**Likely cause:** no commit detail available for this repo.", ""]
    return "\n".join(lines)


def depstat_why(module, main_modules):
    """Local dependency path, for the report. Best effort."""
    try:
        return subprocess.run(
            ["depstat", "why", module, "-m", main_modules],
            capture_output=True, text=True, timeout=120, check=False).stdout.strip()
    except (OSError, subprocess.SubprocessError):
        return ""


def parse_arguments():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--unwanted-json", required=True,
                        help="path to hack/unwanted-dependencies.json")
    parser.add_argument("--before-graph", required=True,
                        help="`go mod graph` output from before the update")
    parser.add_argument("--after-graph", required=True,
                        help="`go mod graph` output from after the update")
    parser.add_argument("--markdown-output", help="write a markdown report here")
    parser.add_argument("--text-output", help="write the plain text table here")
    parser.add_argument("--json-output", help="write the findings as JSON here")
    parser.add_argument("--main-modules", default="",
                        help="comma separated main modules, for depstat why")
    return parser.parse_args()


def main():
    logging.basicConfig(level=logging.INFO, format="%(message)s")
    args = parse_arguments()

    with open(args.unwanted_json, encoding="utf-8") as handle:
        config = json.load(handle)
    unwanted_reasons = config.get("spec", {}).get("unwantedModules", {})

    findings = find_new_references(args.before_graph, args.after_graph, set(unwanted_reasons))

    records = []
    for dependant, was, now, unwanted, unwanted_version in findings:
        logging.info("attributing %s <- %s", unwanted, dependant)
        record = attribute(dependant, unwanted, was, now)
        record["unwanted_version"] = unwanted_version
        if args.main_modules:
            record["depstat_why"] = depstat_why(unwanted, args.main_modules)
        records.append(record)

    table = render_text(records)
    if args.markdown_output:
        with open(args.markdown_output, "w", encoding="utf-8") as handle:
            handle.write(render_markdown(records, unwanted_reasons))
    if args.text_output:
        with open(args.text_output, "w", encoding="utf-8") as handle:
            handle.write(table + "\n")
    if args.json_output:
        with open(args.json_output, "w", encoding="utf-8") as handle:
            json.dump(records, handle, indent=2, sort_keys=True)

    # build logs are plain text, so print the table, not the markdown artifact.
    if not args.text_output:
        print(table)

    return 1 if records else 0


if __name__ == "__main__":
    raise SystemExit(main())
