## Jenkins proxy

This proxy has been built to work in conjuction with the prow jenkins-operator.
It enables multi-master Jenkins setups to work seamlessly with prow.

### Build on Openshift

Create the build pipeline that will build our proxy image.
```
oc process -f openshift/build.yaml | oc create -f -
```

### Deploy on Openshift

Tweak config.toml to your own preference. Then, create the secret that will hold
the tokens required for authenticating with the various Jenkins servers. We need
to run a master with basic auth and another master with bearer token auth so the
rest of the README assumes the aforementioned setup.
```
oc create secret generic jenkins-tokens --from-literal=basic=$(cat /etc/jenkins/basic) --from-literal=bearer=$(cat /etc/jenkins/bearer)
```

Create the ConfigMap that will hold the proxy configuration file.
```
oc create cm jenkins-proxy --from-file=config=config.json
```

Finally, deploy the proxy on the cluster.
```
oc process -f openshift/deploy.yaml | oc create -f -
```

Find the route URL for the proxy via `oc get route` and you should be able to
test that it is working properly.
