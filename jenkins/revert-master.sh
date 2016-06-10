#!/bin/bash
set -o errexit
read -p "Delete master [press enter]"
gcloud compute instances delete pull-jenkins-master
read -p "Delete master disks [press enter]"
gcloud compute disks delete pull-jenkins-master pull-jenkins-master-data pull-jenkins-master-docker

read -p "Recreate master disks from snapshot [press enter]"
gcloud compute disks create pull-jenkins-master --source-snapshot=cadgdhszak0z --size=500GB
gcloud compute disks create pull-jenkins-master-data --source-snapshot=v9ytt8gg7sn0 --size=1000GB
gcloud compute disks create pull-jenkins-master-docker --source-snapshot=lwhewojewcyh --size=200GB

read -p "Recreate master instance [press enter]"
gcloud compute instances create pull-jenkins-master \
  --machine-type=n1-highmem-32 \
  --scopes=bigquery,cloud-platform,compute-rw,logging-write,monitoring-write,service-control,service.management,taskqueue,userinfo-email \
  --tag=do-not-delete,jenkins,jenkins-master \
  --address=104.154.45.126 \
  --disk=name=pull-jenkins-master,boot=yes,device-name=pull-jenkins-master \
  --disk=name=pull-jenkins-master-data,device-name=pull-jenkins-master-data \
  --disk=name=pull-jenkins-master-docker,device-name=pull-jenkins-master-docker

echo "Finished"
