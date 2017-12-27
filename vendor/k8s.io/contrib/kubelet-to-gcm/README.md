# kubelet-to-gcm

This container watches kubelet on a given host, and pushes the kubelet's
summary metrics to the GCM v3 API (stack driver).

## Monitor program and arguments

The monitor polls the kubelet API, and pushes those metrics to stack driver. Preferably, the nanny lives on the node it's monitoring.

Translating between Kubelet's summary API is the bulk of the work and logic in the monitor. This code is decoupled from the rest of the container, and presumably re-usable by other components.

```
Usage of monitor:
      --cluster="unknown": The cluster where this kubelet holds membership.
      --gcm-endpoint="": The GCM endpoint to hit. Defaults to the default endpoint.
      --host="localhost": The kubelet's host name.
      --port=10255: The kubelet's port.
      --project="": The project where this kubelet's host lives.
      --resolution=10: The time, in seconds, to poll the Kubelet.
      --zone="": The zone where this kubelet lives.
```

Some of these fields are required for the gke_container schema in StackDriver (e.g., cluster and project). Others are needed for determining endpoints.

## Example deployment file

The following yaml is an example deployment where the monitor pushes Kubelet metrics to GCM.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: kubelet-mon
  namespace: kube-system
  labels:
    k8s-app: kubelet-mon
spec:
  dnsPolicy: Default
  containers:
  - name: kubelet-mon
    image: gcr.io/gke-test-us-central1-b-0/kubelet-to-gcm:1.0
    resources:
      limits:
        cpu: 50m
        memory: 50Mi
      requests:
        cpu: 50m
        memory: 50Mi
    command:
      - /monitor
      - --host=10.240.0.6
      - --cluster=quintin-test
      - --zone=us-central1-b
      - --project=gke-test-us-central1-b-0
      - --resolution=60
  terminationGracePeriodSeconds: 30
```
