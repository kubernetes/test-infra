# Prometheus Metrics

Some Prow components expose Prometheus metrics that can be used for monitoring
and alerting. The following table describes the metrics that are currently
available.

| Component              	| Type      	| Metric                    	| Labels                	| Description                                               	|
|------------------------	|-----------	|---------------------------	|-----------------------	|-----------------------------------------------------------	|
| Tide                   	| Gauge     	| `pooledprs`               	| org, repo, branch     	| The number of PRs in each Tide pool.                      	|
|                        	| Gauge     	| `updatetime`              	| org, repo, branch     	| The last time each Tide pool was synced.                  	|
|                        	| Gauge     	| `syncdur`                 	|                       	| The Tide sync controller loop duration.                   	|
|                        	| Gauge     	| `statusupdatedur`         	|                       	| The Tide status controller loop duration.                 	|
|                        	| Histogram 	| `merges`                  	| org, repo, branch     	| A histogram of the number of PRs in each merge.           	|
| Hook                   	| Counter   	| `prow_webhook_counter`    	| event_type            	| The number of GitHub webhooks received by Prow.           	|
| Plank/Jenkins-Operator 	| Gauge     	| `prowjobs`                	| job_name, type, state 	| The number of ProwJobs.                                   	|
| Jenkins-Operator       	| Counter   	| `jenkins_requests`        	| verb, handler, code   	| The number of jenkins requests made by Prow.              	|
|                        	| Counter   	| `jenkins_request_retries` 	|                       	| The number of jenkins request retries Prow has made.      	|
|                        	| Histogram 	| `jenkins_request_latency` 	| verb, handler         	| A histogram of round trip times between Prow and Jenkins. 	|
|                        	| Histogram 	| `resync_period_seconds`   	|                       	| A histogram of the jenkins controller loop duration.      	|


## Pushgateway and Proxy

To support metric collection from ephemeral tasks like request handling and to
provide a single scrape endpoint, Prow's prometheus metrics are pushed to a
Prometheus pushgateway that is scraped instead of the metric source. A proxy is
used to limit cluster external requests to GET requests since Prometheus doesn't
provide any form of authentication. The pushgateway and proxy deployment are
defined in [`pushgateway_deployment.yaml`](/config/prow/cluster/pushgateway_deployment.yaml).

## Kubernetes Prow Metrics

Prometheus metrics from the Kubernetes Prow instance are used to create the
graphs at http://velodrome.k8s.io/.
