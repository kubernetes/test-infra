#!/bin/bash
while :; do
  python graph.py && gsutil cp -a public-read k8s-queue-health.png gs://kubernetes-test-history/
  sleep 1h
done
