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
kubectl create secret generic grafana --from-literal=rootpassword="${grafana_passwoord}"
kubectl create secret generic influxdb --from-literal=rootpassword="${influxdb_passwoord}"
```

Create the persistent disks (make sure `gcloud` points to your instance):
```
gcloud compute disks create grafana-database --size 10GiB
gcloud compute disks create influxdb-database --size 100GiB --type pd-ssd
```

Deploying
---------
Deploying is simple:
```
kubectl apply -f grafana.yaml -f influxdb.yaml -f nginx.yaml
```

Adding data-source
------------------
Probably a first time only:
```
./datasource.sh ${nginx_ip} ${grafana_password} ${influxdb_password}
```
