/*
Copyright 2019 The Kubernetes Authors.

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

package bugzilla

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"k8s.io/test-infra/prow/version"
)

const (
	methodField = "method"
)

type Client interface {
	Endpoint() string
	// GetBug retrieves a Bug from the server
	GetBug(id int) (*Bug, error)
	// GetComments gets a list of comments for a specific bug ID.
	// https://bugzilla.readthedocs.io/en/latest/api/core/v1/comment.html#get-comments
	GetComments(id int) ([]Comment, error)
	// GetExternalBugPRsOnBug retrieves external bugs on a Bug from the server
	// and returns any that reference a Pull Request in GitHub
	// https://bugzilla.readthedocs.io/en/latest/api/core/v1/bug.html#get-bug
	GetExternalBugPRsOnBug(id int) ([]ExternalBug, error)
	// GetSubComponentsOnBug retrieves a the list of SubComponents of the bug.
	// SubComponents are a Red Hat bugzilla specific extra field.
	GetSubComponentsOnBug(id int) (map[string][]string, error)
	// GetClones gets the list of bugs that the provided bug blocks that also have a matching summary.
	GetClones(bug *Bug) ([]*Bug, error)
	// CreateBug creates a new bug on the server.
	// https://bugzilla.readthedocs.io/en/latest/api/core/v1/bug.html#create-bug
	CreateBug(bug *BugCreate) (int, error)
	// CreateComment creates a new bug on the server.
	// https://bugzilla.redhat.com/docs/en/html/api/core/v1/comment.html#create-comments
	CreateComment(bug *CommentCreate) (int, error)
	// CloneBug clones a bug by creating a new bug with the same fields, copying the description, and updating the bug to depend on the original bug
	CloneBug(bug *Bug) (int, error)
	// UpdateBug updates the fields of a bug on the server
	// https://bugzilla.readthedocs.io/en/latest/api/core/v1/bug.html#update-bug
	UpdateBug(id int, update BugUpdate) error
	// AddPullRequestAsExternalBug attempts to add a PR to the external tracker list.
	// External bugs are assumed to fall under the type identified by their hostname,
	// so we will provide https://github.com/ here for the URL identifier. We return
	// any error as well as whether a change was actually made.
	// This will be done via JSONRPC:
	// https://bugzilla.redhat.com/docs/en/html/integrating/api/Bugzilla/Extension/ExternalBugs/WebService.html#add-external-bug
	AddPullRequestAsExternalBug(id int, org, repo string, num int) (bool, error)
	// RemovePullRequestAsExternalBug attempts to remove a PR from the external tracker list.
	// External bugs are assumed to fall under the type identified by their hostname,
	// so we will provide https://github.com/ here for the URL identifier. We return
	// any error as well as whether a change was actually made.
	// This will be done via JSONRPC:
	// https://bugzilla.redhat.com/docs/en/html/integrating/api/Bugzilla/Extension/ExternalBugs/WebService.html#remove-external-bug
	RemovePullRequestAsExternalBug(id int, org, repo string, num int) (bool, error)
	// GetAllClones returns all the clones of the bug including itself
	// Differs from GetClones as GetClones only gets the child clones which are one level lower
	GetAllClones(bug *Bug) ([]*Bug, error)
	// GetRootForClone returns the original bug.
	GetRootForClone(bug *Bug) (*Bug, error)
	// SetRoundTripper sets a custom implementation of RoundTripper as the Transport for http.Client
	SetRoundTripper(t http.RoundTripper)

	// ForPlugin and ForSubcomponent allow for the logger used in the client
	// to be created in a more specific manner when spawning parallel workers
	ForPlugin(plugin string) Client
	ForSubcomponent(subcomponent string) Client
	WithFields(fields logrus.Fields) Client
}

// NewClient returns a bugzilla client.
func NewClient(getAPIKey func() []byte, endpoint string, githubExternalTrackerId uint) Client {
	return &client{
		logger: logrus.WithField("client", "bugzilla"),
		delegate: &delegate{
			client:                  &http.Client{},
			endpoint:                endpoint,
			githubExternalTrackerId: githubExternalTrackerId,
			getAPIKey:               getAPIKey,
		},
	}
}

// SetRoundTripper sets the Transport in http.Client to a custom RoundTripper
func (c *client) SetRoundTripper(t http.RoundTripper) {
	c.client.Transport = t
}

// newBugDetailsCache is a constructor for bugDetailsCache
func newBugDetailsCache() *bugDetailsCache {
	return &bugDetailsCache{cache: map[int]Bug{}}
}

// bugDetailsCache holds the already retrieved bug details
type bugDetailsCache struct {
	cache map[int]Bug
	lock  sync.Mutex
}

// get retrieves bug details from the cache and is thread safe
func (bd *bugDetailsCache) get(key int) (bug Bug, exists bool) {
	bd.lock.Lock()
	defer bd.lock.Unlock()
	entry, ok := bd.cache[key]
	return entry, ok
}

// set stores the bug details in the cache and is thread safe
func (bd *bugDetailsCache) set(key int, value Bug) {
	bd.lock.Lock()
	defer bd.lock.Unlock()
	bd.cache[key] = value
}

// list returns a slice of all bugs in the cache
func (bd *bugDetailsCache) list() []Bug {
	bd.lock.Lock()
	defer bd.lock.Unlock()
	result := make([]Bug, 0, len(bd.cache))
	for _, bug := range bd.cache {
		result = append(result, bug)
	}
	return result
}

// client interacts with the Bugzilla api.
type client struct {
	// If logger is non-nil, log all method calls with it.
	logger *logrus.Entry
	// identifier is used to add more identification to the user-agent header
	identifier string
	*delegate
}

// ForPlugin clones the client, keeping the underlying delegate the same but adding
// a plugin identifier and log field
func (c *client) ForPlugin(plugin string) Client {
	return c.forKeyValue("plugin", plugin)
}

// ForSubcomponent clones the client, keeping the underlying delegate the same but adding
// an identifier and log field
func (c *client) ForSubcomponent(subcomponent string) Client {
	return c.forKeyValue("subcomponent", subcomponent)
}

func (c *client) forKeyValue(key, value string) Client {
	return &client{
		identifier: value,
		logger:     c.logger.WithField(key, value),
		delegate:   c.delegate,
	}
}

func (c *client) userAgent() string {
	if c.identifier != "" {
		return version.UserAgentWithIdentifier(c.identifier)
	}
	return version.UserAgent()
}

// WithFields clones the client, keeping the underlying delegate the same but adding
// fields to the logging context
func (c *client) WithFields(fields logrus.Fields) Client {
	return &client{
		logger:   c.logger.WithFields(fields),
		delegate: c.delegate,
	}
}

// delegate actually does the work to talk to Bugzilla
type delegate struct {
	client                  *http.Client
	endpoint                string
	githubExternalTrackerId uint
	getAPIKey               func() []byte
}

// the client is a Client impl
var _ Client = &client{}

func (c *client) Endpoint() string {
	return c.endpoint
}

// GetBug retrieves a Bug from the server
// https://bugzilla.readthedocs.io/en/latest/api/core/v1/bug.html#get-bug
func (c *client) GetBug(id int) (*Bug, error) {
	logger := c.logger.WithFields(logrus.Fields{methodField: "GetBug", "id": id})
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/rest/bug/%d", c.endpoint, id), nil)
	if err != nil {
		return nil, err
	}
	raw, err := c.request(req, logger)
	if err != nil {
		return nil, err
	}
	var parsedResponse struct {
		Bugs []*Bug `json:"bugs,omitempty"`
	}
	if err := json.Unmarshal(raw, &parsedResponse); err != nil {
		return nil, fmt.Errorf("could not unmarshal response body: %v", err)
	}
	if len(parsedResponse.Bugs) != 1 {
		return nil, fmt.Errorf("did not get one bug, but %d: %v", len(parsedResponse.Bugs), parsedResponse)
	}
	return parsedResponse.Bugs[0], nil
}

func getClones(c Client, bug *Bug) ([]*Bug, error) {
	var errs []error
	clones := []*Bug{}
	for _, dependentID := range bug.Blocks {
		dependent, err := c.GetBug(dependentID)
		if err != nil {
			errs = append(errs, fmt.Errorf("Failed to get dependent bug #%d: %v", dependentID, err))
			continue
		}
		if dependent.Summary == bug.Summary {
			clones = append(clones, dependent)
		}
	}
	return clones, utilerrors.NewAggregate(errs)
}

// GetClones gets the list of bugs that the provided bug blocks that also have a matching summary.
func (c *client) GetClones(bug *Bug) ([]*Bug, error) {
	return getClones(c, bug)
}

// Gets children clones recursively using a mechanism similar to bfs
func getRecursiveClones(c Client, root *Bug) ([]*Bug, error) {
	var errs []error
	var bug *Bug
	clones := []*Bug{}
	childrenQ := []*Bug{}
	childrenQ = append(childrenQ, root)
	// FYI Cannot think of any situation for circular clones
	// But might need to revisit in case there are infinite loops at any point
	for len(childrenQ) > 0 {
		bug, childrenQ = childrenQ[0], childrenQ[1:]
		clones = append(clones, bug)
		children, err := getClones(c, bug)
		if err != nil {
			errs = append(errs, fmt.Errorf("Error finding clones Bug#%d: %v", bug.ID, err))
		}
		if len(children) > 0 {
			childrenQ = append(childrenQ, children...)
		}
	}
	return clones, utilerrors.NewAggregate(errs)
}

// getImmediateParents gets the Immediate parents of bugs with a matching summary
func getImmediateParents(c Client, bug *Bug) ([]*Bug, error) {
	var errs []error
	parents := []*Bug{}
	// One option would be to return as soon as the first parent is found
	// ideally that should be enough, although there is a check in the getRootForClone function to verify this
	// Logs would need to be monitored to verify this behavior
	for _, parentID := range bug.DependsOn {
		parent, err := c.GetBug(parentID)
		if err != nil {
			errs = append(errs, fmt.Errorf("Failed to get parent bug #%d: %v", parentID, err))
			continue
		}
		if parent.Summary == bug.Summary {
			parents = append(parents, parent)
		}
	}
	return parents, utilerrors.NewAggregate(errs)
}

func getRootForClone(c Client, bug *Bug) (*Bug, error) {
	curr := bug
	var errs []error
	for len(curr.DependsOn) > 0 {
		parent, err := getImmediateParents(c, curr)
		if err != nil {
			errs = append(errs, err)
		}
		switch l := len(parent); {
		case l <= 0:
			return curr, utilerrors.NewAggregate(errs)
		case l == 1:
			curr = parent[0]
		case l > 1:
			curr = parent[0]
			errs = append(errs, fmt.Errorf("More than one parent found for bug #%d", curr.ID))
		}
	}
	return curr, utilerrors.NewAggregate(errs)
}

// GetRootForClone returns the original bug.
func (c *client) GetRootForClone(bug *Bug) (*Bug, error) {
	return getRootForClone(c, bug)
}

// GetAllClones returns all the clones of the bug including itself
// Differs from GetClones as GetClones only gets the child clones which are one level lower
func (c *client) GetAllClones(bug *Bug) ([]*Bug, error) {
	bugCache := newBugDetailsCache()
	return getAllClones(c, bug, bugCache)
}

func getAllClones(c Client, bug *Bug, bugCache *bugDetailsCache) (clones []*Bug, err error) {

	clones = []*Bug{}
	bugCache.set(bug.ID, *bug)
	err = getAllLinkedBugs(c, bug.ID, bugCache, nil)
	if err != nil {
		return nil, err
	}
	cachedBugs := bugCache.list()
	for index, node := range cachedBugs {
		if node.Summary == bug.Summary {
			clones = append(clones, &cachedBugs[index])
		}
	}
	sort.SliceStable(clones, func(i, j int) bool {
		return clones[i].ID < clones[j].ID
	})
	return clones, nil
}

// Parallel implementation for getAllClones - spawns threads to go up and down the tree
// Also parallelizes the getBug calls if bug has multiple bugs in DependsOn/Blocks
func getAllLinkedBugs(c Client, bugID int, bugCache *bugDetailsCache, errGroup *errgroup.Group) error {
	var shouldWait bool
	if errGroup == nil {
		shouldWait = true
		errGroup = new(errgroup.Group)
	}
	bugObj, cacheHasBug := bugCache.get(bugID)
	if !cacheHasBug {
		bug, err := c.GetBug(bugID)
		if err != nil {
			return err
		}
		bugObj = *bug
	}
	errGroup.Go(func() error {
		return traverseUp(c, &bugObj, bugCache, errGroup)
	})
	errGroup.Go(func() error {
		return traverseDown(c, &bugObj, bugCache, errGroup)
	})

	if shouldWait {
		return errGroup.Wait()
	}
	return nil
}

func traverseUp(c Client, bug *Bug, bugCache *bugDetailsCache, errGroup *errgroup.Group) error {
	for _, dependsOnID := range bug.DependsOn {
		dependsOnID := dependsOnID
		errGroup.Go(func() error {
			_, alreadyFetched := bugCache.get(dependsOnID)
			if alreadyFetched {
				return nil
			}
			parent, err := c.GetBug(dependsOnID)
			if err != nil {
				return err
			}
			bugCache.set(parent.ID, *parent)
			if bug.Summary == parent.Summary {
				return getAllLinkedBugs(c, parent.ID, bugCache, errGroup)
			}
			return nil
		})
	}
	return nil
}

func traverseDown(c Client, bug *Bug, bugCache *bugDetailsCache, errGroup *errgroup.Group) error {
	for _, childID := range bug.Blocks {
		childID := childID
		errGroup.Go(func() error {
			_, alreadyFetched := bugCache.get(childID)
			if alreadyFetched {
				return nil
			}
			child, err := c.GetBug(childID)
			if err != nil {
				return err
			}

			bugCache.set(child.ID, *child)
			if bug.Summary == child.Summary {
				return getAllLinkedBugs(c, child.ID, bugCache, errGroup)
			}
			return nil
		})
	}
	return nil
}

// GetSubComponentsOnBug retrieves a the list of SubComponents of the bug.
// SubComponents are a Red Hat bugzilla specific extra field.
func (c *client) GetSubComponentsOnBug(id int) (map[string][]string, error) {
	logger := c.logger.WithFields(logrus.Fields{methodField: "GetSubComponentsOnBug", "id": id})
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/rest/bug/%d", c.endpoint, id), nil)
	if err != nil {
		return nil, err
	}
	values := req.URL.Query()
	values.Add("include_fields", "sub_components")
	req.URL.RawQuery = values.Encode()
	raw, err := c.request(req, logger)
	if err != nil {
		return nil, err
	}
	var parsedResponse struct {
		Bugs []struct {
			SubComponents map[string][]string `json:"sub_components"`
		} `json:"bugs"`
	}
	if err := json.Unmarshal(raw, &parsedResponse); err != nil {
		return nil, fmt.Errorf("could not unmarshal response body: %v", err)
	}
	// if there is no subcomponent, return an empty struct
	if parsedResponse.Bugs == nil || len(parsedResponse.Bugs) == 0 {
		return map[string][]string{}, nil
	}
	// make sure there is only 1 bug
	if len(parsedResponse.Bugs) != 1 {
		return nil, fmt.Errorf("did not get one bug, but %d: %v", len(parsedResponse.Bugs), parsedResponse)
	}
	return parsedResponse.Bugs[0].SubComponents, nil
}

// GetExternalBugPRsOnBug retrieves external bugs on a Bug from the server
// and returns any that reference a Pull Request in GitHub
// https://bugzilla.readthedocs.io/en/latest/api/core/v1/bug.html#get-bug
func (c *client) GetExternalBugPRsOnBug(id int) ([]ExternalBug, error) {
	logger := c.logger.WithFields(logrus.Fields{methodField: "GetExternalBugPRsOnBug", "id": id})
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/rest/bug/%d", c.endpoint, id), nil)
	if err != nil {
		return nil, err
	}
	values := req.URL.Query()
	values.Add("include_fields", "external_bugs")
	req.URL.RawQuery = values.Encode()
	raw, err := c.request(req, logger)
	if err != nil {
		return nil, err
	}
	var parsedResponse struct {
		Bugs []struct {
			ExternalBugs []ExternalBug `json:"external_bugs"`
		} `json:"bugs"`
	}
	if err := json.Unmarshal(raw, &parsedResponse); err != nil {
		return nil, fmt.Errorf("could not unmarshal response body: %v", err)
	}
	if len(parsedResponse.Bugs) != 1 {
		return nil, fmt.Errorf("did not get one bug, but %d: %v", len(parsedResponse.Bugs), parsedResponse)
	}
	var prs []ExternalBug
	for _, bug := range parsedResponse.Bugs[0].ExternalBugs {
		if bug.BugzillaBugID != id {
			continue
		}
		if bug.Type.URL != "https://github.com/" {
			// TODO: skuznets: figure out how to honor the endpoints given to the GitHub client to support enterprise here
			continue
		}
		org, repo, num, err := PullFromIdentifier(bug.ExternalBugID)
		if IsIdentifierNotForPullErr(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("could not parse external identifier %q as pull: %v", bug.ExternalBugID, err)
		}
		bug.Org = org
		bug.Repo = repo
		bug.Num = num
		prs = append(prs, bug)
	}
	return prs, nil
}

// UpdateBug updates the fields of a bug on the server
// https://bugzilla.readthedocs.io/en/latest/api/core/v1/bug.html#update-bug
func (c *client) UpdateBug(id int, update BugUpdate) error {
	logger := c.logger.WithFields(logrus.Fields{methodField: "UpdateBug", "id": id, "update": update})
	body, err := json.Marshal(update)
	if err != nil {
		return fmt.Errorf("failed to marshal update payload: %v", err)
	}
	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/rest/bug/%d", c.endpoint, id), bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	_, err = c.request(req, logger)
	return err
}

// CreateBug creates a new bug on the server.
// https://bugzilla.readthedocs.io/en/latest/api/core/v1/bug.html#create-bug
func (c *client) CreateBug(bug *BugCreate) (int, error) {
	logger := c.logger.WithFields(logrus.Fields{methodField: "CreateBug", "bug": bug})
	body, err := json.Marshal(bug)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal create payload: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/rest/bug", c.endpoint), bytes.NewBuffer(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.request(req, logger)
	if err != nil {
		return 0, err
	}
	var idStruct struct {
		ID int `json:"id,omitempty"`
	}
	err = json.Unmarshal(resp, &idStruct)
	if err != nil {
		return 0, fmt.Errorf("failed to unmarshal server response: %v", err)
	}
	return idStruct.ID, nil
}

func (c *client) CreateComment(comment *CommentCreate) (int, error) {
	logger := c.logger.WithFields(logrus.Fields{methodField: "CreateComment", "bug": comment.ID})
	body, err := json.Marshal(comment)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal create payload: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/rest/bug/%d/comment", c.endpoint, comment.ID), bytes.NewBuffer(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.request(req, logger)
	if err != nil {
		return 0, err
	}
	var idStruct struct {
		ID int `json:"id,omitempty"`
	}
	err = json.Unmarshal(resp, &idStruct)
	if err != nil {
		return 0, fmt.Errorf("failed to unmarshal server response: %v", err)
	}
	return idStruct.ID, nil
}

func cloneBugStruct(bug *Bug, subcomponents map[string][]string, comments []Comment) *BugCreate {
	newBug := &BugCreate{
		Alias:           bug.Alias,
		AssignedTo:      bug.AssignedTo,
		CC:              bug.CC,
		Component:       bug.Component,
		Flags:           bug.Flags,
		Groups:          bug.Groups,
		Keywords:        bug.Keywords,
		OperatingSystem: bug.OperatingSystem,
		Platform:        bug.Platform,
		Priority:        bug.Priority,
		Product:         bug.Product,
		QAContact:       bug.QAContact,
		Severity:        bug.Severity,
		Summary:         bug.Summary,
		TargetMilestone: bug.TargetMilestone,
		Version:         bug.Version,
	}
	if len(subcomponents) > 0 {
		newBug.SubComponents = subcomponents
	}
	for _, comment := range comments {
		if comment.IsPrivate {
			newBug.CommentIsPrivate = true
			break
		}
	}
	var newDesc strings.Builder
	// The following builds a description comprising all the bug's comments formatted the same way that Bugzilla does on clone
	newDesc.WriteString(fmt.Sprintf("+++ This bug was initially created as a clone of Bug #%d +++\n\n", bug.ID))
	if len(comments) > 0 {
		newDesc.WriteString(comments[0].Text)
	}
	// This is a standard time layout string for golang, which formats the time `Mon Jan 2 15:04:05 -0700 MST 2006` to the layout we want
	bzTimeLayout := "2006-01-02 15:04:05 MST"
	for _, comment := range comments[1:] {
		// Header
		newDesc.WriteString("\n\n--- Additional comment from ")
		newDesc.WriteString(comment.Creator)
		newDesc.WriteString(" on ")
		newDesc.WriteString(comment.Time.UTC().Format(bzTimeLayout))
		newDesc.WriteString(" ---\n\n")

		// Comment
		newDesc.WriteString(comment.Text)
	}
	newBug.Description = newDesc.String()
	// make sure comment isn't above maximum length
	if len(newBug.Description) > 65535 {
		newBug.Description = fmt.Sprint(newBug.Description[:65532], "...")
	}
	return newBug
}

// clone handles the bz client calls for the bug cloning process and allows us to share the implementation
// between the real and fake client to prevent bugs from accidental discrepencies between the two.
func clone(c Client, bug *Bug) (int, error) {
	subcomponents, err := c.GetSubComponentsOnBug(bug.ID)
	if err != nil {
		return 0, fmt.Errorf("failed to check if bug has subcomponents: %v", err)
	}
	comments, err := c.GetComments(bug.ID)
	if err != nil {
		return 0, fmt.Errorf("failed to get parent bug's comments: %v", err)
	}
	id, err := c.CreateBug(cloneBugStruct(bug, subcomponents, comments))
	if err != nil {
		return id, err
	}
	bugUpdate := BugUpdate{
		DependsOn: &IDUpdate{
			Add: []int{bug.ID},
		},
	}
	for _, originalBlocks := range bug.Blocks {
		if bugUpdate.Blocks == nil {
			bugUpdate.Blocks = &IDUpdate{}
		}
		bugUpdate.Blocks.Add = append(bugUpdate.Blocks.Add, originalBlocks)
	}
	err = c.UpdateBug(id, bugUpdate)
	return id, err
}

// CloneBug clones a bug by creating a new bug with the same fields, copying the description, and updating the bug to depend on the original bug
func (c *client) CloneBug(bug *Bug) (int, error) {
	return clone(c, bug)
}

// GetComments gets a list of comments for a specific bug ID.
// https://bugzilla.readthedocs.io/en/latest/api/core/v1/comment.html#get-comments
func (c *client) GetComments(bugID int) ([]Comment, error) {
	logger := c.logger.WithFields(logrus.Fields{methodField: "GetComments", "id": bugID})
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/rest/bug/%d/comment", c.endpoint, bugID), nil)
	if err != nil {
		return nil, err
	}
	raw, err := c.request(req, logger)
	if err != nil {
		return nil, err
	}
	var parsedResponse struct {
		Bugs map[int]struct {
			Comments []Comment `json:"comments,omitempty"`
		} `json:"bugs,omitempty"`
	}
	if err := json.Unmarshal(raw, &parsedResponse); err != nil {
		return nil, fmt.Errorf("could not unmarshal response body: %v", err)
	}
	if len(parsedResponse.Bugs) != 1 {
		return nil, fmt.Errorf("did not get one bug, but %d: %v", len(parsedResponse.Bugs), parsedResponse)
	}
	return parsedResponse.Bugs[bugID].Comments, nil
}

func (c *client) request(req *http.Request, logger *logrus.Entry) ([]byte, error) {
	if apiKey := c.getAPIKey(); len(apiKey) > 0 {
		// some BugZilla servers are too old and can't handle the header.
		// some don't want the query parameter. We can set both and keep
		// everyone happy without negotiating on versions
		req.Header.Set("X-BUGZILLA-API-KEY", string(apiKey))
		values := req.URL.Query()
		values.Add("api_key", string(apiKey))
		req.URL.RawQuery = values.Encode()
	}
	if userAgent := c.userAgent(); userAgent != "" {
		req.Header.Add("User-Agent", userAgent)
	}
	start := time.Now()
	resp, err := c.client.Do(req)
	stop := time.Now()
	promLabels := prometheus.Labels(map[string]string{methodField: logger.Data[methodField].(string), "status": ""})
	if resp != nil {
		promLabels["status"] = strconv.Itoa(resp.StatusCode)
	}
	requestDurations.With(promLabels).Observe(float64(stop.Sub(start).Seconds()))
	if resp != nil {
		logger.WithField("response", resp.StatusCode).Debug("Got response from Bugzilla.")
	}
	if err != nil {
		code := -1
		if resp != nil {
			code = resp.StatusCode
		}
		return nil, &requestError{statusCode: code, message: err.Error()}
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.WithError(err).Warn("could not close response body")
		}
	}()
	raw, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("could not read response body: %v", err)
	}
	var error struct {
		Error   bool   `json:"error"`
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &error); err != nil && len(raw) > 0 {
		logger.WithError(err).Debug("could not read response body as error")
	}
	if error.Error {
		return nil, &requestError{statusCode: resp.StatusCode, bugzillaCode: error.Code, message: error.Message}
	} else if resp.StatusCode != http.StatusOK {
		return nil, &requestError{statusCode: resp.StatusCode, message: fmt.Sprintf("response code %d not %d", resp.StatusCode, http.StatusOK)}
	}
	return raw, nil
}

type requestError struct {
	statusCode   int
	bugzillaCode int
	message      string
}

func (e requestError) Error() string {
	if e.bugzillaCode != 0 {
		return fmt.Sprintf("code %d: %s", e.bugzillaCode, e.message)
	}
	return e.message
}

func IsNotFound(err error) bool {
	reqError, ok := err.(*requestError)
	if !ok {
		return false
	}
	return reqError.statusCode == http.StatusNotFound
}

func IsInvalidBugID(err error) bool {
	reqError, ok := err.(*requestError)
	if !ok {
		return false
	}
	return reqError.bugzillaCode == 101
}

func IsAccessDenied(err error) bool {
	reqError, ok := err.(*requestError)
	if !ok {
		return false
	}
	return reqError.bugzillaCode == 102
}

// AddPullRequestAsExternalBug attempts to add a PR to the external tracker list.
// External bugs are assumed to fall under the type identified by their hostname,
// so we will provide https://github.com/ here for the URL identifier. We return
// any error as well as whether a change was actually made.
// This will be done via JSONRPC:
// https://bugzilla.redhat.com/docs/en/html/integrating/api/Bugzilla/Extension/ExternalBugs/WebService.html#add-external-bug
func (c *client) AddPullRequestAsExternalBug(id int, org, repo string, num int) (bool, error) {
	logger := c.logger.WithFields(logrus.Fields{methodField: "AddExternalBug", "id": id, "org": org, "repo": repo, "num": num})
	pullIdentifier := IdentifierForPull(org, repo, num)
	bugIdentifier := ExternalBugIdentifier{
		ID: pullIdentifier,
	}
	if c.githubExternalTrackerId != 0 {
		bugIdentifier.TrackerID = int(c.githubExternalTrackerId)
	} else {
		bugIdentifier.Type = "https://github.com/"
	}
	rpcPayload := struct {
		// Version is the version of JSONRPC to use. All Bugzilla servers
		// support 1.0. Some support 1.1 and some support 2.0
		Version string `json:"jsonrpc"`
		Method  string `json:"method"`
		// Parameters must be specified in JSONRPC 1.0 as a structure in the first
		// index of this slice
		Parameters []AddExternalBugParameters `json:"params"`
		ID         string                     `json:"id"`
	}{
		Version: "1.0", // some Bugzilla servers support 2.0 but all support 1.0
		Method:  "ExternalBugs.add_external_bug",
		ID:      "identifier", // this is useful when fielding asynchronous responses, but not here
		Parameters: []AddExternalBugParameters{{
			APIKey:       string(c.getAPIKey()),
			BugIDs:       []int{id},
			ExternalBugs: []ExternalBugIdentifier{bugIdentifier},
		}},
	}
	body, err := json.Marshal(rpcPayload)
	if err != nil {
		return false, fmt.Errorf("failed to marshal JSONRPC payload: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/jsonrpc.cgi", c.endpoint), bytes.NewBuffer(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.request(req, logger)
	if err != nil {
		return false, err
	}
	var response struct {
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error,omitempty"`
		ID     string `json:"id"`
		Result *struct {
			Bugs []struct {
				ID      int `json:"id"`
				Changes struct {
					ExternalBugs struct {
						Added   string `json:"added"`
						Removed string `json:"removed"`
					} `json:"ext_bz_bug_map.ext_bz_bug_id"`
				} `json:"changes"`
			} `json:"bugs"`
		} `json:"result,omitempty"`
	}
	if err := json.Unmarshal(resp, &response); err != nil {
		return false, fmt.Errorf("failed to unmarshal JSONRPC response: %v", err)
	}
	if response.Error != nil {
		if response.Error.Code == 100500 && strings.Contains(response.Error.Message, `duplicate key value violates unique constraint "ext_bz_bug_map_bug_id_idx"`) {
			// adding the external bug failed since it is already added, this is not an error
			return false, nil
		}
		return false, fmt.Errorf("JSONRPC error %d: %v", response.Error.Code, response.Error.Message)
	}
	if response.ID != rpcPayload.ID {
		return false, fmt.Errorf("JSONRPC returned mismatched identifier, expected %s but got %s", rpcPayload.ID, response.ID)
	}
	changed := false
	if response.Result != nil {
		for _, bug := range response.Result.Bugs {
			if bug.ID == id {
				changed = changed || strings.Contains(bug.Changes.ExternalBugs.Added, pullIdentifier)
			}
		}
	}
	return changed, nil
}

// RemovePullRequestAsExternalBug attempts to remove a PR from the external tracker list.
// External bugs are assumed to fall under the type identified by their hostname,
// so we will provide https://github.com/ here for the URL identifier. We return
// any error as well as whether a change was actually made.
// This will be done via JSONRPC:
// https://bugzilla.redhat.com/docs/en/html/integrating/api/Bugzilla/Extension/ExternalBugs/WebService.html#remove-external-bug
func (c *client) RemovePullRequestAsExternalBug(id int, org, repo string, num int) (bool, error) {
	logger := c.logger.WithFields(logrus.Fields{methodField: "RemoveExternalBug", "id": id, "org": org, "repo": repo, "num": num})
	pullIdentifier := IdentifierForPull(org, repo, num)
	rpcPayload := struct {
		// Version is the version of JSONRPC to use. All Bugzilla servers
		// support 1.0. Some support 1.1 and some support 2.0
		Version string `json:"jsonrpc"`
		Method  string `json:"method"`
		// Parameters must be specified in JSONRPC 1.0 as a structure in the first
		// index of this slice
		Parameters []RemoveExternalBugParameters `json:"params"`
		ID         string                        `json:"id"`
	}{
		Version: "1.0", // some Bugzilla servers support 2.0 but all support 1.0
		Method:  "ExternalBugs.remove_external_bug",
		ID:      "identifier", // this is useful when fielding asynchronous responses, but not here
		Parameters: []RemoveExternalBugParameters{{
			APIKey: string(c.getAPIKey()),
			BugIDs: []int{id},
			ExternalBugIdentifier: ExternalBugIdentifier{
				Type: "https://github.com/",
				ID:   pullIdentifier,
			},
		}},
	}
	body, err := json.Marshal(rpcPayload)
	if err != nil {
		return false, fmt.Errorf("failed to marshal JSONRPC payload: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/jsonrpc.cgi", c.endpoint), bytes.NewBuffer(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.request(req, logger)
	if err != nil {
		return false, err
	}
	var response struct {
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error,omitempty"`
		ID     string `json:"id"`
		Result *struct {
			ExternalBugs []struct {
				Type string `json:"ext_type_url"`
				ID   string `json:"ext_bz_bug_id"`
			} `json:"external_bugs"`
		} `json:"result,omitempty"`
	}
	if err := json.Unmarshal(resp, &response); err != nil {
		return false, fmt.Errorf("failed to unmarshal JSONRPC response: %v", err)
	}
	if response.Error != nil {
		if response.Error.Code == 1006 && strings.Contains(response.Error.Message, `No external tracker bugs were found that matched your criteria`) {
			// removing the external bug failed since it is already gone, this is not an error
			return false, nil
		}
		return false, fmt.Errorf("JSONRPC error %d: %v", response.Error.Code, response.Error.Message)
	}
	if response.ID != rpcPayload.ID {
		return false, fmt.Errorf("JSONRPC returned mismatched identifier, expected %s but got %s", rpcPayload.ID, response.ID)
	}
	changed := false
	if response.Result != nil {
		for _, bug := range response.Result.ExternalBugs {
			changed = changed || bug.ID == pullIdentifier
		}
	}
	return changed, nil
}

func IdentifierForPull(org, repo string, num int) string {
	return fmt.Sprintf("%s/%s/pull/%d", org, repo, num)
}

func PullFromIdentifier(identifier string) (org, repo string, num int, err error) {
	parts := strings.Split(identifier, "/")
	if len(parts) != 4 {
		return "", "", 0, fmt.Errorf("invalid pull identifier with %d parts: %q", len(parts), identifier)
	}
	if parts[2] != "pull" {
		return "", "", 0, &identifierNotForPull{identifier: identifier}
	}
	number, err := strconv.Atoi(parts[3])
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid pull identifier: could not parse %s as number: %v", parts[3], err)
	}

	return parts[0], parts[1], number, nil
}

type identifierNotForPull struct {
	identifier string
}

func (i identifierNotForPull) Error() string {
	return fmt.Sprintf("identifier %q is not for a pull request", i.identifier)
}

func IsIdentifierNotForPullErr(err error) bool {
	_, ok := err.(*identifierNotForPull)
	return ok
}
