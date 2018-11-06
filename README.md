# Kubernetes Test Infrastructure

[![Go Report Card](https://goreportcard.com/badge/github.com/kubernetes/test-infra)](https://goreportcard.com/report/github.com/kubernetes/test-infra)  [![GoDoc](https://godoc.org/github.com/kubernetes/test-infra?status.svg)](https://godoc.org/github.com/kubernetes/test-infra)

The test-infra repository contains a collection of tools for testing Kubernetes
and displaying Kubernetes tests results. See also [CONTRIBUTING.md](CONTRIBUTING.md).

See the [architecture diagram](docs/architecture.svg) for an overview of how
the different services interact.

## Viewing test results

* The [Kubernetes TestGrid](https://testgrid.k8s.io/) shows historical test results
  - Configure your own testgrid dashboard at [testgrid/config.yaml](testgrid/config.yaml)
  - [Gubernator](https://gubernator.k8s.io/) formats the output of each run
* [PR Dashboard](https://gubernator.k8s.io/pr) finds PRs that need your attention
* [Prow](https://prow.k8s.io) schedules testing and updates issues
  - Prow responds to GitHub events, timers and [manual commands](https://go.k8s.io/bot-commands)
    given in GitHub comments.
  - The [prow dashboard](https://prow.k8s.io/) shows what it is currently testing
  - Configure prow to run new tests at [config/jobs](config/jobs)
* [Triage Dashboard](https://go.k8s.io/triage) aggregates failures
  - Triage clusters together similar failures
  - Search for test failures across jobs
  - Filter down failures in a specific regex of tests and/or jobs
* [Velodrome metrics](http://velodrome.k8s.io/dashboard/db/bigquery-metrics?orgId=1) track job and test health.
  - [Kettle](kettle) does collection, [metrics](metrics) does reporting, and [velodrome](velodrome) is the frontend.


## E2E Testing

Our e2e testing uses [kubetest](/kubetest) to build/deploy/test kubernetes
clusters on various providers. Please see those documents for additional details
about this tool as well as e2e testing generally.

Anyone can reconfigure our CI system with a test-infra PR that updates the
appropriate files. Detailed instructions follow:

## CI Job management

### Create a new job

Bootstrap is deprecated, Please utilize the [podutils](prow/pod-utilities.md#how-to-configure) to create new prowjobs.

Create a PR in this repo to add/update/remove a job or suite. Specifically
you'll need to do the following:
* Add the job to the appropriate section in [`config/jobs`](config/jobs)
  - Directory Structure:
    - In general for jobs for github.com/org/repo use config/jobs/org/repo/filename.yaml
    - For Kubernetes repos we also allow config/jobs/kubernetes/sig-foo/filename.yaml
    - We use basename of the config name as a key in the prow configmap, so the name of your config file need to be unique across the config subdir
  - Type of jobs:
    - Presubmit jobs run on unmerged code in PRs
    - Postsubmit jobs run after merging code
    - Periodic job run on a timed basis
    - You can find more prowjob definitions at [how-to-add-new-jobs](prow/jobs.md#how-to-configure-new-jobs)
  - A simple sample job uses podutil looks like:
    ```yaml
    - name: foo-repo-presubmit-test
      decorate: true
      spec:
        containers:
        - image: gcr.io/k8s-testimages/kubekins-e2e:latest-master
          command:
          - /path/to/cmd
          args:
          - positional
          - --and
          - flags
    ```

* Add the job name to the `test_groups` list in [`testgrid/config.yaml`](testgrid/config.yaml)
  - Also the group to at least one `dashboard_tab`

The configs need to be sorted and kubernetes must be in sync with the security repo, or else presubmit will fail.
You can run the script below to keep them valid:
```
hack/update-config.sh
```

#### Local testing

`docker run` your image locally, and mount in the repos you depend on.

<!-- TODO: We are working on have a utility to run the job locally - https://github.com/kubernetes/test-infra/issues/6590 -->


### Release branch jobs & Image validation jobs

Release branch jobs and image validation jobs are defined in [test_config.yaml](experiment/test_config.yaml).
We test different master/node image versions against multiple k8s branches on different features.

Those jobs are using channel based versions, current supported testing map is:
- k8s-dev : master
- k8s-beta : release-1.13
- k8s-stable1 : release-1.12
- k8s-stable2 : release-1.11
- k8s-stable3 : release-1.10

Our build job will generate a ci/(channel-name) file pointer in gcs.

After you update [test_config.yaml](experiment/test_config.yaml), please run

```
bazel run //experiment:generate_tests -- --yaml-config-path=experiment/test_config.yaml
```

to regenerate the job configs.

We are moving towards making more jobs to fit into the generated config.


Presubmit will tell you if you forget to do any of this correctly.

Merge your PR and [@k8s-ci-robot] will deploy your change automatically.

### Update an existing job

Largely similar to creating a new job, except you can just modify the existing
entries rather than adding new ones.

Update what a job does by editing its definition in [`config/jobs`](config/jobs).

Update where the job appears on testgrid by changing [`testgrid/config.yaml`].

### Delete a job

The reverse of creating a new job: delete the appropriate entries in
[`config/jobs`] and [`testgrid/config.yaml`].

Merge your PR and [@k8s-ci-robot] will deploy your change automatically.

## Building and testing the test-infra

We use [Bazel](https://www.bazel.io/) to build and test the code in this repo.
The commands `bazel build //...` and `bazel test //...` should be all you need
for most cases. If you modify Go code, run `./hack/update-bazel.sh` to keep
`BUILD.bazel` files up-to-date.

## Contributing Test Results

The Kubernetes project encourages organizations to contribute execution of e2e
test jobs for a variety of platforms (e.g., Azure, rktnetes). For information about
how to contribute test results, see [Contributing Test Results](docs/contributing-test-results.md).

## Other Docs

* [kubernetes/test-infra dependency management](docs/dep.md)


[`config/jobs`]: /config/jobs
[`testgrid/config.yaml`]: /testgrid/config.yaml
[test-infra oncall]: https://go.k8s.io/oncall
[@k8s-ci-robot]: (https://github.com/k8s-ci-robot)
