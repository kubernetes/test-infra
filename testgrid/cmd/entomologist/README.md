# Entomologist

Entomologist collects bugs from GitHub and pins them to TestGrid tests. When an issue is "pinned"
to a TestGrid target, a hyperlink to the issue will appear on that row.

## Pinning Behavior

Entomologist is expecting targets to be explicitly called out in issues. You can do this by writing
`pin:` at the start of a new line, and then the test group you want to pin to.


>Some update caused these tests to start failing!
>
>pin: ci-test-infra-bazel
>
>pin: post-test-infra-bazel
>
>/help


GitHub calls Pull Requests "Issues", but Entomologist doesn't.
Targets in Pull Requests won't be pinned.

Entomologist can be configured with a caching proxy, such as [ghProxy](https://github.com/kubernetes/test-infra/tree/master/ghproxy),
to minimize API token usage.
Entomologist is writing multiple [issue_state.proto](https://github.com/kubernetes/test-infra/blob/master/testgrid/issue_state/issue_state.proto)
files to Google Cloud Storage (GCS). TestGrid consumes the information placed there.

## Required Flags

- `--repos`: The `organization/repository` sets you're looking through for issues, comma-separated.\
  For example, `--repos=kubernetes/test-infra,kubernetes/kubernetes`
- `--output`: The location of the issue_state.proto file that Entomologist will write to
- `--config`: The location of the config.proto file that this TestGrid is using
- `--gcs-credentials-file`: GCS requires credentials to write to a GCS location