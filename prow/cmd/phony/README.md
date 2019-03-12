# Phony

`phony` sends fake GitHub webhooks.

## Running a GitHub event manager
`phony` is most commonly used for testing [hook](../hook), but can be used for testing any
externally exposed service configured to receive GitHub events.

To get an idea of `phony`'s behavior, start a local instance of `hook` with
this:
```
go run prow/cmd/hook/main.go
--deck-url=<production deck URL>
--config-path=prow/config.yaml
--plugin-config=prow/plugins.yaml
--hmac-secret-file=path/to/hmac
-github-token-path=path/to/github-token
```

## Usage
Once you have a running server that manages github webhook events, generate an
`hmac` token (same process as in [prow](../..)), and point a `phony` pull
request event at it with the following:
```
bazel run //prow/cmd/phony --
--address=http://localhost:8888/hook
--hmac=<hmac token>
--event=pull_request
--payload="{}"
```

If you are testing `hook` and successfully sent the webhook from `phony`, you should see a log from `hook` resembling the following:
```
{"author":"","component":"hook","event-GUID":"GUID","event-type":"pull_request","level":"info","msg":"Pull request .","org":"","pr":0,"repo":"","time":"2018-05-29T11:38:57-07:00","url":""}
```

A list of supported events can be found in the [Github API Docs](https://developer.github.com/v3/activity/events/types/).
