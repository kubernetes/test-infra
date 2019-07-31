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
a) `ci-kubernetes-e2e-gke-device-plugin-gpu` (20m) (00:00)  
b) `ci-kubernetes-e2e-gke-device-plugin-gpu-beta` (20m) (00:30)  
c) `ci-kubernetes-e2e-gke-device-plugin-gpu-ubuntu` (20m) (01:00)  
d) `ci-kubernetes-e2e-gce-device-plugin-gpu` (22m) (01:30)  
e) `ci-kubernetes-e2e-gce-device-plugin-gpu-beta` (22m) (00:00)

### 3 hours
f) `ci-kubernetes-e2e-gke-device-plugin-gpu-monitoring` (21m) (00:30)  
g) `ci-cri-containerd-e2e-gce-device-plugin-gpu` (22m) (01:30)

### 4 hours
h) `ci-kubernetes-e2e-gci-gke-autoscaling-gpu-k80` (1h33m) (00:00)  
i) `ci-kubernetes-e2e-gci-gke-autoscaling-gpu-p100` (1h33m) (02:00)

### 6 hours
j) `ci-kubernetes-e2e-gke-device-plugin-gpu-stable1` (21m) (01:00)  
k) `ci-kubernetes-e2e-gce-device-plugin-gpu-stable1` (23m) (03:00)  
l) `ci-kubernetes-e2e-gke-ubuntu1-k8sstable2-gpu` (generated) (18m) (05:30)

### 12 hours
n) `ci-kubernetes-e2e-gke-device-plugin-gpu-p100` (18m) (02:30)  
o) `ci-kubernetes-e2e-gke-device-plugin-gpu-p100-beta` (19m) (08:30)  
p) `ci-kubernetes-e2e-gke-device-plugin-gpu-p100-stable1` (20m) (00:00)  
q) `ci-kubernetes-e2e-gke-device-plugin-gpu-p100-stable2` (20m) (06:00)  
r) `ci-kubernetes-e2e-gke-device-plugin-gpu-stable2` (21m) (02:00)  
s) `ci-kubernetes-e2e-gce-device-plugin-gpu-stable2` (25m) (08:00)  
t) `ci-kubernetes-e2e-gce-gpu-stable2-stable1-cluster-upgrade` (40m) (04:00)  
u) `ci-kubernetes-e2e-gce-gpu-stable2-stable1-master-upgrade` (30m) (05:00)  
v) `ci-kubernetes-e2e-gce-gpu-stable1-beta-cluster-upgrade` (40m) (01:00)  
w) `ci-kubernetes-e2e-gce-gpu-stable1-beta-master-upgrade` (30m) (07:00)  
x) `ci-kubernetes-e2e-gce-gpu-stable1-master-cluster-upgrade` (40m) (03:00)  
y) `ci-kubernetes-e2e-gce-gpu-stable1-master-master-upgrade` (30m - 50m on failure) (09:00)  
z) `ci-kubernetes-e2e-gce-gpu-beta-stable1-cluster-downgrade` (40m) (10:00)  
@) `ci-kubernetes-e2e-gce-gpu-master-stable1-cluster-downgrade` (40m) (11:00)



Generated
---------

These are defined in `experiment/test_config.yaml`

### 2 hours
A) `ci-kubernetes-e2e-gke-cos1-k8sbeta-gpu` (25m, always fails) (00:00)  
B) `ci-kubernetes-e2e-gke-cos2-k8sbeta-gpu` (25m, always fails) (00:30)  
C) `ci-kubernetes-e2e-gke-ubuntu1-k8sbeta-gpu` (18m) (01:00)  
D) `ci-kubernetes-e2e-gke-ubuntu2-k8sbeta-gpu` (19m) (01:30)

### 4 hours
E) `ci-kubernetes-e2e-gke-cos1-k8sstable1-gpu` (28m, always fails) (00:00)  
F) `ci-kubernetes-e2e-gke-cos1-k8sstable2-gpu` (28m, always fails) (00:30)  
G) `ci-kubernetes-e2e-gke-cos2-k8sstable1-gpu` (28m, always fails) (03:30)  
H) `ci-kubernetes-e2e-gke-cos2-k8sstable2-gpu` (28m, always fails) (01:30)  
I) `ci-kubernetes-e2e-gke-ubuntu1-k8sstable1-gpu` (20m) (02:00)  
J) `ci-kubernetes-e2e-gke-ubuntu2-k8sstable1-gpu` (20m) (02:30)

### 6 hours
K) `ci-kubernetes-e2e-gke-cos1-k8sstable3-gpu` (30m, always fails) (03:00)  
L) `ci-kubernetes-e2e-gke-cos2-k8sstable3-gpu` (27m, always fails) (05:00)  
M) `ci-kubernetes-e2e-gke-ubuntu1-k8sstable2-gpu` (19m) (05:30)  
N) `ci-kubernetes-e2e-gke-ubuntu1-k8sstable3-gpu` (19m) (00:30)  
O) `ci-kubernetes-e2e-gke-ubuntu2-k8sstable2-gpu` (20m) (02:30)  
P) `ci-kubernetes-e2e-gke-ubuntu2-k8sstable3-gpu` (20m) (01:00)

Visualisation
-----

This sequence repeats in the afternoon.

```
|00:00|00:30|01:00|01:30|02:00|02:30|03:00|03:30|04:00|04:30|05:00|05:30|06:00|06:30|07:00|07:30|08:00|08:30|09:00|09:30|10:00|10:30|11:00|11:30|
|--a--|--b--|--c--|--d--|--a--|--b--|--c--|--d--|--a--|--b--|--c--|--d--|--a--|--b--|--c--|--d--|--a--|--b--|--c--|--d--|--a--|--b--|--c--|--d--|
|--e--|--f--|--j--|--g--|--e--|--n--|--k--|--f--|--e--|--g--|--l--|--m--|--e--|--f--|--j--|--g--|--e--|--o--|--k--|--f--|--e--|--g--|--l--|--m--|
|-----------h-----------|-----------i-----------|-----------h-----------|-----------i-----------|-----------h-----------|-----------i-----------|
|--p--|--N--|-----v-----|--r--|--O--|-----x-----|-----t-----|-----u-----|--q--|--N--|-----w-----|--s--|--O--|-----y-----|-----z-----|-----@-----|
|--A--|--B--|--C--|--D--|--A--|--B--|--C--|--D--|--A--|--B--|--C--|--D--|--A--|--B--|--C--|--D--|--A--|--B--|--C--|--D--|--A--|--B--|--C--|--D--|
|--E--|--F--|--P--|--H--|--I--|--J--|--K--|--G--|--E--|--F--|--L--|--H--|--I--|--J--|--P--|--G--|--E--|--F--|--K--|--H--|--I--|--J--|--L--|--G--|
```
