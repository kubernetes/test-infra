The goal of this directory is to set-up the following monitoring stack:
- InfluxDB as the Time-series Database
- Grafana as the front-end/display
- Nginx is use as a proxy (mostly to fix CORS issue)

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
Deploying is simple:
```
kubectl apply -f grafana.yaml -f influxdb.yaml -f nginx.yaml
```

Adding data-source
------------------
First, you need to create the grafana user in Influxdb:
```
kubectl exec -i -t influxdb-123456789-abcde influx -username=root -password="${influxdb_password}" -execute "create user grafana with password 'password'; grant read on github to grafana"
```

Probably a first time only:
```
./datasource.sh ${nginx_ip} ${grafana_password}
```
