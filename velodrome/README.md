[Velodrome](http://velodrome.k8s.io/)
=========

Overview
--------

Velodrome is the dashboard, monitoring and metrics for Kubernetes Developer
Productivity. It is hosted at:

http://velodrome.k8s.io.

[Grafana stack](grafana-stack/) is the front-end website where users can
visualize the metrics along with the back-end databases used to print those
metrics. It has:
* an InfluxDB (a time-series database) to save precalculated metrics,
* a Prometheus instance to save poll-based metrics (more monitoring
based)
* a Grafana instance to display graphs based on these metrics
* and an nginx to proxy all of these services in a single URL.


Metrics/monitoring components
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
