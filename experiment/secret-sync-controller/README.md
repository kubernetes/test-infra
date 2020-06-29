# secret_sync
Synchronizing between Google Cloud Secret Manager secrets and Kubernetes secrets.

## Current Functions
Parse configuration file into source and destination secrets.

Fetch the latest versions of from Secret Manager secret and Kubernetes secrets.

## Prerequisites
- Create a gke cluster.

- [Create service account for app](https://cloud.google.com/docs/authentication/production#command-line)

- Grant required permissions to the service account `service-account-name`.

	- Permission to get clusters:

		    gcloud projects add-iam-policy-binding <gcloud-project-id> --member "serviceAccount:<service-account-name>@<gcloud-project-id>.iam.gserviceaccount.com" --role "roles/container.clusterViewer"
	
	- Permission to manage secrets:

		    gcloud projects add-iam-policy-binding <gcloud-project-id> --member "serviceAccount:<service-account-name>@<gcloud-project-id>.iam.gserviceaccount.com" --role "roles/secretmanager.admin"

	- Permission to manage secrets within containers:

		- Create a custom iam role `iam-role-id` with container.secrets.* permissions and add the role to service account `service-account-name`:
			- service-secret-role.yaml

				    title: Kubernetes Engine Secret Admin
				    description: Provides access to management of Kubernetes Secrets
				    stage: GA
				    includedPermissions:
				    - container.secrets.create
				    - container.secrets.list
				    - container.secrets.get
				    - container.secrets.delete
				    - container.secrets.update
			
			- Create a custom iam role

				    gcloud iam roles create <iam-role-id> --project=<gcloud-project-id> --file=service-secret-role.yaml

			- Add the role to service account `service-account-name`:

				    gcloud projects add-iam-policy-binding <gcloud-project-id> --member "serviceAccount:<service-account-name>@<gcloud-project-id>.iam.gserviceaccount.com" --role "roles/<iam-role-id>"

		- Or just add [Kubernetes Engine Developer] role to service account `service-account-name`:

			    gcloud projects add-iam-policy-binding <gcloud-project-id> --member "serviceAccount:<service-account-name>@<gcloud-project-id>.iam.gserviceaccount.com" --role "roles/container.developer"

- [Configure cluster access for kubectl](https://cloud.google.com/kubernetes-engine/docs/how-to/cluster-access-for-kubectl) (This is already done in the entrypoint.sh file in this project)

- Generating service account key for authentication.
```
gcloud iam service-accounts keys create <key-name> --iam-account <service-account-name>@<gcloud-project-id>.iam.gserviceaccount.com
```

- Set the environment variable
``` 
export GOOGLE_APPLICATION_CREDENTIALS="<path/to/your/service/account/key.json>"
```

## Usage
```
go build
go test -v
./secret-sync-controller --config-path config.yaml --period 1000
```

## TODO
Script to setup workload identity binding.