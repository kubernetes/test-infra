# Kettle Streaming Big Query Insert

## Google Cloud Requirements
1. PubSub Topic Subscription to the [gcs-changes] Topic.
2. [BigQuery] Tables to write rows to
3. Google [Cloud Functions]

## High Level Flow
- When Prow completes a job, a PubSub message is published to the `gcs-changes` topic
- The subscription to this topic is alerted and triggers the execution of a Cloud Function.
- The Cloud Function spins a container who's entry point consumes the PubSub message
- From the [GCS PubSub] data, the Cloud Function collects build information that can be used to collect the json artifacts used to create a `build` that will be serialized into a BigQuery row.

## Deployment

Run `./deploy.sh`

*This can only be run from the project in which the topic exists*

To delete run `gcloud functions delete <Name of method>`

[BigQuery]: https://console.cloud.google.com/bigquery?utm_source=bqui&utm_medium=link&utm_campaign=classic&project=k8s-gubernator
[Cloud Functions]: https://cloud.google.com/functions/?utm_source=google&utm_medium=cpc
[gcs-change]: https://console.cloud.google.com/cloudpubsub/topic/detail/gcs-changes?project=kubernetes-jenkins&authuser=1
[GCS PubSub]: https://cloud.google.com/pubsub/docs/reference/rest/v1/PubsubMessage
