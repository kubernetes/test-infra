# `webhook-server`

The `webhook-server` is a component which manages two webhook endpoints (`/mutate` and `/validate`) which listen for requests to the Kubernetes API Server for the creation of ProwJobs and handle the admission (mutation and validation) of this ProwJob resource before persistence to the etcd.

the `webhook-server` can be configured by either passing in flags required amongst these flags are the `project-id` and `secret-id` for use with GCP Secret Manager.

# `Adding validation and mutation rules`
The validation rules that this component enforces against ProwJobs are located in `prow/cmd/webhook-server/validation.go` and the mutation rules as well in `prow/cmd/webhook-server/mutation.go`. 

If you would like to add more rules and/or specify more fields within a ProwJob to be defaulted, the logic within these files could be updated to fit the desired use cases. The latest configuration is generated each time, a request is sent to the Kubernetes API Server for the creation of a ProwJob so you need not worry about any future changes to the cluster configuration files.