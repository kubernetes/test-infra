/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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

package mungers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

const (
	cpCandidateLabel = "cherrypick-candidate"
	cpApprovedLabel  = "cherrypick-approved"
)

var (
	_       = fmt.Print
	maxTime = time.Unix(1<<63-62135596801, 999999999) // http://stackoverflow.com/questions/25065055/what-is-the-maximum-time-time-in-go
)

type rawReadyInfo struct {
	Number int
	Title  string
	SHA    string
}

type cherrypickStatus struct {
	statusPullRequest
	ExtraInfo []string
}

type queueData struct {
	MergedAndApproved []cherrypickStatus
	Merged            []cherrypickStatus
	Unmerged          []cherrypickStatus
}

// CherrypickQueue will merge PR which meet a set of requirements.
type CherrypickQueue struct {
	sync.Mutex
	lastMergedAndApproved map[int]*github.MungeObject // info from the last run of the munger
	lastMerged            map[int]*github.MungeObject // info from the last run of the munger
	lastUnmerged          map[int]*github.MungeObject // info from the last run of the munger
	mergedAndApproved     map[int]*github.MungeObject // info from the current run of the munger
	merged                map[int]*github.MungeObject // info from the current run of the munger
	unmerged              map[int]*github.MungeObject // info from the current run of the munger
}

func init() {
	RegisterMungerOrDie(&CherrypickQueue{})
}

// Name is the name usable in --pr-mungers
func (c *CherrypickQueue) Name() string { return "cherrypick-queue" }

// RequiredFeatures is a slice of 'features' that must be provided
func (c *CherrypickQueue) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger
func (c *CherrypickQueue) Initialize(config *github.Config, features *features.Features) error {
	c.Lock()
	defer c.Unlock()

	if len(config.Address) > 0 {
		if len(config.WWWRoot) > 0 {
			http.Handle("/", http.FileServer(http.Dir(config.WWWRoot)))
		}
		http.HandleFunc("/queue", c.serveQueue)
		http.HandleFunc("/raw", c.serveRaw)
		http.HandleFunc("/queue-info", c.serveQueueInfo)
		config.ServeDebugStats("/stats")
		go http.ListenAndServe(config.Address, nil)
	}
	c.lastMergedAndApproved = map[int]*github.MungeObject{}
	c.lastMerged = map[int]*github.MungeObject{}
	c.lastUnmerged = map[int]*github.MungeObject{}
	c.mergedAndApproved = map[int]*github.MungeObject{}
	c.merged = map[int]*github.MungeObject{}
	c.unmerged = map[int]*github.MungeObject{}
	return nil
}

// EachLoop is called at the start of every munge loop
func (c *CherrypickQueue) EachLoop() error {
	c.Lock()
	defer c.Unlock()
	c.lastMergedAndApproved = c.mergedAndApproved
	c.lastMerged = c.merged
	c.lastUnmerged = c.unmerged
	c.mergedAndApproved = map[int]*github.MungeObject{}
	c.merged = map[int]*github.MungeObject{}
	c.unmerged = map[int]*github.MungeObject{}
	return nil
}

// AddFlags will add any request flags to the cobra `cmd`
func (c *CherrypickQueue) AddFlags(cmd *cobra.Command, config *github.Config) {}

// Munge is the workhorse the will actually make updates to the PR
func (c *CherrypickQueue) Munge(obj *github.MungeObject) {
	if !obj.HasLabel(cpCandidateLabel) {
		return
	}
	if !obj.IsPR() {
		return
	}
	// This will cache the PR and events so when we try to view the queue we don't
	// hit github while trying to load the page
	obj.GetPR()

	num := *obj.Issue.Number
	c.Lock()
	merged, _ := obj.IsMerged()
	if merged {
		if obj.HasLabel(cpApprovedLabel) {
			c.mergedAndApproved[num] = obj
		} else {
			c.merged[num] = obj
		}
	} else {
		c.unmerged[num] = obj
	}
	c.Unlock()
	return
}

func mergeTime(obj *github.MungeObject) time.Time {
	t := obj.MergedAt()
	if t == nil {
		t = &maxTime
	}
	return *t
}

type cpQueueSorter []*github.MungeObject

func (s cpQueueSorter) Len() int      { return len(s) }
func (s cpQueueSorter) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

//  PLEASE PLEASE PLEASE update serveQueueInfo() if you update this.
func (s cpQueueSorter) Less(i, j int) bool {
	a := s[i]
	b := s[j]

	// Sort first based on release milestone
	aDue := a.ReleaseMilestoneDue()
	bDue := b.ReleaseMilestoneDue()
	if aDue.Before(bDue) {
		return true
	} else if aDue.After(bDue) {
		return false
	}

	// Then sort by the order in which they were merged
	aTime := mergeTime(a)
	bTime := mergeTime(b)
	if aTime.Before(bTime) {
		return true
	} else if aTime.After(bTime) {
		return false
	}

	// Then show those which have been approved
	aApproved := a.HasLabel(cpApprovedLabel)
	bApproved := b.HasLabel(cpApprovedLabel)
	if aApproved && !bApproved {
		return true
	} else if !aApproved && bApproved {
		return false
	}

	// Sort by LGTM as humans are likely to want to approve
	// those first. After it merges the above check will win
	// and LGTM won't matter
	aLGTM := a.HasLabel(lgtmLabel)
	bLGTM := b.HasLabel(lgtmLabel)
	if aLGTM && !bLGTM {
		return true
	} else if !aLGTM && bLGTM {
		return false
	}

	// And finally by issue number, just so there is some consistency
	return *a.Issue.Number < *b.Issue.Number
}

// c.Lock() better held!!!
func (c *CherrypickQueue) orderedQueue(queue map[int]*github.MungeObject) []int {
	objs := []*github.MungeObject{}
	for _, obj := range queue {
		objs = append(objs, obj)
	}
	sort.Sort(cpQueueSorter(objs))

	var ordered []int
	for _, obj := range objs {
		ordered = append(ordered, *obj.Issue.Number)
	}
	return ordered
}

// getCurrentQueue returns the merger of the lastQueue and the currentQueue.
func (c *CherrypickQueue) getMergedQueue(last map[int]*github.MungeObject, current map[int]*github.MungeObject) map[int]*github.MungeObject {
	c.Lock()
	defer c.Unlock()

	out := map[int]*github.MungeObject{}
	for i, v := range last {
		out[i] = v
	}
	for i, v := range current {
		out[i] = v
	}
	return out
}

func (c *CherrypickQueue) serveRaw(res http.ResponseWriter, req *http.Request) {
	queue := c.getMergedQueue(c.lastMergedAndApproved, c.mergedAndApproved)
	keyOrder := c.orderedQueue(queue)
	sortedQueue := []rawReadyInfo{}
	for _, key := range keyOrder {
		obj := queue[key]
		sha := obj.MergeCommit()
		if sha == nil {
			empty := "UnknownSHA"
			sha = &empty
		}
		rri := rawReadyInfo{
			Number: *obj.Issue.Number,
			Title:  *obj.Issue.Title,
			SHA:    *sha,
		}
		sortedQueue = append(sortedQueue, rri)
	}
	data, err := json.Marshal(sortedQueue)
	if err != nil {
		res.Header().Set("Content-type", "text/plain")
		res.WriteHeader(http.StatusInternalServerError)
		glog.Errorf("Unable to Marshal Status: %v: %v", data, err)
		return
	}
	res.Header().Set("Content-type", "application/json")
	res.WriteHeader(http.StatusOK)
	res.Write(data)
}

func (c *CherrypickQueue) getQueueData(last, current map[int]*github.MungeObject) []cherrypickStatus {
	out := []cherrypickStatus{}
	queue := c.getMergedQueue(last, current)
	keyOrder := c.orderedQueue(queue)
	for _, key := range keyOrder {
		obj := queue[key]
		cps := cherrypickStatus{
			statusPullRequest: *objToStatusPullRequest(obj),
		}
		if obj.HasLabel(cpApprovedLabel) {
			cps.ExtraInfo = append(cps.ExtraInfo, cpApprovedLabel)
		}
		milestone := obj.ReleaseMilestone()
		if milestone != "" {
			cps.ExtraInfo = append(cps.ExtraInfo, milestone)
		}
		merged, _ := obj.IsMerged()
		if !merged && obj.HasLabel(lgtmLabel) {
			// Don't bother showing LGTM for merged things
			// it's just a distraction at that point
			cps.ExtraInfo = append(cps.ExtraInfo, lgtmLabel)
		}
		out = append(out, cps)
	}

	return out
}

func (c *CherrypickQueue) serveQueue(res http.ResponseWriter, req *http.Request) {
	outData := queueData{}

	outData.MergedAndApproved = c.getQueueData(c.lastMergedAndApproved, c.mergedAndApproved)
	outData.Merged = c.getQueueData(c.lastMerged, c.merged)
	outData.Unmerged = c.getQueueData(c.lastUnmerged, c.unmerged)

	data, err := json.Marshal(outData)
	if err != nil {
		res.Header().Set("Content-type", "text/plain")
		res.WriteHeader(http.StatusInternalServerError)
		glog.Errorf("Unable to Marshal Status: %v: %v", data, err)
		return
	}
	res.Header().Set("Content-type", "application/json")
	res.WriteHeader(http.StatusOK)
	res.Write(data)
}

func (c *CherrypickQueue) serveQueueInfo(res http.ResponseWriter, req *http.Request) {
	res.Header().Set("Content-type", "text/plain")
	res.WriteHeader(http.StatusOK)
	res.Write([]byte(`The cherrypick queue is sorted by the following. If there is a tie in any test the next test will be used.
<ol>
  <li>Milestone Due Date
    <ul>
      <li>Release milestones must be of the form vX.Y</li>
      <li>PRs without a milestone are considered after PRs with a milestone</li>
    </ul>
  </li>
  <li>Labeld with "` + cpApprovedLabel + `"
    <ul>
      <li>PRs with the "` + cpApprovedLabel + `" label come before those without</li>
    </ul>
  </li>
  <li>Merge Time
    <ul>
      <li>The earlier a PR was merged the earlier it is in the list</li>
      <li>PRs which have not merged are considered 'after' any merged PR</li>
    </ul>
  </li>
  <li>Labeld with "` + lgtmLabel + `"
    <ul>
      <li>PRs with the "` + lgtmLabel + `" label come before those without</li>
    </ul>
  <li>PR number</li>
</ol> `))
}
