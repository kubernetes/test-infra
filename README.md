# Kubernetes Test Infrastructure

[![Build Status](https://travis-ci.org/kubernetes/test-infra.svg?branch=master)](https://travis-ci.org/kubernetes/test-infra)

The test-infra repository contains a collection of tools for testing Kubernetes
and displaying Kubernetes tests results. There is no merge bot here. Feel free
to click the big green merge button once your code is reviewed and passes on
Travis. You will need to be a member of
[kubernetes/test-infra-maintainers](https://github.com/orgs/kubernetes/teams/test-infra-maintainers)
to merge.

## Testing Kubernetes

The YAML files under `jenkins/job-configs` define our Jenkins jobs via [Jenkins
Job Builder](http://docs.openstack.org/infra/jenkins-job-builder/). Travis will
run `jenkins/diff-job-config-patch.sh` to print out the XML diff between your
change and master.

## CI on GKE and PR Jenkins

Due to the abundance of bugs and security vulnerabilities in Jenkins and its
plugins, we are switching our entire CI system to Kubernetes. Currently, we
trigger PR Jenkins jobs using the code under `ciongke/`, and we plan on moving
more functionality out of Jenkins plugins and into there.

## Viewing Test Results

* The [Kubernetes TestGrid](https://k8s-testgrid.appspot.com/) shows the results
of test jobs for the last few weeks. It is currently not open-sourced, but we
we would like to move in that direction eventually.
* The [24-hour test history
dashboard](http://storage.googleapis.com/kubernetes-test-history/static/index.html)
collects test results from the last 24 hours. It is updated hourly by the
scripts under `jenkins/test-history`.
* [Gubernator](https://k8s-gubernator.appspot.com/) is a Google App Engine site
that parses and presents the results from individual test jobs.

## Federated Testing

The Kubernetes project encourages organizations to contribute execution of e2e
test jobs for a variety of platforms (e.g., Azure, rktnetes).  The test-history
scripts gather e2e results from these federated jobs.  For information about
how to contribute test results, see [Federated Testing](docs/federated_testing.md).
