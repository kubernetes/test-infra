#!/bin/bash

# Copyright 2016 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e

kubectl apply -f - <<EOF
apiVersion: v1
kind: PersistentVolume
metadata:
  labels:
    app: machine-learning-app
  name: machine-learning-volume
spec:
  capacity:
    storage: 10Gi
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Retain
  gcePersistentDisk:
    pdName: machine-learning-volume
    fsType: ext4
EOF

kubectl apply -f - <<EOF
kind: PersistentVolumeClaim
apiVersion: v1
metadata:
  name: machine-learning-volume-claim
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
  storageClassName: standard
  volumeName: machine-learning-volume
EOF

! test -z "$(gcloud compute disks list --uri machine-learning-volume)" || \
	    gcloud compute disks create machine-learning-volume --size 10GB

IMAGE=${1:-gcr.io/k8s-testimages/issue-triager:latest}
docker build --pull -t "$IMAGE" -f Dockerfile . 
docker push "$IMAGE"

kubectl apply -f - <<EOF
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  name: issue-triager
spec:
  replicas: 1
  template:
    metadata:
      labels:
        app: issue-triager
    spec:
      containers:
      - name: issue-triager
        command: ["python", "simple_app.py"]
        image: $IMAGE
        ports:
        - name: ml-port
          containerPort: 5000
        volumeMounts:
        - mountPath: /models/
          name: database-volume
      volumes:
      - name: database-volume
        persistentVolumeClaim:
          claimName: machine-learning-volume-claim
EOF

kubectl apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  labels:
    app: issue-triager
  name: issue-triager-service
  namespace: default
spec:
  ports:
  - name: ml-port
    port: 5000
    targetPort: ml-port
  selector:
    app: issue-triager
EOF
