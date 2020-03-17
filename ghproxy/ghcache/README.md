# ghCache

## What?

ghCache is an HTTP cache optimized for caching responses from the GitHub API (https://api.github.com). Specifically, it has the following non-standard caching behavior:
- Every cache hit is revalidated with a conditional HTTP request to GitHub regardless of cache entry freshness (TTL). The 'Cache-Control' header is ignored and overwritten to achieve this.
- Concurrent requests for the same resource are coalesced and share a single request/response from GitHub instead of each request resulting in a corresponding upstream request and response.

ghCache also provides prometheus instrumentation to expose cache activity,
request duration, and API token usage/savings.

## Why?

The most important behavior of ghCache is the mandatory cache entry revalidation.
While this property would cause most API caches to use tokens excessively, in the case of GitHub, we can actually save API tokens. This is because conditional requests for unchanged resources don't cost any API tokens!!! See: https://developer.github.com/v3/#conditional-requests
Free revalidation allows us to ensure that every request is satisfied with the most up to date resource without actually spending an API token unless the resource has been updated since we last checked it.

Request coalescing is beneficial for use cases in which the same resource is requested multiple times in rapid succession. Normally these requests would each result in an upstream request to GitHub, potentially costing API tokens, but with request coalescing at most one token is used. This particularly helps when many handlers react to the same event like in Prow's [hook component](/prow/cmd/hook).
