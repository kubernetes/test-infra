# Additional throttling algorithm
## Motivation
An additional throttling algorithm was introduced to `ghproxy` to prevent secondary rate
limiting issues (code `403`) in large Prow installations, consisting of several organizations.
Its purpose is to schedule incoming requests to adjust to the GitHub general rate-limiting
[guidelines](https://docs.github.com/en/rest/guides/best-practices-for-integrators#dealing-with-secondary-rate-limits).

## Implementation
An incoming request is analyzed whether it is targeting GitHub API v3 or API v4.
Separate queues are formed not only per API but also per organization if Prow installation is
using GitHub Apps. If a user account in a form of the bot is used, every request coming
from that user account is categorized as coming from the same organization. This is due to
the fact, that such a request identifies not using `AppID` and organization name, but `sha256` token hash.

There is a possibility to apply different throttling times per API version.

In the situation of a very high load, the algorithm prefers hitting
secondary rate limits instead of forming a massive queue of throttled messages, thus default
max waiting time in a queue is introduced. It is 30 seconds.

## Flags
Flags `--throttling-time-ms` and `--get-throttling-time-ms` have to be set to a non-zero value, otherwise, additional throttling mechanism will be disabled.

**All available flags:**
- `throttling-time-ms` enables a throttling mechanism which imposes time spacing between
outgoing requests. Counted per organization. Has to be set together with `--get-throttling-time-ms`.
- `throttling-time-v4-ms` is the same flag as above, but when set applies a separate time spacing for API v4.
- `get-throttling-time-ms` allows setting different time spacing for API v3 `GET` requests.
- `throttling-max-delay-duration-seconds` and `throttling-max-delay-duration-v4-seconds` allow
setting max throttling time for respectively API v3 and API v4. The default value is 30.
They are present to prefer hitting secondary rate limits, instead of forming massive queues of messages
during periods of high load.
- `request-timeout` refers to request timeout which applies also to paged requests. The default
is 30 seconds. You may consider increasing it if `throttling-max-delay-duration-seconds` and
`throttling-max-delay-duration-v4-seconds` are modified.

## Example configuration
Args from `ghproxy` configuration YAML file:
```
...
          args:
          - --cache-dir=/cache
          - --cache-sizeGB=10
          - --legacy-disable-disk-cache-partitions-by-auth-header=false
          - --get-throttling-time-ms=300
          - --throttling-time-ms=900
          - --throttling-time-v4-ms=850
          - --throttling-max-delay-duration-seconds=45
          - --throttling-max-delay-duration-v4-seconds=110
          - --request-timeout=120
          - --concurrency=1000 # rely only on additional throttling algorithm and "disable" the previous solution
...
```

## Metrics

Impact and the results after applying additional throttling can be consulted using two
`ghproxy` Prometheus metrics:

- `github_request_duration` to consult returned status codes across user agents and paths.
- `github_request_wait_duration_seconds` to consult the status and waiting times of the
requests handled by the throttling algorithm.

Both metrics are `histogram` type.
