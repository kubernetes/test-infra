#!/bin/bash

FORK_TO_TEST="${FORK_TO_TEST:-kubernetes/perf-tests}"
BRANCH_TO_TEST="${BRANCH_TO_TEST:-master}"
IMAGE_TO_TEST="${IMAGE_TO_TEST:-ghcr.io/ritwikranjan/nptest:latest}"

KUBECONFIG=${KUBECONFIG:-~/.kube/config}

echo "Forking the repository..."
git clone "https://github.com/$FORK_TO_TEST.git"
cd $(basename $FORK_TO_TEST)
git checkout "$BRANCH_TO_TEST"

echo "Navigating to network/benchmarks/netperf directory..."
cd network/benchmarks/netperf
echo "Running netperf test..."
go run launch.go -image=$IMAGE_TO_TEST -kubeConfig=$KUBECONFIG -testFrom=0 -testTo=1 -json

echo "Cleaning up..."
cd ../../../..
rm -rf $(basename $FORK_TO_TEST)
echo "Script execution completed."
