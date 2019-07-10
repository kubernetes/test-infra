package ghmetrics

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"
)

var (
	reposRegex          = regexp.MustCompile(`^/repos/(?P<owner>[^/]*)/(?P<repo>[^/]*)(?P<rest>/.*)$`)
	userRegex           = regexp.MustCompile(`^/user(?P<rest>/.*)?$`)
	usersRegex          = regexp.MustCompile(`^/users/(?P<username>[^/]*)(?P<rest>/.*)?$`)
	orgsRegex           = regexp.MustCompile(`^/orgs/(?P<orgname>[^/]*)(?P<rest>/.*)?$`)
	notificationsRegex  = regexp.MustCompile(`^/notifications(?P<rest>/.*)?$`)
	varAndConstantRegex = regexp.MustCompile(`^/(?P<var>[^/]*)(?P<path>/.*)?$`)
	constantAndVarRegex = regexp.MustCompile(`^/(?P<path>[^/]*)(?P<var>/.*)?$`)
)

func getSimplifiedPath(path string) string {
	fragment := getFirstFragment(path)
	switch fragment {
	case "repos":
		// /:owner/:repo
		return handleRepos(path)
	case "user":
		// /user
		return handleUser(path)
	case "users":
		// /users
		return handleUsers(path)
	case "orgs":
		// /orgs
		return handleOrgs(path)
	case "issues":
		// /issues
		return fmt.Sprintf("/%s%s", fragment, handleConstantAndVar(path))
	case "search":
		// do we care to handle search sub-paths differently?
		// e.g.: /search/repositories, /search/commits, /search/code, /search/issues, /search/users, /search/topics, /search/labels
		return path
	case "gists":
		// do we care to handle gist sub-paths differently?
		// e.g. /gists/public, /gists/starred
		return path
	case "notifications":
		// /notifications
		return handleNotifications(path)
	case "repositories", "emojis", "events", "feeds", "hub", "rate_limit", "teams", "licenses":
		return path
	default:
		logrus.WithField("path", path).Warning("Path not handled")
		return path
	}
}

// getFirstFragment returns the first fragment of a path, of a
// `*url.URL.Path`. e.g. `/repos/kubernetes/test-infra/` will return `repos`
func getFirstFragment(path string) string {
	result := strings.Split(path, "/")
	for _, str := range result {
		if str != "" {
			return str
		}
	}
	return ""
}

func handleRepos(path string) string {
	match := reposRegex.FindStringSubmatch(path)
	result := make(map[string]string)
	for i, name := range reposRegex.SubexpNames() {
		if i != 0 && name != "" && i <= len(match) { // skip first and empty
			if match[i] != "" {
				result[name] = match[i]
			}
		}
	}
	if result["owner"] == "" || result["repo"] == "" {
		logrus.WithField("path", path).Warning("Not handling /repos/.. path correctly")
		return "/repos"
	}
	rest := result["rest"]
	sanitizedPath := fmt.Sprintf("/repos/%s/%s", ":owner", ":repo")
	if rest == "" || rest == "/" {
		return sanitizedPath
	}
	switch fragment := getFirstFragment(rest); fragment {
	case "issues", "branches":
		return fmt.Sprintf("%s%s", sanitizedPath, handlePrefixedVarAndConstant(fmt.Sprintf("/%s", fragment), rest))
	case "keys", "labels", "milestones", "pulls", "releases", "statuses", "subscribers", "assignees", "archive", "collaborators", "comments", "compare", "contents", "commits":
		// archive is a special path that might need better handling
		return fmt.Sprintf("%s/%s%s", sanitizedPath, fragment, handleConstantAndVar(rest))
	case "git":
		return fmt.Sprintf("%s%s", sanitizedPath, handlePrefixedConstantAndVar(fmt.Sprintf("/%s", fragment), rest))

	case "merges", "stargazers", "notifications", "hooks":
		return fmt.Sprintf("%s%s", sanitizedPath, rest)
	default:
		logrus.WithField("sanitizedPath", sanitizedPath).WithField("rest", rest).Warning("Path not handled")
		return fmt.Sprintf("%s%s", sanitizedPath, rest)
	}
}

func handleUser(path string) string {
	match := userRegex.FindStringSubmatch(path)
	result := make(map[string]string)
	for i, name := range userRegex.SubexpNames() {
		if i != 0 && name != "" && i <= len(match) { // skip first and empty
			if match[i] != "" {
				result[name] = match[i]
			}
		}
	}
	rest := result["rest"]
	sanitizedPath := fmt.Sprintf("/user")
	if rest == "" || rest == "/" {
		return sanitizedPath
	}
	switch fragment := getFirstFragment(rest); fragment {
	case "following", "keys":
		// archive is a special path that might need better handling
		return fmt.Sprintf("%s/%s%s", sanitizedPath, fragment, handleConstantAndVar(rest))

	case "emails", "public_emails", "followers", "starred", "issues", "email":
		return fmt.Sprintf("%s%s", sanitizedPath, rest)
	default:
		logrus.WithField("sanitizedPath", sanitizedPath).WithField("rest", rest).Warning("Path not handled")
		return fmt.Sprintf("%s%s", sanitizedPath, rest)
	}
}

func handleUsers(path string) string {
	match := usersRegex.FindStringSubmatch(path)
	result := make(map[string]string)
	for i, name := range usersRegex.SubexpNames() {
		if i != 0 && name != "" && i <= len(match) { // skip first and empty
			if match[i] != "" {
				result[name] = match[i]
			}
		}
	}
	if result["username"] == "" {
		logrus.WithField("path", path).Warning("Not handling /users/.. path correctly")
		return "/users"
	}
	rest := result["rest"]
	sanitizedPath := fmt.Sprintf("/users/%s", ":username")
	if rest == "" || rest == "/" {
		return sanitizedPath
	}
	switch fragment := getFirstFragment(rest); fragment {
	case "followers":
		return fmt.Sprintf("%s/%s%s", sanitizedPath, fragment, handleConstantAndVar(rest))

	case "repos", "hovercard", "following":
		return fmt.Sprintf("%s%s", sanitizedPath, rest)
	default:
		logrus.WithField("sanitizedPath", sanitizedPath).WithField("rest", rest).Warning("Path not handled")
		return fmt.Sprintf("%s%s", sanitizedPath, rest)
	}
}

func handleOrgs(path string) string {
	match := orgsRegex.FindStringSubmatch(path)
	result := make(map[string]string)
	for i, name := range orgsRegex.SubexpNames() {
		if i != 0 && name != "" && i <= len(match) { // skip first and empty
			if match[i] != "" {
				result[name] = match[i]
			}
		}
	}
	if result["orgname"] == "" {
		logrus.WithField("path", path).Warning("Not handling /orgs/.. path correctly")
		return "/orgs"
	}
	rest := result["rest"]
	sanitizedPath := fmt.Sprintf("/orgs/%s", ":orgname")
	if rest == "" || rest == "/" {
		return sanitizedPath
	}
	switch fragment := getFirstFragment(rest); fragment {
	case "credential-authorizations":
		return fmt.Sprintf("%s/%s%s", sanitizedPath, fragment, handleConstantAndVar(rest))

	case "repos", "issues":
		return fmt.Sprintf("%s%s", sanitizedPath, rest)
	default:
		logrus.WithField("sanitizedPath", sanitizedPath).WithField("rest", rest).Warning("Path not handled")
		return fmt.Sprintf("%s%s", sanitizedPath, rest)
	}
}

func handleNotifications(path string) string {
	match := notificationsRegex.FindStringSubmatch(path)
	result := make(map[string]string)
	for i, name := range notificationsRegex.SubexpNames() {
		if i != 0 && name != "" && i <= len(match) { // skip first and empty
			if match[i] != "" {
				result[name] = match[i]
			}
		}
	}

	rest := result["rest"]
	sanitizedPath := "/notifications"
	if rest == "" || rest == "/" {
		logrus.WithField("path", path).Warning("Not handling /notifications/.. path correctly")
		return sanitizedPath
	}

	if strings.HasSuffix(rest, "/threads") {
		return fmt.Sprintf("%s/%s", sanitizedPath, "threads")
	}
	return fmt.Sprintf("%s%s", sanitizedPath, handlePrefixedVarAndConstant("/threads", rest))
}

func handlePrefixedVarAndConstant(prefix, path string) string {
	pathTrimmed := strings.Replace(path, prefix, "", 1)
	fragment := getFirstFragment(pathTrimmed)
	switch fragment {
	case "comments", "events":
		return fmt.Sprintf("%s/%s%s", prefix, fragment, handleConstantAndVar(pathTrimmed))
	}

	match := varAndConstantRegex.FindStringSubmatch(pathTrimmed)
	result := make(map[string]string)
	for i, name := range varAndConstantRegex.SubexpNames() {
		if i != 0 && name != "" && i <= len(match) { // skip first and empty
			if name == "var" && match[i] != "" {
				result[name] = ":var" // mask issue number
			} else {
				result[name] = match[i]
			}
		}
	}
	rest := result["path"]
	sanitizedPath := fmt.Sprintf("%s/%s", prefix, result["var"])
	if result["var"] == "" && rest == "" {
		return prefix
	} else if rest == "" || rest == "/" {
		return sanitizedPath
	}
	return fmt.Sprintf("%s/%s%s", prefix, result["var"], rest)
}

func handlePrefixedConstantAndVar(prefix, path string) string {
	pathTrimmed := strings.Replace(path, prefix, "", 1)
	fragment := getFirstFragment(pathTrimmed)
	if fragment != "" {
		return fmt.Sprintf("%s/%s%s", prefix, fragment, handleConstantAndVar(pathTrimmed))
	}

	match := varAndConstantRegex.FindStringSubmatch(pathTrimmed)
	result := make(map[string]string)
	for i, name := range varAndConstantRegex.SubexpNames() {
		if i != 0 && name != "" && i <= len(match) { // skip first and empty
			if name == "var" && match[i] != "" {
				result[name] = ":var" // mask issue number
			} else {
				result[name] = match[i]
			}
		}
	}
	rest := result["path"]
	sanitizedPath := fmt.Sprintf("%s/%s", prefix, result["var"])
	if result["var"] == "" && rest == "" {
		return prefix
	} else if rest == "" || rest == "/" {
		return sanitizedPath
	}
	return fmt.Sprintf("%s/%s%s", prefix, result["var"], rest)
}

func handleConstantAndVar(path string) string {
	path = strings.TrimPrefix(path, "/")
	if strings.Contains(path, "/") {
		result := strings.Split(path, "/")

		if len(result) > 1 {
			return "/:var"
		}
	}
	return ""
}

func handleVarAndConstant(path string) string {
	result := strings.Split(path, "/")
	if len(result) > 1 && path != "/" {
		return "/:var"
	}
	return ""
}
