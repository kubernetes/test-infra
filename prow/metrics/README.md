# Prometheus Metrics

Some Prow components expose Prometheus metrics that can be used for monitoring
and alerting. The following table describes the metrics that are currently
available.

| Component                 | Type      	| Metric                    	    | Labels                	    		| Description                                               	                |
|---------------------------|---------------|---------------------------------------|-------------------------------------------|-------------------------------------------------------------------------------|
| Tide                      | Gauge         | `pooledprs`               	    | org, repo, branch     	    		| The number of PRs in each Tide pool.                      	                |
|                           | Gauge         | `updatetime`              	    | org, repo, branch     	    		| The last time each Tide pool was synced.                  	                |
|                           | Gauge         | `syncdur`                 	    |                       	    		| The Tide sync controller loop duration.                   	                |
|                           | Gauge         | `statusupdatedur`         	    |                       	    		| The Tide status controller loop duration.                 	                |
|                           | Histogram     | `merges`                  	    | org, repo, branch     	    		| A histogram of the number of PRs in each merge.           	                |
|                           | Counter       | `tidepoolerrors`                      | org, repo, branch             		| Count of Tide pool sync errors.                                               |
|                           | Counter       | `tidequeryresults`                    | query_index, org_shard, result		| Count of Tide queries by query index, org shard, and result (success/error).  |
|                           | Counter       | `tidesyncheartbeat`                   | controller                    		| Count of Tide syncs per controller.                                           |
| Hook                      | Counter       | `prow_webhook_counter`    	    | event_type            	    		| The number of GitHub webhooks received by Prow.           	                |
| Plank/Jenkins-Operator    | Gauge         | `prowjobs`                	    | job_name, type, state 	    		| The number of ProwJobs.                                   	                |
| Jenkins-Operator          | Counter       | `jenkins_requests`        	    | verb, handler, code   	    		| The number of jenkins requests made by Prow.              	                |
|                           | Counter       | `jenkins_request_retries` 	    |                       	    		| The number of jenkins request retries Prow has made.      	                |
|                           | Histogram     | `jenkins_request_latency` 	    | verb, handler         	    		| A histogram of round trip times between Prow and Jenkins. 	                |
|                           | Histogram     | `resync_period_seconds`   	    |                       	    		| A histogram of the jenkins controller loop duration.      	                |
| Bugzilla                  | Histogram     | `bugzilla_request_duration`           | method, status                		| Bugzilla request duration by API path.                                        |
| Sinker                    | Gauge         | `sinker_pods_existing`                |                               		| Number of the existing pods in each sinker cleaning.                          |
|                           | Gauge         | `sinker_loop_duration_seconds`        |                               		| Time used in each sinker cleaning.                                            |
|                           | Gauge         | `sinker_pods_removed`                 | reason                        		| Number of pods removed in each sinker cleaning.                               |
|                           | Gauge         | `sinker_pod_removal_errors`           | reason                        		| Number of errors which occurred in each sinker pod cleaning.                  |
|                           | Gauge         | `sinker_prow_jobs_existing`           |                               		| Number of the existing prow jobs in each sinker cleaning.                     |
|                           | Gauge         | `sinker_prow_jobs_cleaned`            | reason                        		| Number of prow jobs cleaned in each sinker cleaning.                          |
|                           | Gauge         | `sinker_prow_jobs_cleaning_errors`    | reason                        		| Number of errors which occurred in each sinker prow job cleaning.             |
| Crier   | Histogram | `crier_report_latency`    | reporter                      	| Histogram of time spent reporting, calculated by the time difference between job completion and end of reporting.	|
|                           | Counter       | `crier_reporting_results`             | reporter, result              		| Count of successful and failed reporting attempts by reporter.                |
| Flagutil                  | Counter       | `kubernetes_failed_client_creations`  | cluster                       		| The number of clusters for which we failed to create a client.                |
| Gerrit/Adapter            | Counter       | `gerrit_processing_results`           | instance, repo, result        		| Count of change processing by instance, repo, and result.                     |
|                           | Histogram     | `gerrit_trigger_latency`              | instance                      		| Histogram of seconds between triggering event and ProwJob creation time.      |
| Gerrit/Client             | Counter       | `gerrit_query_results`                | instance, repo, result        		| Count of Gerrit API queries by instance, repo, and result.                    |
| GitHub                    | Gauge         | `github_user_info`                    | token_hash, login, email      		| Metadata about a user, tied to their token hash.                              |
| GitHub-Server             | Counter       | `prow_webhook_counter`                | event_type                    		| A counter of the webhooks made to prow.                                       |
|                           | Counter       | `prow_webhook_response_codes`         | response_code                 		| A counter of the different responses hook has responded to webhooks with.     |
|                           | Histogram     | `prow_plugin_handle_duration_seconds` | event_type, action, plugin, took_action	| How long Prow took to handle an event by plugin, event type and action.	|
| 			    | Counter	    | `prow_plugin_handle_errors`	    | event_type, action, plugin, took_action	| Prow errors handling an event by plugin, event type and action.		|
| Jenkins		    | Counter	    | `jenkins_requests`      		    | verb, handler, code			| Number of Jenkins requests made from prow.					|
|			    | Counter	    | `jenkins_request_retries`		    | 						| Number of Jenkins request retries made from prow.				|
|			    | Histogram	    | `jenkins_request_latency`       	    | verb, handler				| Time for a request to roundtrip between prow and Jenkins.			|
| 			    | Histogram	    | `resync_period_seconds`     	    | 						| Time the controller takes to complete one reconciliation loop.		|
| Jira			    | Histogram	    | `jira_request_duration_seconds`	    | method, path, status			| 										|
| Kube			    | Gauge	    | `prowjobs`			    | job_namespace, job_name, type, state, org, repo, base_ref, cluster, retest| Number of prowjobs in the system.		|
|			    | Counter	    | `prowjob_state_transitions`	    | job_namespace, job_name, type, state, org, repo, base_ref, cluster, retest| Number of prowjobs transitioning states. 	|
| Plugins		    | Gauge	    | `prow_configmap_size_bytes`	    | name, namespace				| Size of data fields in ConfigMaps updated automatically by Prow in bytes.	|
| Pubsub/Subscriber	    | Counter	    | `prow_pubsub_message_counter`	    | subscription				| A counter of the webhooks made to prow.					|
|			    | Counter	    | `prow_pubsub_error_counter`	    | subscription, error_type			| A counter of the webhooks made to prow.					|
|			    | Counter	    | `prow_pubsub_ack_counter`             | subscription				| A counter for message acked made to prow.					|
| 			    | Counter	    | `prow_pubsub_nack_counter`	    | subscription				| A counter for message nacked made to prow.					|
| 			    | Counter	    | `prow_pubsub_response_codes`	    | response_code, subscription		| A counter of the different responses server has responded to Push Events with.|
| Version		    | Gauge	    | `prow_version`			    | 						| Prow Version.									|


## Pushgateway and Proxy

To support metric collection from ephemeral tasks like request handling and to
provide a single scrape endpoint, Prow's prometheus metrics are pushed to a
Prometheus pushgateway that is scraped instead of the metric source. A proxy is
used to limit cluster external requests to GET requests since Prometheus doesn't
provide any form of authentication. The pushgateway and proxy deployment are
defined in [`pushgateway_deployment.yaml`](/config/prow/cluster/pushgateway_deployment.yaml).

## Kubernetes Prow Metrics

Prometheus metrics from the Kubernetes Prow instance are used to create the
graphs at http://monitoring.prow.k8s.io
