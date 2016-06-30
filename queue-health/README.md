This app monitors the submit queue and produces the chart at
http://submit-queue.k8s.io/#/e2e.

It does this with two components:

- a poller, which polls the current state of the queue and appends it to a
  historical log.
- a grapher, which gets the historical log and renders it into charts.

This folder is organized in the following way:

- base: base image that graph and poll customize
- graph: a python app that downloads the polled history and renders it
- poll: a python app that polls the submit queue every 60s
- queue-health-graph-dev.yaml: test graph-only deployment
- queue-health-prod.yaml: production queue-health deployment
- queue-health.yaml: test queue-health deployment


Expected usage on a cluster whose nodes have a storage-rw or -full scope:
```
# vim graph/graph.py
make -C graph && make -C poll  # Make new images
kubectl create -f queue-health.yaml  # Deploy them to your cluster
kubectl get pods -l app=queue-health-dev
kubectl logs queue-health-dev-STUFF -c graph
```

Test changes locally with a bit of docker volume hackery
to send credentials to gsutil.
See the comment above the `CMD` line
in the `Dockerfile` of the folder of interest.

However it is probably easier to iterate outside docker:
```
python poller.py $HISTORY $GRAPH
python graph.py $HISTORY $GRAPH
```
