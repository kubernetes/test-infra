# Phony

`phony` sends fake GitHub webhooks.

## Running a GitHub event manager
`phony` is most commonly used for testing [`hook`](../hook) and its [plugins](/prow/plugins), but can be used for testing any externally exposed service configured to receive GitHub events (external plugins).

To get an idea of `phony`'s behavior, start a local instance of `hook` with
this:
```
go run prow/cmd/hook/main.go
--deck-url=<production deck URL>
--config-path=config/prow/config.yaml
--plugin-config=config/prow/plugins.yaml
--hmac-secret-file=path/to/hmac
--github-token-path=path/to/github-token

# Note:
# --deck-url is required because --dry-run flag requires it
# --hmac-secret-file is required for running locally, use the same hmac token for phony below
```

## Usage
Once you have a running server that manages github webhook events, generate an
`hmac` token (same process as in [prow](../..)), and point a `phony` pull
request event at it with the following:
```
phony --help
Usage of ./phony:
  -address string
    	Where to send the fake hook. (default "http://localhost:8888/hook")
  -event string
    	Type of event to send, such as pull_request. (default "ping")
  -hmac string
    	HMAC token to sign payload with. (default "abcde12345")
  -payload string
    	File to send as payload. If unspecified, sends "{}".
```

If you are testing `hook` and successfully sent the webhook from `phony`, you should see a log from `hook` resembling the following:
```
{"author":"","component":"hook","event-GUID":"GUID","event-type":"pull_request","level":"info","msg":"Pull request .","org":"","pr":0,"repo":"","time":"2018-05-29T11:38:57-07:00","url":""}
```

A list of supported events can be found in the [GitHub API Docs](https://developer.github.com/v3/activity/events/types/). Some example event payloads can be found in the [`examples`](/prow/cmd/phony/examples) directory.
