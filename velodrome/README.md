Overview
========

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

2. A SQL Database containing a copy of the issues, events, and PRs in Github
repositories. It is used for calculating statistics about developer
productivity. It has the following components:
  * [Fetcher](fetcher/): fetches Github data and stores in a SQL database
  * [SQL Proxy](mysql/): SQL Proxy deployment to Cloud SQL
  * [Transform](transform/): Transform SQL (Github db) into valuable metrics

3. Other monitoring tools, only one for the moment:
  * [token-counter](token-counter/): Monitors RateLimit usage of your github
    tokens

Github statistics
=================

Here is how the github statistics are communicating between each other:

```
=> pulls from
-> pushes to
* External components

Github* <= Fetcher -> Cloud SQL* <= Transform -> InfluxDb
```

Other metrics/monitoring components
===================================

One can set-up monitoring components in two different ways:

1. Push data directly into InfluxDb. Influx uses a SQL-like syntax and
receives that data (there is no scraping). If you have events that you would
like to push from time to time rather than report a current status, you should
push to InfluxDB. Examples: build time, test time, etc ...

2. Data can be polled on a regular interval by Prometheus. Prometheus will
scrape the data and measure the current state of something. This is much more
useful for monitoring as you can see what is the health of a service at a given
time.

As an example, the token counter measures the usage of our github-tokens, and
has a new value every hour. We can push the new value to InfluxDB.
