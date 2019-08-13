# DEPRECATION NOTICE

*October 9, 2018* `scenarios/*.py` will be moved to become part of kubetest v2, so we are
not taking PRs except for urgent bug fixes.

Also please bump [bootstrap image](/images/bootstrap) and 
[kubekins image](/images/kubekins-e2e) to take in any future changes.

# Test scenarios

Place scripts to run test scenarios inside this location.

Test jobs are composed of two things:
1) A scenario to test
2) Configuration options for the scenario.

Three example scenarios are:

* Unit tests
* Node e2e tests
* e2e tests

Example configurations are:

* Parallel tests on gce
* Build all platforms

The assumption is that each scenario will be called a variety of times with
different configuration options. For example at the time of this writing there
are over 300 e2e jobs, each run with a slightly different set of options.

## Contract

The scenario assumes the calling process (bootstrap.py) has setup all
prerequisites, such as checking out the right repository, setting pwd,
activating service accounts, etc.

The scenario also assumes that the calling process will handle all post-job
works, such as recording log output, copying logs to gcs, etc.

The scenario should exit 0 if and only on success.

The calling process can configure the scenario by calling the scenario
with different arguments. For example: `kubernetes\_build.py --fast`
configures the scenario to do a fast build (one platform) and/or
`kubernetes\_build.py --federation=random-project` configures the scenario
to do a federation build using the random-project.

Call the scenario with `-h` to see configuration options.
