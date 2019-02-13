package plugins

import "k8s.io/test-infra/prow/github"

type HandlerResult int

const (
	// indicates next handler should run as normal
	ContinueResult HandlerResult = iota
	// indicates event processing should stop
	BreakResult
)

var (
	priorityPullRequestHandlers = map[string]PriorityPullRequestHandler{}
)

type PriorityPullRequestHandler func(Agent, github.PullRequestEvent) (HandlerResult, error)

func RegisterPriorityPullRequestHandler(name string, fn PriorityPullRequestHandler, help HelpProvider) {
	pluginHelp[name] = help
	priorityPullRequestHandlers[name] = fn
}

func (pa *ConfigAgent) PriorityPullRequestHandlers(owner, repo string) map[string]PriorityPullRequestHandler {
	pa.mut.Lock()
	defer pa.mut.Unlock()

	hs := map[string]PriorityPullRequestHandler{}
	for _, p := range pa.getPlugins(owner, repo) {
		if h, ok := priorityPullRequestHandlers[p]; ok {
			hs[p] = h
		}
	}

	return hs
}

func priorityEventsForPlugin(name string) []string {
	var events []string
	if _, ok := priorityPullRequestHandlers[name]; ok {
		events = append(events, "pull_request")
	}
	return events
}
