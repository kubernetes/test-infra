# pull-test-infra-gubernator image

Use this image when you want to use `google-cloud-sdk` and `google-app-engine (GAE)` in a job.

## contents

- base:
  - `ubuntu:bionic-20200526`
- directories:
  - `/workspace` default working dir for `run` commands
  - `/google_appengine` root installation directory for GAE binary
- languages:
  - `python` with `pip`
- tools:
  - `git`
  - `mocha` 
  - `google-app-engine` (v1.9.40) installed from [appengine-sdk zip binary](https://storage.googleapis.com/appengine-sdks/featured/google_appengine_1.9.40.zip)
  - `google-cloud-sdk` installed as [documented](https://cloud.google.com/sdk/docs/install#deb)
  - `unzip`
  - `wget`
