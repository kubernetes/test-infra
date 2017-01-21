Webhooks-publisher is a Kubernetes deployment used to receive Github
webhooks. It changes the format and pushes it inside a Pub/Sub queue so
that they can be later processed by the mungebot.

Deploy
------

Deploying, updating configmap:
```
# Create container and push it to google-containers
make push

# Make sure you update the config version
kubectl create configmap webhook-config-v10 --from-file=config.yaml

# Edit deployment with new container name and/or config version

kubectl apply -f deployment.yaml
```

How to use
----------

`config.yaml` contains the configuration:

- `project` is the Google Cloud project-id that has the PubSub queue (see permissions below)
- Each item in `paths` is the path it listens to webhooks and maps to the Github
  `secret` and the PubSub `topic` where it should publish

If you want to listen to a repository:
- Add the path for the new webhook: `/my-repo`
- Give it the github secret you configured for that repo/webhook (https://developer.github.com/webhooks/creating/#setting-up-a-webhook)
- Create a new `topic` in `project` to receive the events, and put it in the config file
- Create as many subscription for the topic as you need, and consume messages
  from there (refer to Google Cloud PubSub documentation if needed)

Message format
--------------

The format of the messages pushed in the queue is simple. The signature has
already been validated so you don't need to do that again.

```
{
    "type": "For example: `push`, as received from X-Github-Event header.",
    "payload": "Complete body/event message as sent by Github. This is JSON in a string."
}
```

Permissions
-----------

In order to push/pull events into/from the PubSub, you need a "Service Account"
key. It needs *Project Editor* role.

It is set-up for the publisher as a secret key named `webhooks-publisher-secret`:

```
kubectl create secret generic webhooks-publisher-secret --from-file=credential.json=your-service-account.json
```
