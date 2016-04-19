Gubernator is a frontend for displaying Kubernetes test results stored in GCS.

It runs on Google App Engine, and parses JSON and junit.xml results for display.

An example URL is:

	http://k8s-gubernator.appspot.com/build/kubernetes-jenkins/logs/kubernetes-soak-continuous-e2e-gce/5043

For development:

- Install the Google App Engine Python SDK
- Run locally using dev_appserver.py
- Visit the example URL at localhost:8080

The tests are run using nose-gae. See the comments in main_test.py for more
information.
