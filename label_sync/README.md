# label_sync

Update or migrate github labels on repos in a github org based on a YAML file

## Configuration

A typical labels.yaml file looks like:

```yaml
---
labels:
  - color: 00ff00
    name: lgtm
  - color: ff0000
    name: priority/P0
    previously:
    - color: 0000ff
      name: P0
  - name: dead-label
    color: cccccc
    deleteAfter: 2017-01-01T13:00:00Z
```

This will ensure that:

- there is a green `lgtm` label
- there is a red `priority/P0` label, and previous labels should be migrated to it:
  - if a `P0` label exists:
    - if `priority/P0` does not, modify the existing `P0` label
    - if `priority/P0` exists, `P0` labels will be deleted, `priority/P0` labels will be added
- if there is a `dead-label` label, it will be deleted after 2017-01-01T13:00:00Z

## Usage

```sh
# test
bazel test //label_sync/...

# add or migrate labels on all repos in the kubernetes org
bazel run //label_sync -- \
  --config $(pwd)/label_sync/labels.yaml \
  --token /path/to/github_oauth_token \
  --orgs kubernetes
  # actually you need to pass the --confirm flag too, it will
  # run in dry-run mode by default so you avoid doing something
  # too hastily, hence why this copy-pasta isn't including it

# add or migrate labels on all repos except helm in the kubernetes org
bazel run //label_sync -- \
  --config $(pwd)/label_sync/labels.yaml \
  --token /path/to/github_oauth_token \
  --orgs kubernetes \
  --skip kubernetes/helm
  # see above

# add or migrate labels on the community and steering repos in the kubernetes org
bazel run //label_sync -- \
  --config $(pwd)/label_sync/labels.yaml \
  --token /path/to/github_oauth_token \
  --only kubernetes/community,kubernetes/steering
  # see above

# generate docs and a css file contains labels styling based on labels.yaml
bazel run //label_sync -- \
  --action docs \
  --config $(pwd)/label_sync/labels.yaml \
  --docs-template $(pwd)/label_sync/labels.md.tmpl \
  --docs-output $(pwd)/label_sync/labels.md \
  --css-template $(pwd)/label_sync/labels.css.tmpl \
  --css-output $(pwd)/prow/cmd/deck/static/labels.css
```

## Our Deployment

We run this as a [`CronJob`](./cluster/label_sync_cron_job.yaml) on a kubernetes cluster managed by [test-infra oncall](https://go.k8s.io/oncall), and can also schedule it as a [`Job`](./cluster/label_sync_cron_job.yaml) for one-shot usage.

These pods read [`labels.yaml`](./labels.yaml) from a ConfigMap that is updated by the [prow updateconfig plugin](/prow/plugins/updateconfig).

To update the `labels.yaml` file, simply open a pull request against it.
