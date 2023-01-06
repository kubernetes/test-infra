# LogExporter

LogExporter is a tool that runs post-test on our kubernetes test clusters.
It does the job of computing the set of logfiles to be exported (based on the
node type (master/node), cloud provider, and the node's system services),and
then actually exports them to the GCS path provided to it.

## How to run the tool?

To run the tool as a pod on a single node:

- Fill in the template variables with the right values in cluster/logexporter-pod.yaml
- `kubectl create -f cluster/logexporter-pod.yaml` (for master, run this as a static pod)

To run the tool as a daemonset on a k8s cluster:

- Fill in the template variables with the right values in cluster/logexporter-daemonset.yaml
- `kubectl create -f cluster/logexporter-pod.yaml`
- Delete the daemonset after detecting all work has been done as the pods just sleep after uploading logs

## Why not other logging tools?

Open source logging tools like Elasticsearch, Fluentd and Kibana are mostly centred
around taking in a stream of logs, adding metadata to them, creating individual log
entries that are then indexable/searchable/visualizable. We do not want all this
complicated machinery around the logfiles. Firstly because we don't want to affect
the performance of our tests depending on custom logging tools which can significantly
change from one to another. Secondly, a simple multi-threaded file block upload to GCS
(like this tool does) performs way faster than streaming log entries. Finally, having
logs as files on GCS fits into our current test-infra framework, while using these
tools would make log retrieval an API-oriented process, an unneeded complexity.
