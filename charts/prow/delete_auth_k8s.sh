#!/bin/sh

set -e
GITHUB_USER_NAME=`kubectl get secret hookmanager-cred --output=jsonpath={.data.user_id} | base64 --decode | tr -d '\n\r'`
GITHUB_AUTH_ID=`kubectl get secret hookmanager-cred --output=jsonpath={.data.auth_id} | base64 --decode | tr -d '\n\r'`

#delete the token on github
docker run -it jfelten/hook_manager /hook_manager delete_authorization --account=${GITHUB_USER_NAME} --auth_id=${GITHUB_AUTH_ID}

#delete the token and secret used by this cluster
kubectl delete secret hookmanager-cred