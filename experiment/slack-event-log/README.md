# slack-event-log

slack-event-log sends a log of interesting Slack events to a Slack channel (and stdout) to make it
clearer to moderators when interesting things are happening. It also doubles as an audit log.

In the name of simplicity, slack-event-log currently only provides the information that can be
directly extracted from the webhook.

Currently runs on appengine, but can be trivially moved to a Kubernetes cluster given DNS and SSL
is set up.
