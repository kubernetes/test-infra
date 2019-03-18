package scallywag

import (
	"context"
)

// Client is an interface defining interactions with git providers.
type Client interface {
	BotName() (string, error)
	Email() (string, error)
	IsMember(org, user string) (bool, error)
	ListOrgHooks(org string) ([]Hook, error)
	ListRepoHooks(org, repo string) ([]Hook, error)
	EditRepoHook(org, repo string, id int, req HookRequest) error
	EditOrgHook(org string, id int, req HookRequest) error
	CreateOrgHook(org string, req HookRequest) (int, error)
	CreateRepoHook(org, repo string, req HookRequest) (int, error)
	GetOrg(name string) (*Organization, error)
	EditOrg(name string, config Organization) (*Organization, error)
	ListOrgInvitations(org string) ([]OrgInvitation, error)
	ListOrgMembers(org, role string) ([]TeamMember, error)
	HasPermission(org, repo, user string, roles ...string) (bool, error)
	GetUserPermission(org, repo, user string) (string, error)
	UpdateOrgMembership(org, user string, admin bool) (*OrgMembership, error)
	RemoveOrgMembership(org, user string) error
	CreateComment(org, repo string, number int, comment string) error
	DeleteComment(org, repo string, id int) error
	EditComment(org, repo string, id int, comment string) error
	CreateCommentReaction(org, repo string, id int, reaction string) error
	CreateIssueReaction(org, repo string, id int, reaction string) error
	DeleteStaleComments(org, repo string, number int, comments []IssueComment, isStale func(IssueComment) bool) error
	ListIssueComments(org, repo string, number int) ([]IssueComment, error)
	GetPullRequests(org, repo string) ([]PullRequest, error)
	GetPullRequest(org, repo string, number int) (*PullRequest, error)
	GetPullRequestPatch(org, repo string, number int) ([]byte, error)
	CreatePullRequest(org, repo, title, body, head, base string, canModify bool) (int, error)
	UpdatePullRequest(org, repo string, number int, title, body *string, open *bool, branch *string, canModify *bool) error
	GetPullRequestChanges(org, repo string, number int) ([]PullRequestChange, error)
	ListPullRequestComments(org, repo string, number int) ([]ReviewComment, error)
	ListReviews(org, repo string, number int) ([]Review, error)
	CreateStatus(org, repo, SHA string, s Status) error
	ListStatuses(org, repo, ref string) ([]Status, error)
	GetRepo(owner, name string) (Repo, error)
	GetRepos(org string, isUser bool) ([]Repo, error)
	GetSingleCommit(org, repo, SHA string) (SingleCommit, error)
	GetBranches(org, repo string, onlyProtected bool) ([]Branch, error)
	RemoveBranchProtection(org, repo, branch string) error
	UpdateBranchProtection(org, repo, branch string, config BranchProtectionRequest) error
	AddRepoLabel(org, repo, label, description, color string) error
	UpdateRepoLabel(org, repo, label, newName, description, color string) error
	DeleteRepoLabel(org, repo, label string) error
	GetCombinedStatus(org, repo, ref string) (*CombinedStatus, error)
	GetRepoLabels(org, repo string) ([]Label, error)
	GetIssueLabels(org, repo string, number int) ([]Label, error)
	AddLabel(org, repo string, number int, label string) error
	RemoveLabel(org, repo string, number int, label string) error
	AssignIssue(org, repo string, number int, logins []string) error
	UnassignIssue(org, repo string, number int, logins []string) error
	CreateReview(org, repo string, number int, r DraftReview) error
	RequestReview(org, repo string, number int, logins []string) error
	UnrequestReview(org, repo string, number int, logins []string) error
	CloseIssue(org, repo string, number int) error
	ReopenIssue(org, repo string, number int) error
	ClosePR(org, repo string, number int) error
	ReopenPR(org, repo string, number int) error
	GetRef(org, repo, ref string) (string, error)
	DeleteRef(org, repo, ref string) error
	FindIssues(query, sort string, asc bool) ([]Issue, error)
	GetFile(org, repo, filepath, commit string) ([]byte, error)
	Query(ctx context.Context, q interface{}, vars map[string]interface{}) error
	CreateTeam(org string, team Team) (*Team, error)
	EditTeam(t Team) (*Team, error)
	DeleteTeam(id int) error
	ListTeams(org string) ([]Team, error)
	UpdateTeamMembership(id int, user string, maintainer bool) (*TeamMembership, error)
	RemoveTeamMembership(id int, user string) error
	ListTeamMembers(id int, role string) ([]TeamMember, error)
	ListTeamInvitations(id int) ([]OrgInvitation, error)
	Merge(org, repo string, pr int, details MergeDetails) error
	IsCollaborator(org, repo, user string) (bool, error)
	ListCollaborators(org, repo string) ([]User, error)
	CreateFork(owner, repo string) error
	ListIssueEvents(org, repo string, num int) ([]ListedIssueEvent, error)
	IsMergeable(org, repo string, number int, SHA string) (bool, error)
	ClearMilestone(org, repo string, num int) error
	SetMilestone(org, repo string, issueNum, milestoneNum int) error
	ListMilestones(org, repo string) ([]Milestone, error)
	ListPRCommits(org, repo string, number int) ([]RepositoryCommit, error)
	GetRepoProjects(owner, repo string) ([]Project, error)
	GetOrgProjects(org string) ([]Project, error)
	GetProjectColumns(projectID int) ([]ProjectColumn, error)
	CreateProjectCard(columnID int, projectCard ProjectCard) (*ProjectCard, error)
	GetColumnProjectCard(columnID int, cardNumber int) (*ProjectCard, error)
	MoveProjectCard(projectCardID int, newColumnID int) error
	DeleteProjectCard(projectCardID int) error
	TeamHasMember(teamID int, memberLogin string) (bool, error)
	Throttle(hourlyTokens, burst int)
}
