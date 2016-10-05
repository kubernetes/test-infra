Velodrome is the dashboard, monitoring and metrics for Kubernetes Developer
Productivity. It is hosted at http://velodrome.k8s.io.

It is made of three different main parts:
- [Grafana stack](grafana-stack/) is mostly the front-end website where users
  see the metrics along with the back-end databases used to print those
  metrics. It has:
  - an InfluxDB (a time-series database) to save all sorts of metrics,
  - a Prometheus instance to save poll-based metrics (more monitoring
  based)
  - a Grafana instance to display graphs based on these metrics
  - and an nginx to proxy all of these services in a single URL.

- Github statistics are created from a copy of Github database in a SQL
  database. It has the following components:
  - [Fetcher](fetcher/): fetches Github database into a SQL database
  - [SQL Proxy](mysql/): SQL Proxy deployment to Cloud SQL
  - [Transform](transform/): Transform SQL (Github db) into valuable metrics

- Various other monitoring tools, only one for the moment:
  - [token-counter](token-counter/): Monitors RateLimit usage of your github
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

- Pushing data directly into InfluxDb. Influx uses a SQL like syntax and
receives that data (there is no scraping). If you have events that you would
like to push from time to time rather than report a current status, you should
push to InfluxDB. Examples: build time, test time, etc ...

- Data can be polled on a regular interval by Prometheus. Prometheus will scrape
the data and measure the current state of something. This is much more useful
for monitoring as you can see what is the health of a service at a given time.

As an example, the token counter measures the usage of our github-tokens, and
has a new value every hour. We can push the new value to InfluxDB.
