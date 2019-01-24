package scallywag

import context "golang.org/x/net/context"

type GitService struct{}

func (gs *GitService) UpdateIssue(ctx context.Context, req *Issue) (*Issue, error) {
	req.Body = "This issue body has been updated"
	return req, nil
}

func (gs *GitService) ListRepositories(*Organization, GitService_ListRepositoriesServer) error {
	return nil
}

func (gs *GitService) CreateRepository(context.Context, *RepositoryInfo) (*Repository, error) {
	return nil, nil
}

func (gs *GitService) GetRepository(context.Context, *RepositoryInfo) (*Repository, error) {
	return nil, nil
}

func (gs *GitService) DeleteRepository(context.Context, *Repository) (*Confirmation, error) {
	return nil, nil
}

func (gs *GitService) ForkRepository(context.Context, *ForkInfo) (*Repository, error) {
	return nil, nil
}

func (gs *GitService) RenameRepository(context.Context, *Repository) (*Repository, error) {
	return nil, nil
}

func (gs *GitService) ValidateRepositoryName(context.Context, *RepositoryInfo) (*Confirmation, error) {
	return nil, nil
}

func (gs *GitService) CreatePullRequest(context.Context, *PullRequest) (*PullRequest, error) {
	return nil, nil
}

func (gs *GitService) UpdatePullRequestStatus(context.Context, *PullRequest) (*Confirmation, error) {
	return nil, nil
}

func (gs *GitService) GetPullRequest(context.Context, *PullRequest) (*PullRequest, error) {
	return nil, nil
}

func (gs *GitService) GetPullRequestCommits(*PullRequest, GitService_GetPullRequestCommitsServer) error {
	return nil
}

func (gs *GitService) PullRequestLastCommitStatus(context.Context, *PullRequest) (*Commit, error) {
	return nil, nil
}

func (gs *GitService) ListCommitStatus(*Commit, GitService_ListCommitStatusServer) error {
	return nil
}

func (gs *GitService) UpdateCommitStatus(context.Context, *CommitStatus) (*CommitStatus, error) {
	return nil, nil
}

func (gs *GitService) MergePullRequest(context.Context, *PullRequest) (*Confirmation, error) {
	return nil, nil
}

func (gs *GitService) CreateWebhook(context.Context, *Webhook) (*Confirmation, error) {
	return nil, nil
}

func (gs *GitService) ListWebhooks(*Repository, GitService_ListWebhooksServer) error {
	return nil
}

func (gs *GitService) UpdateWebHook(*Webhook, GitService_UpdateWebHookServer) error {
	return nil
}

func (gs *GitService) GetIssue(context.Context, *Issue) (*Issue, error) {
	return nil, nil
}

func (gs *GitService) SearchIssues(context.Context, *IssueQuery) (*Issue, error) {
	return nil, nil
}

func (gs *GitService) CreateIssue(context.Context, *Issue) (*Issue, error) {
	return nil, nil
}

func (gs *GitService) HasIssues(context.Context, *Repository) (*Confirmation, error) {
	return nil, nil
}

func (gs *GitService) AddPRComment(context.Context, *PullRequestComment) (*PullRequestComment, error) {
	return nil, nil
}

func (gs *GitService) CreateIssueComment(context.Context, *IssueComment) (*IssueComment, error) {
	return nil, nil
}

func (gs *GitService) UpdateRelease(context.Context, *Release) (*Confirmation, error) {
	return nil, nil
}

func (gs *GitService) ListReleases(*Repository, GitService_ListReleasesServer) error {
	return nil
}
