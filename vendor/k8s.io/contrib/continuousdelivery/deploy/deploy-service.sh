#!/bin/bash

# Copyright 2014 The Kubernetes Authors.
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

# $1 = the kubernetes context (specified in kubeconfig)
# $2 = directory that contains your kubernetes files to deploy
# $3 = pass in rolling to perform a rolling update

DIR=$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )
CONTEXT="$1"
DEPLOYDIR="$2"
ROLLING=$(echo "${3:0:7}" | tr '[:upper:]' '[:lower:]')

#make sure we have the kubectl comand
chmod +x $DIR/ensure-kubectl.sh
$DIR/ensure-kubectl.sh

#set config context
~/.kube/kubectl config use-context ${CONTEXT}
~/.kube/kubectl version

#get user, password, certs, namespace and api ip from config data
export kubepass=`(~/.kube/kubectl config view -o json --raw --minify  | jq .users[0].user.password | tr -d '\"')`

export kubeuser=`(~/.kube/kubectl config view -o json --raw --minify  | jq .users[0].user.username | tr -d '\"')`

export kubeurl=`(~/.kube/kubectl config view -o json --raw --minify  | jq .clusters[0].cluster.server | tr -d '\"')`

export kubenamespace=`(~/.kube/kubectl config view -o json --raw --minify  | jq .contexts[0].context.namespace | tr -d '\"')`

export kubeip=`(echo $kubeurl | sed 's~http[s]*://~~g')`

export https=`(echo $kubeurl | awk 'BEGIN { FS = ":" } ; { print $1 }')`

export certdata=`(~/.kube/kubectl config view -o json --raw --minify  | jq '.users[0].user["client-certificate-data"]' | tr -d '\"')`

export certcmd=""

if [ "$certdata" != "null" ] && [ "$certdata" != "" ];
then
    ~/.kube/kubectl config view -o json --raw --minify  | jq '.users[0].user["client-certificate-data"]' | tr -d '\"' | base64 --decode > ${CONTEXT}-cert.pem
    export certcmd="$certcmd --cert ${CONTEXT}-cert.pem"
fi

export keydata=`(~/.kube/kubectl config view -o json --raw --minify  | jq '.users[0].user["client-key-data"]' | tr -d '\"')`

if [ "$keydata" != "null" ] && [ "$keydata" != "" ];
then
    ~/.kube/kubectl config view -o json --raw --minify  | jq '.users[0].user["client-key-data"]' | tr -d '\"' | base64 --decode > ${CONTEXT}-key.pem
    export certcmd="$certcmd --key ${CONTEXT}-key.pem"
fi

export cadata=`(~/.kube/kubectl config view -o json --raw --minify  | jq '.clusters[0].cluster["certificate-authority-data"]' | tr -d '\"')`

if [ "$cadata" != "null" ] && [ "$cadata" != "" ];
then
    ~/.kube/kubectl config view -o json --raw --minify  | jq '.clusters[0].cluster["certificate-authority-data"]' | tr -d '\"' | base64 --decode > ${CONTEXT}-ca.pem
    export certcmd="$certcmd --cacert ${CONTEXT}-ca.pem"
fi

#set -x

#print some useful data for folks to check on their service later
echo "Deploying service to ${https}://${kubeuser}:${kubepass}@${kubeip}/api/v1/proxy/namespaces/${kubenamespace}/services/${SERVICENAME}"
echo "Monitor your service at ${https}://${kubeuser}:${kubepass}@${kubeip}/api/v1/proxy/namespaces/kube-system/services/kibana-logging/?#/discover?_a=(columns:!(log),filters:!(),index:'logstash-*',interval:auto,query:(query_string:(analyze_wildcard:!t,query:'tag:%22kubernetes.${SERVICENAME}*%22')),sort:!('@timestamp',asc))"

if [ "${ROLLING}" = "rolling" ]; then
  # perform a rolling update.
  # assumes your service\rc are already created
  ~/.kube/kubectl rolling-update ${SERVICENAME} --image=${DOCKER_REGISTRY}/${CONTAINER1}:latest || true
  
else

  # delete service (throws and error to ignore if service does not exist already)
  for f in ${DEPLOYDIR}/*.yaml; do envsubst < $f > kubetemp.yaml; cat kubetemp.yaml; echo ""; ~/.kube/kubectl delete --namespace=${kubenamespace} -f kubetemp.yaml || true; done

  # create service (does nothing if the service already exists)
  for f in ${DEPLOYDIR}/*.yaml; do envsubst < $f > kubetemp.yaml; ~/.kube/kubectl create --namespace=${kubenamespace} -f kubetemp.yaml --validate=false || true; done
fi

# wait for services to start
sleep 30

COUNTER=0
while [  $COUNTER -lt 30 ]; do
  let COUNTER=COUNTER+1
  echo Service Check: $COUNTER
  STATUSCODE=$(curl -k --silent --output /dev/stdnull --write-out "%{http_code}" $certcmd  ${https}://${kubeuser}:${kubepass}@${kubeip}/api/v1/proxy/namespaces/${kubenamespace}/services/${SERVICENAME}/)
  echo HTTP Status: $STATUSCODE
  if [ "$STATUSCODE" -eq "200" ]; then
    break
  else
    sleep 10
    false
  fi
done
