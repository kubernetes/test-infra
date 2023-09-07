package topic

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/repoowners"

	githubql "github.com/shurcooL/githubv4"
)

const (
	// pluginName defines this plugin's registered name.
	pluginName = "topic"

	topicCommand       = "TOPIC"
	cleanTopicCommand  = "CLEAN-TOPIC"
	integrTopicCommand = "INTEGR-TOPIC"

	defaultIntegrationRepo = "deepin-community/Repository-Integration"
)

var (
	commandRegex       = regexp.MustCompile(`(?m)^/([^\s]+)[\t ]*([^\n\r]*)`)
	projectNameToIDMap = make(map[string]int)
)

type githubClient interface {
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	GetPullRequests(org, repo string) ([]github.PullRequest, error)
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
	ListPullRequestComments(org, repo string, number int) ([]github.ReviewComment, error)
	CreatePullRequest(org, repo, title, body, head, base string, canModify bool) (int, error)
	UpdatePullRequest(org, repo string, number int, title, body *string, open *bool, branch *string, canModify *bool) error
	DeleteComment(org, repo string, ID int) error
	CreateComment(org, repo string, number int, comment string) error
	AddLabel(org, repo string, number int, label string) error
	RemoveLabel(org, repo string, number int, label string) error
	WasLabelAddedByHuman(org, repo string, num int, label string) (bool, error)
	GetRepos(org string, isUser bool) ([]github.Repo, error)
	GetRepoProjects(owner, repo string) ([]github.Project, error)
	GetOrgProjects(org string) ([]github.Project, error)
	GetProjectColumns(org string, projectID int) ([]github.ProjectColumn, error)
	CreateProjectCard(org string, columnID int, projectCard github.ProjectCard) (*github.ProjectCard, error)
	GetColumnProjectCards(org string, columnID int) ([]github.ProjectCard, error)
	MoveProjectCard(org string, projectCardID int, newColumnID int) error
	DeleteProjectCard(org string, projectCardID int) error
	TeamHasMember(org string, teamID int, memberLogin string) (bool, error)
	BotUser() (*github.UserData, error)
	MutateWithGitHubAppsSupport(context.Context, interface{}, githubql.Input, map[string]interface{}, string) error
	QueryWithGitHubAppsSupport(ctx context.Context, q interface{}, vars map[string]interface{}, org string) error
}

type ownersClient interface {
	LoadRepoOwners(org, repo, base string) (repoowners.RepoOwner, error)
}

func init() {
	plugins.RegisterGenericCommentHandler(pluginName, handleGenericCommentEvent, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	topicConfig := map[string]string{}
	for _, repo := range enabledRepos {
		integrateRepo := configForRepo(optionsForRepo(config, repo.Org, repo.Repo))
		topicConfig[repo.String()] = fmt.Sprintf("The topic plugin configured to manage integrate PRs to %s", integrateRepo)
	}

	yamlSnippet, err := plugins.CommentMap.GenYaml(&plugins.Configuration{
		Topic: []plugins.Topic{
			{
				Repos: []string{
					"org/repo1",
					"org/repo2",
				},
				IntegrateRepo: defaultIntegrationRepo,
			},
		},
	})
	if err != nil {
		logrus.WithError(err).Warnf("cannot generate comments for %s plugin", pluginName)
	}

	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The topic plugin manage incoming PRs with a topic name.",
		Config:      topicConfig,
		Snippet:     yamlSnippet,
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/[remove-|integr]topic",
		Description: "Manager associated PRs with topic",
		Featured:    true,
		WhoCanUse:   "Users listed as 'approvers' in appropriate OWNERS files.",
		Examples:    []string{"/topic", "/integr-topic", "/remove-topic"},
	})

	return pluginHelp, nil
}

func handleGenericCommentEvent(pc plugins.Agent, ce github.GenericCommentEvent) error {
	return handleGenericComment(
		pc.Logger,
		pc.GitHubClient,
		pc.OwnersClient,
		pc.Config.GitHubOptions,
		pc.PluginConfig,
		&ce,
		pc.GitClient,
	)
}

func handleGenericComment(log *logrus.Entry, ghc githubClient, oc ownersClient,
	githubConfig config.GitHubOptions, config *plugins.Configuration, ce *github.GenericCommentEvent, gc git.ClientFactory) error {
	funcStart := time.Now()
	defer func() {
		log.WithField("duration", time.Since(funcStart).String()).Debug("Completed handleGenericComment")
	}()
	if ce.Action != github.GenericCommentActionCreated || !ce.IsPR || ce.IssueState == "closed" {
		log.Debug("Event is not a creation of a comment on an open PR, skipping.")
		return nil
	}

	cmd, topic := parseTopicCommand(&comment{Body: ce.Body, Author: ce.User.Login})
	if cmd == "" {
		log.Debug("Comment does not constitute topic, skipping event.")
		return nil
	}

	switch cmd {
	case topicCommand:
		log.Infof("Add to topic %s", topic)
		// AddOrCreateTopic()
	case cleanTopicCommand:
		log.Infof("Clean association topic %s", topic)
		// CleanTopic()
	case integrTopicCommand:
		log.Infof("Integrate to topic %s", topic)
		if err := integrateTopic(log, ghc, config, topic, ce, gc); err != nil {
			log.Errorf("Integrate with topic[%s] failed: %v", topic, err)
			return err
		}
	default:
		log.Warnf("Unknown command %s", cmd)
	}

	return nil
}

type comment struct {
	Body        string
	Author      string
	CreatedAt   time.Time
	HTMLURL     string
	ID          int
	ReviewState github.ReviewState
}

// See: https://developer.github.com/v4/object/pullrequest/.
type pullRequest struct {
	BaseRepository struct {
		Name githubql.String
	}
	Number githubql.Int
}

// See: https://docs.github.com/en/graphql/reference/objects#projectv2.
type projectV2 struct {
	Closed githubql.Boolean
	Items  struct {
		PageInfo struct {
			HasNextPage githubql.Boolean
			EndCursor   githubql.String
		}
		Nodes []struct {
			Content struct {
				PullRequest pullRequest `graphql:"... on PullRequest"`
			}
			Type githubql.String
		}
	} `graphql:"items(first:100, after: $searchCursor)"`
}

// See: https://developer.github.com/v4/query/.
type searchProjectV2ItemsQuery struct {
	Organization struct {
		ProjectV2 projectV2 `graphql:"projectV2(number: $number)"`
	} `graphql:"organization(login: $org)"`
}

type projectsV2 struct {
	PageInfo struct {
		HasNextPage githubql.Boolean
		EndCursor   githubql.String
	}
	Nodes []struct {
		Title  githubql.String
		Number githubql.Int
		ID     githubql.String
	}
}

// See: https://developer.github.com/v4/query/.
type searchProjectsV2Query struct {
	Organization struct {
		ProjectsV2 projectsV2 `graphql:"projectsV2(first: 100, after: $searchCursor)"`
	} `graphql:"organization(login: $org)"`
}

func parseTopicCommand(c *comment) (string, string) {

	for _, match := range commandRegex.FindAllStringSubmatch(c.Body, -1) {
		cmd := strings.ToUpper(match[1])
		if len(match) > 2 && (cmd == topicCommand ||
			cmd == cleanTopicCommand || cmd == integrTopicCommand) {
			return cmd, match[2]
		}
	}
	return "", ""
}

func updateProjectNameToIDMap(projects []github.Project) {
	for _, project := range projects {
		projectNameToIDMap[project.Name] = project.ID
	}
}

type integrateRepo struct {
	Repo string `yaml:"repo"`
	Sha  string `yaml:"tagsha"`
}

type integrateTopicInfo struct {
	Message   string          `yaml:"message"`
	Milestone string          `yaml:"milestone"`
	Repos     []integrateRepo `yaml:"repos"`
}

func integrateTopic(log *logrus.Entry, ghc githubClient, config *plugins.Configuration,
	topic string, ce *github.GenericCommentEvent, gc git.ClientFactory) error {
	org := ce.Repo.Owner.Login

	var projects []github.Project
	var topicInfo integrateTopicInfo
	topicInfo.Message = fmt.Sprintf("Auto integrate from topic: %s by @%s", topic, ce.User.Login)
	topicInfo.Milestone = configForMilestone(optionsForRepo(config, org, ""))

	// see if the project in the same org
	repoProjects, err := ghc.GetOrgProjects(org)
	searchProjectV2 := false
	if err == nil && len(repoProjects) > 0 {
		projects = append(projects, repoProjects...)

		// update projects info cache
		updateProjectNameToIDMap(projects)
		var projectColumns []github.ProjectColumn
		log.Infof("topic project cache: %v\n%v", projects, projectColumns)
		// get project id from cache
		if projectID, ok := projectNameToIDMap[topic]; ok {
			// get all columns for proposedProject
			projectColumns, err = ghc.GetProjectColumns(org, projectID)
			if err != nil {
				return err
			}

			for _, c := range projectColumns {
				if projectCards, err := ghc.GetColumnProjectCards(org, c.ID); err == nil {
					log.Infof("projectCards: %v", projectCards)
					for _, p := range projectCards {
						//example: https://api.github.com/repos/api-playground/projects-test/pull/3
						contentInfo := strings.Split(p.ContentURL, "/")
						log.Infof("Topic pr url: %s, pr info len: %d", p.ContentURL, len(contentInfo))
						if len(contentInfo) == 8 {
							pOwner := contentInfo[4]
							pRepo := contentInfo[5]
							pNumberStr := contentInfo[7]
							if pNumber, err := strconv.Atoi(pNumberStr); err == nil {
								if prInfo, err := ghc.GetPullRequest(pOwner, pRepo, pNumber); err == nil {
									var repo integrateRepo
									sha := prInfo.Head.SHA
									if prInfo.State == "closed" {
										sha = *prInfo.MergeSHA
									}
									repo.Repo = fmt.Sprintf("%s/%s", pOwner, pRepo)
									repo.Sha = sha
									topicInfo.Repos = append(topicInfo.Repos, repo)
								}
							} else {
								log.Warnf("Get Topic[%s] pr[%s] info failed: %v", topic, p.ContentURL, err)
							}
						}
					}
				} else {
					log.Errorf("Get Topic[%s]'s Column[%s] failed: %v", topic, c.Name, err)
				}
			}
		} else {
			log.Infof("topic[%s]'s projectV1 not found, continue search at V2", topic)
			searchProjectV2 = true
		}

	} else {
		searchProjectV2 = true
	}

	if searchProjectV2 {
		pNumber := getProjectsV2Number(log, ghc, org, topic)
		if pNumber < 0 {
			return fmt.Errorf("topic[%s]'s projectV2 not found", topic)
		}

		topicInfo.Repos = getProjectV2Items(log, ghc, org, pNumber)
	}

	if topicData, err := yaml.Marshal(topicInfo); err == nil {
		log.Infof("Topic[%s]'s integrate info: %s", topic, string(topicData))
		integratePrsRepo := configForRepo(optionsForRepo(config, org, ""))
		return createIntegratePr(log, ghc, integratePrsRepo, topic, ce, gc, topicData)
	}

	return nil
}

// func getProjectV2Items
// get ProjectV2 Items
func getProjectV2Items(log *logrus.Entry, ghc githubClient, org string, projectNumber githubql.Int) []integrateRepo {
	var reposInfo []integrateRepo

	vars := map[string]interface{}{
		"number":       projectNumber,
		"org":          githubql.String(org),
		"searchCursor": (*githubql.String)(nil),
	}

	requestStart := time.Now()
	var pageCount int
	for {
		pageCount++
		sq := searchProjectV2ItemsQuery{}
		if err := ghc.QueryWithGitHubAppsSupport(context.Background(), &sq, vars, org); err != nil {
			log.Warnf("Get org[%s]'s project[%d] items from projectv2 failed: %v", org, projectNumber, err)
		}

		for _, n := range sq.Organization.ProjectV2.Items.Nodes {
			if n.Type == "PULL_REQUEST" {
				pRepo := string(n.Content.PullRequest.BaseRepository.Name)
				pNumber := int(n.Content.PullRequest.Number)
				if prInfo, err := ghc.GetPullRequest(org, pRepo, pNumber); err == nil {
					var repo integrateRepo
					sha := prInfo.Head.SHA
					if prInfo.State == "closed" {
						sha = *prInfo.MergeSHA
					}
					repo.Repo = fmt.Sprintf("%s/%s", org, pRepo)
					repo.Sha = sha
					reposInfo = append(reposInfo, repo)
				}
			}
		}
		if !sq.Organization.ProjectV2.Items.PageInfo.HasNextPage {
			break
		}
		vars["searchCursor"] = githubql.NewString(sq.Organization.ProjectV2.Items.PageInfo.EndCursor)
	}

	log = log.WithFields(logrus.Fields{
		"duration":       time.Since(requestStart).String(),
		"search_pages":   pageCount,
		"project_number": projectNumber,
	})
	log.Debug("Finished query")

	return reposInfo
}

// func getProjectsV2Number
// get projectv2 number with name
func getProjectsV2Number(log *logrus.Entry, ghc githubClient, org, topic string) githubql.Int {
	projectNumber := githubql.Int(-1)
	vars := map[string]interface{}{
		"org":          githubql.String(org),
		"searchCursor": (*githubql.String)(nil),
	}

	requestStart := time.Now()
	var pageCount int
	found := false
	for {
		pageCount++
		sq := searchProjectsV2Query{}
		if err := ghc.QueryWithGitHubAppsSupport(context.Background(), &sq, vars, org); err != nil {
			log.Warnf("Get org[%s]'s topic[%s] from projectv2 failed with err: %v",
				org, topic, err)
		}

		for _, n := range sq.Organization.ProjectsV2.Nodes {
			if n.Title == githubql.String(topic) {
				projectNumber = n.Number
				found = true
				break
			}
		}
		if githubql.Boolean(found) || !sq.Organization.ProjectsV2.PageInfo.HasNextPage {
			break
		}
		vars["searchCursor"] = githubql.NewString(sq.Organization.ProjectsV2.PageInfo.EndCursor)
	}

	log = log.WithFields(logrus.Fields{
		"page_count":     pageCount,
		"duration":       time.Since(requestStart).String(),
		"project_number": projectNumber,
	})
	log.Infof("Get org[%s]'s topic[%s] number: %d", org, topic, projectNumber)
	log.Debug("Finished query")

	return projectNumber
}

func createIntegratePr(log *logrus.Entry, ghc githubClient, integratePrsRepo, topic string,
	ce *github.GenericCommentEvent, gc git.ClientFactory, topicData []byte) error {
	integratePrsRepoInfo := strings.Split(integratePrsRepo, "/")
	if len(integratePrsRepoInfo) != 2 {
		return fmt.Errorf("invalid integrate PRs Repo config: %s", integratePrsRepo)
	}

	integratePrOrg := integratePrsRepoInfo[0]
	integratePrRepo := integratePrsRepoInfo[1]
	repo, err := gc.ClientFor(integratePrOrg, integratePrRepo)
	if err != nil {
		return err
	}

	topicBranch := fmt.Sprintf("topics/%s", topic)
	var oldPr *github.PullRequest
	if repo.BranchExists(topicBranch) {
		err = repo.Checkout(topicBranch)
		// Find old pr number
		if prs, err := ghc.GetPullRequests(integratePrOrg, integratePrRepo); err == nil {
			for _, pr := range prs {
				if pr.Head.Ref == topicBranch {
					log.Infof("found integrated topic[%s] with pr[%v] at branch[%s]", topic, pr, topicBranch)
					oldPr = &pr
					break
				}
			}
		}
	} else {
		err = repo.CheckoutNewBranch(topicBranch)
	}

	if err != nil {
		return err
	}

	file, err := os.OpenFile(fmt.Sprintf("%s/%s", repo.Directory(), "integration.yml"), os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	_, err = file.Write(topicData)
	if err != nil {
		return err
	}

	botUser, err := ghc.BotUser()
	if err != nil {
		botUser = &github.UserData{
			Login: "deepin-ci-robot",
			Email: "packages@deepin.org",
		}
	}
	repo.Config("user.name", botUser.Login)
	repo.Config("user.email", botUser.Email)
	err = repo.Commit("chore: Update integration.yml", fmt.Sprintf("auto integrate from topic %s.", topic))
	if err != nil {
		if strings.Contains(err.Error(), "nothing to commit") {
			if oldPr != nil {
				return ghc.CreateComment(ce.Repo.Owner.Login, ce.Repo.Name, ce.Number, fmt.Sprintf("Alreadly latest topic integration with %s", oldPr.HTMLURL))
			}
		} else {
			return err
		}
	}

	// Push repo
	err = repo.PushToCentral(topicBranch, true)
	if err != nil {
		return err
	}

	// Create an new PR
	number, err := ghc.CreatePullRequest(
		integratePrOrg,
		integratePrRepo,
		fmt.Sprintf("auto integration with topic %s by @%s", topic, ce.User.Login),
		fmt.Sprintf("from %s.", ce.HTMLURL),
		topicBranch,
		"master",
		true,
	)
	if err != nil {
		return err
	}

	// Create Comment
	return ghc.CreateComment(ce.Repo.Owner.Login, ce.Repo.Name, ce.Number, fmt.Sprintf("https://github.com/deepin-community/Repository-Integration/pull/%d", number))
}

// optionsForRepo gets the plugins.Welcome struct that is applicable to the indicated repo.
func optionsForRepo(config *plugins.Configuration, org, repo string) *plugins.Topic {
	fullName := fmt.Sprintf("%s/%s", org, repo)

	// First search for repo config
	for _, c := range config.Topic {
		if !sets.NewString(c.Repos...).Has(fullName) {
			continue
		}
		return &c
	}

	// If you don't find anything, loop again looking for an org config
	for _, c := range config.Topic {
		if !sets.NewString(c.Repos...).Has(org) {
			continue
		}
		return &c
	}

	// Return an empty config, and default to defaultWelcomeMessage
	return &plugins.Topic{}
}

func configForRepo(options *plugins.Topic) string {
	if options.IntegrateRepo != "" {
		return options.IntegrateRepo
	}
	return defaultIntegrationRepo
}

func configForMilestone(options *plugins.Topic) string {
	if options.IntegrateMilestone != "" {
		return options.IntegrateMilestone
	}
	return ""
}
