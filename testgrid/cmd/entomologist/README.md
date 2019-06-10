# Entomologist

Entomologist collects bugs from GitHub and pins them to TestGrid tests. When an issue is "pinned"
to a TestGrid target, a hyperlink to the issue will appear on that row.

## Pinning Behavior

Entomologist is expecting targets to be explicitly called out in issues. You can do this by writing
"target:" at the start of a new line, and then the target you want to pin to.


>Some update caused these tests to start failing!
>
>target: [sig-storage] ConfigMap binary data should be reflected in volume [NodeConformance] [Conformance] [coreos-beta]
>
>target: [sig-storage] ConfigMap should be consumable from pods in volume as non-root [LinuxOnly] [NodeConformance] [Conformance] [coreos-beta]
>
>/help


GitHub calls Pull Requests "Issues", but Entomologist doesn't.
Targets in Pull Requests won't be pinned.

Entomologist can be configured with a caching proxy, such as [ghProxy](https://github.com/kubernetes/test-infra/tree/master/ghproxy),
to minimize API token usage.
Entomologist is writing an [issue_state.proto](https://github.com/kubernetes/test-infra/blob/master/testgrid/issue_state/issue_state.proto)
file to Google Cloud Storage (GCS). TestGrid consumes the information placed there.

## Required Flags

- `--github-org/--github-repo`: The organization/repository you're looking through for issues.\
  For example, this org/repo is `--github-org=kubernetes --github-repo=test-infra`
- `--output`: The location of the issue_state.proto file that Entomologist will write to
- `--gcs-credentials-file`: GCS requires credentials to write to a GCS location