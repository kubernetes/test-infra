## Usage

See the [Makefile](Makefile) for details.

### Start a cluster

Edit the variables at the top of the `Makefile`. Your project will need to have
container engine enabled and enough quota for your nodes. You will need to know
the HMAC secret for your webhook and the OAuth token for your robot account.
Leave `DRY_RUN` set to `false` unless you want comments and statuses and the
like to appear on GitHub.

Now run `make create-cluster`. This will take a few minutes. It will create
a GKE cluster and the `hook` image, deployment, and service. Once it finishes,
it will wait for the external IP to show up and print out the webhook address.
Add that to your GitHub webhook.
