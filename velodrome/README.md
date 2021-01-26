[Velodrome](http://velodrome.k8s.io/)
=========

 > # :warning: **Velodrome no longer exists**: Cleanup is in progress

Overview
--------

Velodrome is the dashboard, monitoring and metrics for Kubernetes Developer
Productivity. It is hosted at:

http://velodrome.k8s.io.

It is comprised of three components:

1. [Grafana stack](grafana-stack/) is the front-end website where users can
  visualize the metrics along with the back-end databases used to print those
  metrics. It has:
  * an InfluxDB (a time-series database) to save precalculated metrics,
  * a Prometheus instance to save poll-based metrics (more monitoring
  based)
  * a Grafana instance to display graphs based on these metrics
  * and an nginx to proxy all of these services in a single URL.

2. A SQL Database containing a copy of the issues, events, and PRs in GitHub
repositories. It is used for calculating statistics about developer
productivity. It has the following components:
  * [Fetcher](fetcher/): fetches GitHub data and stores in a SQL database
  * [SQL Proxy](mysql/): SQL Proxy deployment to Cloud SQL
  * [Transform](transform/): Transform SQL (GitHub db) into valuable metrics

3. Other monitoring tools, only one for the moment:
  * [token-counter](token-counter/): Monitors RateLimit usage of your github
    tokens

GitHub statistics
-----------------

Here is how the github statistics are communicating between each other:

```
=> pulls from
-> pushes to
* External components

GitHub* <= Fetcher -> Cloud SQL* <= Transform -> InfluxDb
```

Other metrics/monitoring components
-----------------------------------

One can set-up monitoring components in two different ways:

1. Push data directly into InfluxDb. Influx uses a SQL-like syntax and
receives that data (there is no scraping). If you have events that you would
like to push from time to time rather than reporting a current status, you should
push to InfluxDB. Examples: build time, test time, etc ...

2. Data can be polled on a regular interval by Prometheus. Prometheus will
scrape the data and measure the current state of something. This is much more
useful for monitoring as you can see what is the health of a service at a given
time.

As an example, the token counter measures the usage of our github-tokens, and
has a new value every hour. We can push the new value to InfluxDB.

Naming convention
-----------------

To disambiguate how each word is used, let's give a description of the naming
convention used by velodrome:
- Organization: This has the same meaning as the GitHub Organization. This is
  holding multiple repositories. e.g. In `github.com/istio/manager`, the
  organization is `istio`.
- Repository can be either the last part of the github repository URL (i.e. in
  `github.com/istio/manager`, it would be `manager`), or the fully qualified
  repository name: `istio/manager`.
- Project: A project describe a completely hermetic instance of the website for
  a given team. A project can span across multiple organizations and multiple
  repositories. e.g. The kubernetes project is made of repositories in the
  `kubernetes` organization, and `kubernetes-sigs`.

Adding a new project
====================

Adding a new project is as simple as adding it to [config.yaml](config.yaml).
Typically, add the name of your project, the list of repositories. Don't worry
about the `public-ip` field as the IP will be created later. You can also leave
prometheus configuration if you don't need it initially.

There are new project specific deployments necessary, and they are
described [below](#new-project-deployments).

Deployment
==========

Update/Create deployments
-------------------------

[config.py](config.py) will generate all the deployments file for you. It reads
the configuration in [config.yaml](config.yaml) to generate deployments for each
project and/or repositories with proper labels. You can then use `kubectl`
labels help you select what you want to do exactly, for example:

```
./config.py # Generates the configuration and prints it on stdout
./config.py | kubectl apply -f - # Creates/Updates everything
./config.py | kubectl delete -f - # Deletes everything
./config.py | kubectl apply -f - -l project=kubernetes # Only creates/updates kubernetes
./config.py | kubectl apply -f - -l app=fetcher # Only creates/updates fetcher
```

First time deployments
----------------------

- Make sure you create
  [the secrets for SQL Proxy](mysql/#set-up-google-cloud-sql-proxy)
- Make sure your github tokens are also in a secret:

```
kubectl create secret generic github-tokens --from-file=${TOKEN_FILE_1} --from-file=${TOKEN_FILE_2}
```

New project deployments
-----------------------

- Create [secret for InfluxDB](grafana-stack/#first-time-only)
- Deploy everything: `./config.py | kubectl apply -f - -l project=${NEW_PROJECT_NAME}`
- Once the kubernetes service has its public IP, connect to the grafana instance, and add the
  default dashboard, star the dashboard, set-it as the default dashboard in the
  org preference.
- Set the static IP in the GCP project, and update `config.yaml` with its
  value. Potentially create a domain-name pointing to it.
