Gubernator is a frontend for displaying Kubernetes test results stored in GCS.

It runs on Google App Engine, and parses JSON and junit.xml results for display.

https://k8s-gubernator.appspot.com/

For development:

- Install the Google App Engine Python SDK
- Run locally using dev_appserver.py and visit http://localhost:8080

The tests are run using nose-gae. See the comments in main_test.py for more
information on how to run them.
