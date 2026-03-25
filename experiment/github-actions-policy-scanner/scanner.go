/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
	yaml "gopkg.in/yaml.v3"
)

const (
	perPage          = 100
	lowRateReserve   = 50
	maxRetries       = 3
	minRateLimitWait = time.Second
)

type scanner struct {
	client *github.Client
}

type summary struct {
	OwnersScanned int
	ReposSeen     int
	ReposScanned  int
	ReposSkipped  int
	FilesScanned  int
	Findings      int
	ScanErrors    int
}

type finding struct {
	Owner   string
	Repo    string
	File    string
	Line    int
	Uses    string
	Message string
	Fix     string
}

type violation struct {
	Message string
	Fix     string
}

func newScanner(token, endpoint string) (*scanner, error) {
	httpClient := oauth2.NewClient(
		context.Background(),
		oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token}),
	)
	client := github.NewClient(httpClient)

	baseURL, err := normalizeGitHubAPIEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	client.BaseURL = baseURL

	return &scanner{client: client}, nil
}

func normalizeGitHubAPIEndpoint(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("endpoint was empty")
	}
	if !strings.HasSuffix(raw, "/") {
		raw += "/"
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse endpoint %q: %w", raw, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid endpoint %q: expected scheme and host", raw)
	}
	return parsed, nil
}

func (s *scanner) run(ctx context.Context, owners []string) (*summary, error) {
	result := &summary{}
	for _, owner := range owners {
		result.OwnersScanned++
		log.Printf("scanning owner=%s", owner)

		repos, err := s.listRepositories(ctx, owner)
		if err != nil {
			result.ScanErrors++
			log.Printf("scan_error owner=%s err=%v", owner, err)
			continue
		}

		result.ReposSeen += len(repos)
		for _, repo := range repos {
			s.scanRepository(ctx, owner, repo, result)
		}
	}

	return result, nil
}

func (s *scanner) listRepositories(ctx context.Context, owner string) ([]*github.Repository, error) {
	account, err := s.getAccount(ctx, owner)
	if err != nil {
		return nil, err
	}

	if strings.EqualFold(stringValue(account.Type), "Organization") {
		return s.listOrgRepositories(ctx, owner)
	}
	return s.listUserRepositories(ctx, owner)
}

func (s *scanner) getAccount(ctx context.Context, owner string) (*github.User, error) {
	var user *github.User
	_, err := s.doWithRetry(ctx, fmt.Sprintf("get_user owner=%s", owner), func() (*github.Response, error) {
		var (
			callResp *github.Response
			callErr  error
		)
		user, callResp, callErr = s.client.Users.Get(ctx, owner)
		return callResp, callErr
	})
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (s *scanner) listOrgRepositories(ctx context.Context, org string) ([]*github.Repository, error) {
	var all []*github.Repository

	for page := 1; page != 0; {
		opts := &github.RepositoryListByOrgOptions{
			Type: "all",
			ListOptions: github.ListOptions{
				Page:    page,
				PerPage: perPage,
			},
		}

		var (
			repos []*github.Repository
		)
		resp, err := s.doWithRetry(ctx, fmt.Sprintf("list_repos owner=%s page=%d source=org", org, page), func() (*github.Response, error) {
			var (
				callResp *github.Response
				callErr  error
			)
			repos, callResp, callErr = s.client.Repositories.ListByOrg(ctx, org, opts)
			return callResp, callErr
		})
		if err != nil {
			return nil, err
		}

		all = append(all, repos...)
		page = resp.NextPage
	}

	return all, nil
}

func (s *scanner) listUserRepositories(ctx context.Context, user string) ([]*github.Repository, error) {
	var all []*github.Repository

	for page := 1; page != 0; {
		opts := &github.RepositoryListOptions{
			Type: "owner",
			ListOptions: github.ListOptions{
				Page:    page,
				PerPage: perPage,
			},
		}

		var repos []*github.Repository
		resp, err := s.doWithRetry(ctx, fmt.Sprintf("list_repos owner=%s page=%d source=user", user, page), func() (*github.Response, error) {
			var (
				callResp *github.Response
				callErr  error
			)
			repos, callResp, callErr = s.client.Repositories.List(ctx, user, opts)
			return callResp, callErr
		})
		if err != nil {
			return nil, err
		}

		all = append(all, repos...)
		page = resp.NextPage
	}

	return all, nil
}

func (s *scanner) scanRepository(ctx context.Context, owner string, repo *github.Repository, result *summary) {
	name := stringValue(repo.Name)
	if name == "" {
		result.ReposSkipped++
		log.Printf("skipping owner=%s repo=<unknown> reason=missing_name", owner)
		return
	}

	if boolValue(repo.Archived) {
		result.ReposSkipped++
		log.Printf("skipping owner=%s repo=%s reason=archived", owner, name)
		return
	}

	if stringValue(repo.DefaultBranch) == "" {
		result.ReposSkipped++
		log.Printf("skipping owner=%s repo=%s reason=missing_default_branch", owner, name)
		return
	}

	log.Printf("scanning owner=%s repo=%s", owner, name)

	err := s.walkDirectory(ctx, owner, name, ".github", result)
	if isNotFound(err) {
		result.ReposSkipped++
		log.Printf("skipping owner=%s repo=%s reason=no_.github", owner, name)
		return
	}
	result.ReposScanned++
	if err == nil {
		return
	}

	result.ScanErrors++
	log.Printf("scan_error owner=%s repo=%s path=.github err=%v", owner, name, err)
}

func (s *scanner) walkDirectory(ctx context.Context, owner, repo, dir string, result *summary) error {
	_, entries, err := s.getContents(ctx, owner, repo, dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		entryPath := stringValue(entry.Path)
		switch stringValue(entry.Type) {
		case "dir":
			// TODO: Skip known non-action subtrees under .github/ if API call volume becomes a problem.
			if err := s.walkDirectory(ctx, owner, repo, entryPath, result); err != nil {
				result.ScanErrors++
				log.Printf("scan_error owner=%s repo=%s path=%s err=%v", owner, repo, entryPath, err)
			}
		case "file":
			if !isCandidateFile(entryPath) {
				continue
			}
			if err := s.scanFile(ctx, owner, repo, entryPath, result); err != nil {
				result.ScanErrors++
				log.Printf("scan_error owner=%s repo=%s file=%s err=%v", owner, repo, entryPath, err)
			}
		}
	}

	return nil
}

func (s *scanner) scanFile(ctx context.Context, owner, repo, path string, result *summary) error {
	file, _, err := s.getContents(ctx, owner, repo, path)
	if err != nil {
		return err
	}
	if file == nil {
		return fmt.Errorf("path %q returned no file content", path)
	}

	content, err := file.GetContent()
	if err != nil {
		return fmt.Errorf("decode %q: %w", path, err)
	}

	result.FilesScanned++
	findings, err := scanUses(owner, repo, path, content)
	if err != nil {
		return fmt.Errorf("parse %q: %w", path, err)
	}
	for _, finding := range findings {
		result.Findings++
		log.Printf(
			"VIOLATION owner=%s repo=%s file=%s line=%d uses=%q message=%q fix=%q",
			finding.Owner,
			finding.Repo,
			finding.File,
			finding.Line,
			finding.Uses,
			finding.Message,
			finding.Fix,
		)
	}

	return nil
}

func (s *scanner) getContents(ctx context.Context, owner, repo, path string) (*github.RepositoryContent, []*github.RepositoryContent, error) {
	var (
		fileContent      *github.RepositoryContent
		directoryContent []*github.RepositoryContent
	)

	_, err := s.doWithRetry(ctx, fmt.Sprintf("get_contents owner=%s repo=%s path=%s", owner, repo, path), func() (*github.Response, error) {
		var (
			callResp *github.Response
			callErr  error
		)
		fileContent, directoryContent, callResp, callErr = s.client.Repositories.GetContents(ctx, owner, repo, path, nil)
		return callResp, callErr
	})
	if err != nil {
		return nil, nil, err
	}

	return fileContent, directoryContent, nil
}

func (s *scanner) doWithRetry(ctx context.Context, action string, call func() (*github.Response, error)) (*github.Response, error) {
	var (
		resp *github.Response
		err  error
	)

	for attempt := 0; ; attempt++ {
		resp, err = call()
		if err == nil {
			if sleep := lowRateSleep(resp); sleep > 0 {
				log.Printf(
					"rate_limit_wait action=%q reason=low_remaining remaining=%d sleep=%s",
					action,
					resp.Rate.Remaining,
					sleep,
				)
				if sleepErr := sleepContext(ctx, sleep); sleepErr != nil {
					return resp, sleepErr
				}
			}
			return resp, nil
		}

		delay, retry := retryDelay(err, attempt)
		if !retry {
			return resp, err
		}

		log.Printf("retrying action=%q attempt=%d sleep=%s err=%v", action, attempt+1, delay, err)
		if sleepErr := sleepContext(ctx, delay); sleepErr != nil {
			return resp, sleepErr
		}
	}
}

func lowRateSleep(resp *github.Response) time.Duration {
	if resp == nil {
		return 0
	}
	if resp.Rate.Remaining > lowRateReserve {
		return 0
	}

	delay := time.Until(resp.Rate.Reset.Time) + minRateLimitWait
	if delay <= 0 {
		return 0
	}
	return delay
}

func retryDelay(err error, attempt int) (time.Duration, bool) {
	if attempt >= maxRetries {
		return 0, false
	}

	switch err := err.(type) {
	case *github.RateLimitError:
		delay := time.Until(err.Rate.Reset.Time) + minRateLimitWait
		if delay < minRateLimitWait {
			delay = minRateLimitWait
		}
		return delay, true
	case *github.AbuseRateLimitError:
		if err.RetryAfter != nil && *err.RetryAfter > 0 {
			return *err.RetryAfter, true
		}
		delay := time.Duration(60*(1<<attempt)) * time.Second
		if delay > 120*time.Second {
			delay = 120 * time.Second
		}
		return delay, true
	case *github.ErrorResponse:
		if err.Response == nil {
			return 0, false
		}
		status := err.Response.StatusCode
		if (status == http.StatusForbidden || status == http.StatusTooManyRequests) && err.Response.Header != nil {
			if delay, ok := retryAfter(err.Response.Header.Get("Retry-After")); ok {
				return delay, true
			}
		}
		if status >= 500 && status <= 599 {
			return time.Duration(1<<attempt) * time.Second, true
		}
	}

	return 0, false
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func retryAfter(raw string) (time.Duration, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}

	seconds, err := strconv.Atoi(raw)
	if err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second, true
	}

	when, err := http.ParseTime(raw)
	if err != nil {
		return 0, false
	}

	delay := time.Until(when)
	if delay <= 0 {
		return minRateLimitWait, true
	}
	return delay, true
}

func scanUses(owner, repo, filePath, content string) ([]finding, error) {
	decoder := yaml.NewDecoder(strings.NewReader(content))
	var findings []finding

	for {
		var doc yaml.Node
		err := decoder.Decode(&doc)
		switch {
		case err == nil:
		case errors.Is(err, io.EOF):
			return findings, nil
		default:
			return nil, err
		}

		if len(doc.Content) == 0 {
			continue
		}

		root := doc.Content[0]
		if strings.HasPrefix(filePath, ".github/workflows/") {
			findings = append(findings, scanWorkflowNode(owner, repo, filePath, root)...)
			continue
		}

		findings = append(findings, scanActionMetadataNode(owner, repo, filePath, root)...)
	}
}

func classifyUsesValue(value string) (violation, bool) {
	if value == "" {
		return violation{}, false
	}
	if strings.HasPrefix(value, "./") {
		return violation{}, false
	}
	if strings.HasPrefix(value, "docker://") {
		return violation{}, false
	}
	if strings.Contains(value, "${{") {
		return violation{
			Message: "dynamic ref; cannot verify immutable SHA pin",
			Fix:     "replace the dynamic ref with a full commit SHA",
		}, true
	}
	if !strings.Contains(value, "/") {
		return violation{}, false
	}

	at := strings.LastIndex(value, "@")
	if at == -1 || at == len(value)-1 {
		return violation{
			Message: "missing ref; expected 40-character commit SHA",
			Fix:     "add a full commit SHA after @",
		}, true
	}

	ref := value[at+1:]
	if isSHA40(ref) {
		return violation{}, false
	}

	return violation{
		Message: "mutable ref; expected 40-character commit SHA",
		Fix:     "replace the ref with a full commit SHA",
	}, true
}

func scanWorkflowNode(owner, repo, filePath string, root *yaml.Node) []finding {
	jobsValue := mappingValue(root, "jobs")
	if jobsValue == nil || jobsValue.Kind != yaml.MappingNode {
		return nil
	}

	findings := []finding{}
	for i := 0; i+1 < len(jobsValue.Content); i += 2 {
		jobValue := jobsValue.Content[i+1]
		if jobValue.Kind != yaml.MappingNode {
			continue
		}

		if usesNode := mappingValue(jobValue, "uses"); usesNode != nil {
			if finding, ok := findingForUsesNode(owner, repo, filePath, usesNode); ok {
				findings = append(findings, finding)
			}
		}

		stepsValue := mappingValue(jobValue, "steps")
		if stepsValue == nil || stepsValue.Kind != yaml.SequenceNode {
			continue
		}
		for _, stepNode := range stepsValue.Content {
			if stepNode.Kind != yaml.MappingNode {
				continue
			}
			if usesNode := mappingValue(stepNode, "uses"); usesNode != nil {
				if finding, ok := findingForUsesNode(owner, repo, filePath, usesNode); ok {
					findings = append(findings, finding)
				}
			}
		}
	}

	return findings
}

func scanActionMetadataNode(owner, repo, filePath string, root *yaml.Node) []finding {
	runsValue := mappingValue(root, "runs")
	if runsValue == nil || runsValue.Kind != yaml.MappingNode {
		return nil
	}

	stepsValue := mappingValue(runsValue, "steps")
	if stepsValue == nil || stepsValue.Kind != yaml.SequenceNode {
		return nil
	}

	findings := []finding{}
	for _, stepNode := range stepsValue.Content {
		if stepNode.Kind != yaml.MappingNode {
			continue
		}
		if usesNode := mappingValue(stepNode, "uses"); usesNode != nil {
			if finding, ok := findingForUsesNode(owner, repo, filePath, usesNode); ok {
				findings = append(findings, finding)
			}
		}
	}

	return findings
}

func findingForUsesNode(owner, repo, filePath string, usesNode *yaml.Node) (finding, bool) {
	value := strings.TrimSpace(usesNode.Value)
	issue, ok := classifyUsesValue(value)
	if !ok {
		return finding{}, false
	}

	return finding{
		Owner:   owner,
		Repo:    repo,
		File:    filePath,
		Line:    usesNode.Line,
		Uses:    value,
		Message: issue.Message,
		Fix:     issue.Fix,
	}, true
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func isSHA40(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, r := range value {
		if ('0' <= r && r <= '9') || ('a' <= r && r <= 'f') || ('A' <= r && r <= 'F') {
			continue
		}
		return false
	}
	return true
}

func isCandidateFile(path string) bool {
	if !strings.HasPrefix(path, ".github/") {
		return false
	}

	if strings.HasPrefix(path, ".github/workflows/") && (strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml")) {
		return true
	}

	name := path[strings.LastIndex(path, "/")+1:]
	return name == "action.yml" || name == "action.yaml"
}

func isNotFound(err error) bool {
	errResponse, ok := err.(*github.ErrorResponse)
	if !ok || errResponse.Response == nil {
		return false
	}
	return errResponse.Response.StatusCode == http.StatusNotFound
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func boolValue(value *bool) bool {
	if value == nil {
		return false
	}
	return *value
}
