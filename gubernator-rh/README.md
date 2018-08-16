Gubernator is a frontend for displaying Kubernetes test results stored in GCS.

It runs on Google App Engine, and parses JSON and junit.xml results for display.

https://k8s-gubernator.appspot.com/

For development:

- Install the Google Cloud SDK: https://cloud.google.com/sdk/
- Run locally using `dev_appserver.py` and visit http://localhost:8080
- Test and lint using `./test-gubernator.sh`
- Deploy with `make deploy` followed by `make migrate`

For deployment:

- Get the "Gubernator Github Webhook Secret" (ask test-infra for help) and write
  it to `github/webhook_secret`.
- Set up `secrets.json` to support Github [OAuth logins](https://github.com/settings/applications).
  The skeleton might look like:

```json
    {
        "k8s-gubernator.appspot.com": {
            "session": "(128+  bits of entropy for signing secure cookies)",
            "github_client": {
                "id": "(client_id for the oauth application)",
                "secret": "(client_secret for the oauth application)"
            }
        }
    }
```
