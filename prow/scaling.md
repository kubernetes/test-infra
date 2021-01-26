# Using Prow at Scale

If you are maintaining a Prow instance that will need to scale to handle a large
load, consider using the following best practices, features, and additional tools.
You may also be interested in ["Getting more out of Prow"](/prow/more_prow.md).

## Features and Tools

### Separate Build Cluster(s)

It is frequently not secure to run all ProwJobs in the same cluster that runs
Prow's service components (`hook`, `plank`, etc.). In particular, ProwJobs that
execute presubmit tests for OSS projects should typically be isolated from
Prow's microservices. This isolation prevents a malicious PR author from
modifying the presubmit test to do something evil like breaking out of the
container and stealing secrets that live in the cluster or DOSing a
cluster-internal Prow component service.

Any number of build clusters can be used in order to isolate specific jobs from
each other, improve scalability, or allow tenants to provide and manage their
own execution environments. Instructions for configuring jobs to run in
different clusters can be found
[here](/prow/getting_started_deploy.md#Run-test-pods-in-different-clusters).

Production Prow instances should run most ProwJobs in a build cluster separate
from the Prow service cluster (the cluster where the Prow components live). Any
'trusted' jobs that require secrets or services that should not be exposed to
presubmit jobs, such as publishing or deployment jobs, should run in a different
cluster from the rest of the 'untrusted' jobs.
It is common for the Prow service cluster to be reused as a build cluster for
these 'trusted' jobs since they are typically fast and few in number so running
and managing an additional build cluster would be wasteful.

### Pull Request Merge Automation

Pull Requests can be automatically merged when they satisfy configured merge
requirements using [`tide`](/prow/cmd/tide/). Automating merge is critical for
large projects where allowing human to click the merge button is either a bottle
neck, a security concern, or both. Tide ensures that PRs have been tested
against the most recent base branch commit before merging (retesting if
necessary), and automatically groups multiple PRs to be tested and merged as a
batch whenever possible.

### Config File Split

If your Prow config starts to grow too large, consider splitting the job config
files into more specific and easily reviewed files. This is particularly useful
for delegating ownership of ProwJob config to different users or groups via the
use of OWNERS files with the [`approve` plugin](/prow/plugins/approve) and
[`Tide`](/prow/cmd/tide). It is common to enforce custom config policies for
jobs defined in certain files or directories via presubmit unit tests. This
makes it safe for Prow admins to delegate job config ownership by enforcing
limitations on what can be configured and by whom. For example, we use a golang
unit test in a presubmit job to validate that all jobs that are configured to
run in the `test-infra-trusted` build cluster are defined in a file controlled
by test-infra oncall.
([examples](https://github.com/kubernetes/test-infra/tree/5c388ffe5e45f44ac4b46a0d25e941d7fe22b126/config/tests/jobs))

To use this pattern simply aggregate all job configs in a directory of files
with unique base names and supply the directory path to components via
`--job-config-path`. The [`updateconfig` plugin](/prow/plugins/updateconfig) and
[`config-bootstrapper`](/prow/cmd/config-bootstrapper) support this pattern by
allowing multiple files to be loaded into a single configmap under different
keys (different files once mounted to a container).

### GitHub API Cache

[`ghproxy`](/ghproxy/) is a reverse proxy HTTP cache optimized for the GitHub API.
It takes advantage of how GitHub responds to E-tags in order to fulfill repeated
requests without spending additional API tokens. Check out this tool if you find
that your GitHub bot is consuming or approaching its token limit. Similarly,
re-deploying Prow components may trigger a large amount of API requests to GitHub
which may trip the abuse detection mechanisms. At scale, the `tide` deployment
itself may create enough API throughput to trigger this on its own. Deploying the
GitHub proxy cache is critical to ensuring that Prow does not trip this mechanism
when operating at scale.

### Config Driven GitHub Org Management

Managing org and repo scoped settings across multiple orgs and repos is not easy
with the mechanisms that GitHub provides. Only a few people have access to the
settings, they must be manually synced between repos, and they can easily become
inconsistent. These problems grow with number of orgs/repos and with the number
of contributors.
We have a few tools that automate this kind of administration and integrate well
with Prow:
- [`label_sync`](/label_sync/) is a tool that synchronizes labels and their
metadata across multiple orgs and repos in order to provide a consistent user
experience in a multi-repo project.
- [`branch_protector`](/prow/cmd/branchprotector) is a Prow component that
synchronizes GitHub branch requirements and restrictions based on config.
- [`peribolos`](/prow/cmd/periobolos) is a tool that synchronizes org settings,
teams, and memberships based on config.

### Metrics

Prow exposes some [Prometheus metrics](/prow/metrics/README.md) that can be used to generate graphs and
alerts. If you are maintaining a Prow instance that handles important workloads
you should consider using these metrics for monitoring.

## Best Practices

### Don’t share Prow’s GitHub bot token with other automation.

Some parts of Prow do not behave well if the GitHub bot token's rate limit is
exhausted. It is imperative to avoid this so it is a good practice to avoid
using the bot token that Prow uses for any other purposes.

### Working around GitHub's limited ACLs.

GitHub provides an extremely limited access control system that makes it
impossible to control granular permissions like authority to add and remove
specific labels from PRs and issues. Instead, write access to the entire
repo must be granted. This problem grows as projects scale and granular
permissions become more important.

Much of the GitHub automation that Prow provides is designed to fill in the gaps
in GitHub's permission system. The core idea is to limit repo write access to
the Prow bot (and a minimal number of repo admins) and then let Prow determine
if users have the appropriate permissions before taking action on their behalf.
The following is an overview of some of the automation Prow implements to work
around GitHub's limited permission system:
  - Permission to trigger presubmit tests is determined based on org membership
  as configured in the [`triggers`](https://github.com/kubernetes/test-infra/blob/526195d3e22cb90d784c1e4db1c43041a006c848/prow/plugins/plugins.go#L180) plugin config section.
  - File ownership is described with OWNERS files and change approval is
  enforced with the [`approve` plugin](/prow/plugins/approve). See the [docs](/prow/plugins/approve/approvers/README.md) for details.
  - Org member review of the most recent version of the PR is enforced with the
  [`lgtm` plugin](/prow/plugins/lgtm).
  - Various other plugins manage labels, milestone, and issue state based on 
  `/foo` style commands from authorized users. Authorization may be based on
  org membership, GitHub team membership, or OWNERS file membership.
  - [`Tide`](/prow/cmd/tide) provides PR merge automation so that humans do not need to (and are not
  allowed to) merge PRs. Without Tide, a user either has no permission to
  merge or they have repo write access which grants permission to merge any PR
  in the entire repo. Additionally, Tide enforces merge requirements like
  required and forbidden labels that humans may not respect if they are allowed
  to manually click the merge button.
