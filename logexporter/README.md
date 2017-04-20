# LogExporter

LogExporter is a tool that runs post-test on our kubernetes test clusters.
It does the job of computing the set of logfiles to be exported (based on the
node type (master/node), cloud provider, and the node's system services),and
then actually exports them to the GCS path provided to it.

## How to run the tool?

Before running the tool on any node, create a secret with the gcloud service account credentials:
1. Prepare service-account.json file (that should have write access to specified GCS path)
2. `kubectl create secret generic logexporter-service-account --from-file=service-account.json=/path/to/service-account.json`

To run the tool as a pod on a single node:
3. Fill in the template with environment variable values in cluster/pod.yaml
4. `kubectl create -f pod.yaml` (for master, run this as a static pod or set nodeName field in the podspec)

To run the tool as a run-to-completion job on an entire k8s cluster (we ensure exactly 1 logexporter pod runs per node using hard inter-pod anti-affinity):
3. Fill in the template with environment variable values in cluster/job.yaml
4. `kubectl create -f job.yaml`

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
