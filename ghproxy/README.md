# ghProxy

ghProxy is a reverse proxy HTTP cache optimized for use with the GitHub API (https://api.github.com).
It is essentially just a reverse proxy wrapper around [ghCache](/ghproxy/ghcache) with some additional prometheus instrumentation logic to monitor disk usage and push metrics to a prometheus push gateway.

ghProxy is designed to reduce API token usage by allowing many components to share a single ghCache. Note that components must use the same API token to benefit from the cache and avoid clobbering existing cache entries for other tokens.