Gubernator is a frontend for displaying Kubernetes test results stored in GCS.

It runs on Google App Engine, and parses JSON and junit.xml results for display.

https://k8s-gubernator.appspot.com/

For development:

- Install the Google App Engine Python SDK
- Set the GAE_ROOT environment variable to your GAE SDK directory.
- Run locally using dev_appserver.py and visit http://localhost:8080
- Test using `./test.sh`. Arguments are passed to the test runner, so `./test.sh log_parser_test.py`
  only runs the tests for the log parser. See `./test.sh -h` for more options.
- Lint using `./lint.sh`.

For deployment:

- Get the "Gubernator Github Webhook Secret" (ask test-infra for help) and write
  it to `github/webhook_secret`.
- Run `./test.sh && appcfg.py update .`. If the `github/` service was modified,
  deploy that too.
