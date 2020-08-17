GPU Job Scheduling
======

A finite number of GPU projects is available (at time of writing, seventeen). If all the jobs
are left to run on an interval without further intervention, we will exhaust the available projects
and jobs will fail trying to get one.

This file sets out an explicit schedule to minimise the number of concurrent jobs we've used.
It's probably going to go stale, but is accurate at time of writing.

Currently we only need to use six projects, leaving eleven for PR jobs and other exceptional events. 

Artisanal
------

### 2 hours
d) `ci-kubernetes-e2e-gce-device-plugin-gpu` (22m) (01:30)  
e) `ci-kubernetes-e2e-gce-device-plugin-gpu-beta` (22m) (00:00)

### 3 hours
g) `ci-cri-containerd-e2e-gce-device-plugin-gpu` (22m) (01:30)

### 4 hours
h) `ci-kubernetes-e2e-gci-gke-autoscaling-gpu-k80` (1h33m) (00:00)  
i) `ci-kubernetes-e2e-gci-gke-autoscaling-gpu-p100` (1h33m) (02:00)

### 6 hours
k) `ci-kubernetes-e2e-gce-device-plugin-gpu-stable1` (23m) (03:00)  

### 12 hours
s) `ci-kubernetes-e2e-gce-device-plugin-gpu-stable2` (25m) (08:00)  
t) `ci-kubernetes-e2e-gce-gpu-stable2-stable1-cluster-upgrade` (40m) (04:00)  
u) `ci-kubernetes-e2e-gce-gpu-stable2-stable1-master-upgrade` (30m) (05:00)  
v) `ci-kubernetes-e2e-gce-gpu-stable1-beta-cluster-upgrade` (40m) (01:00)  
w) `ci-kubernetes-e2e-gce-gpu-stable1-beta-master-upgrade` (30m) (07:00)  
x) `ci-kubernetes-e2e-gce-gpu-stable1-master-cluster-upgrade` (40m) (03:00)  
y) `ci-kubernetes-e2e-gce-gpu-stable1-master-master-upgrade` (30m - 50m on failure) (09:00)  
z) `ci-kubernetes-e2e-gce-gpu-beta-stable1-cluster-downgrade` (40m) (10:00)  
@) `ci-kubernetes-e2e-gce-gpu-master-stable1-cluster-downgrade` (40m) (11:00)

Visualisation
-----

This sequence repeats in the afternoon.

```
|00:00|00:30|01:00|01:30|02:00|02:30|03:00|03:30|04:00|04:30|05:00|05:30|06:00|06:30|07:00|07:30|08:00|08:30|09:00|09:30|10:00|10:30|11:00|11:30|
|-----|-----|-----|--d--|--a--|--b--|--c--|--d--|--a--|--b--|--c--|--d--|-----|-----|-----|--d--|--a--|--b--|--c--|--d--|--a--|--b--|--c--|--d--|
|--e--|-----|-----|--g--|-----|-----|--k--|-----|--e--|--g--|-----|-----|--e--|-----|-----|--g--|--e--|-----|--k--|-----|--e--|--g--|-----|-----|
|-----------h-----------|-----------i-----------|-----------h-----------|-----------i-----------|-----------h-----------|-----------i-----------|
|-----|-----|-----v-----|-----|-----|-----x-----|-----t-----|-----u-----|-----|-----|-----w-----|--s--|-----|-----y-----|-----z-----|-----@-----|
```
