package ghmetrics

import (
	"fmt"
	"strings"
)

func getSimplifiedPath(path string) string {
	var requestPath string
	frag := getFirstFragment(path)
	requestPath = frag
	switch frag {
	case "/repos":
		// /:owner/:repo
		requestPath = fmt.Sprintf("%s/owner/repo", requestPath)
		after := strings.Split(path, "/")
		if len(after) > 3 {
			// do some more
		}
		// do some more
		return requestPath
	case "/user":
		// do some more
		return path
	case "/users":
		// do some more
		return path
	case "/orgs":
		// do some more
		return path
	case "/issues":
		// do some more
		return path
	case "/search":
		// do some more
		return path
	case "/gists":
		// do some more
		return path
	case "/notifications":
		// do some more
		return path
	case "/repositories":
	case "/emojis":
	case "/events":
	case "/feeds":
	case "/hub":
	case "/rate_limits":
	case "/teams":
	case "/licenses":
	}
	return path
}

// getFirstPathFragment returns the first fragment of a path, of a
// `*url.URL.Path`. e.g. `/repo/kubernetes/test-infra/` will return `repo`
func getFirstFragment(path string) string {
	if len(path) > 1 {
		if path[0] == '/' {
			return fmt.Sprintf("/%s", strings.Split(path[:1], "/")[0])
		}
		return fmt.Sprintf("/%s", strings.Split(path, "/")[0])
	}
	return path
}
