Overview
========

The goal of this directory is to set-up the following monitoring stack:
- InfluxDB as the Time-series Database
- Grafana as the front-end/display
- Prometheus as a different flavor of time-series database
- Nginx is used as a proxy (mostly to fix CORS issue)

Testing locally
===============

You can set-up your own local grafana-stack easily, it doesn't have the same
constraints as production: No need to set-up a specific password or to go
through the `nginx` proxy.

You can run:

```
docker run -d -p 8083:8083 -p 8086:8086 tutum/influxdb:0.13
docker run -i -p 3000:3000 grafana/grafana
docker run -p 9090:9090 -v prom/prometheus
```

You then need to set-up the datasources/users into Grafana and InfluxDB as
described [below](#adding-data-source), and then import dashboards from
[dashboards/](dashboards/) directly through the web interface.

Step-by-step
============

First time-only
---------------
Create the passwords:
```
kubectl create secret generic grafana --from-literal=rootpassword="${grafana_password}"
kubectl create secret generic influxdb --from-literal=rootpassword="${influxdb_password}"
```

Deploying
---------
Create/Update prometheus configuration configmap:
```
kubectl create configmap prometheus --from-file=prometheus-config.yml
```

Deploying is simple:
```
kubectl apply -f grafana.yaml -f influxdb.yaml -f nginx.yaml
```

Adding data-source
------------------
First, you need to create the grafana user in Influxdb if it's not already there:
```
kubectl exec -i -t influxdb-123456789-abcde influx -username=root -password="${influxdb_password}" -execute "create user grafana with password 'password'; grant read on github to grafana; grant read on github to monitoring"
```

Probably a first time only:
```
./datasource.sh ${nginx_ip} ${grafana_password}
```
