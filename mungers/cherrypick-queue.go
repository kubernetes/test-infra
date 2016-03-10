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

type cherrypickStatus struct {
	statusPullRequest
	ExtraInfo []string
}

// CherrypickQueue will merge PR which meet a set of requirements.
type CherrypickQueue struct {
	sync.Mutex
	lastQueue map[int]*github.MungeObject // info from the last run of the munger
	queue     map[int]*github.MungeObject // info from the current run of the munger
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
		http.HandleFunc("/queue-info", c.serveQueueInfo)
		config.ServeDebugStats("/stats")
		go http.ListenAndServe(config.Address, nil)
	}
	c.lastQueue = map[int]*github.MungeObject{}
	c.queue = map[int]*github.MungeObject{}
	return nil
}

// EachLoop is called at the start of every munge loop
func (c *CherrypickQueue) EachLoop() error {
	c.Lock()
	defer c.Unlock()
	c.lastQueue = c.queue
	c.queue = map[int]*github.MungeObject{}
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
	// This will cache the PR so when we try to view the queue we don't pull
	// the PR info while the user is waiting on the page
	obj.GetPR()

	c.Lock()
	c.queue[*obj.Issue.Number] = obj
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

	// Then show those which have been approved
	aApproved := a.HasLabel(cpApprovedLabel)
	bApproved := b.HasLabel(cpApprovedLabel)
	if aApproved && !bApproved {
		return true
	} else if !aApproved && bApproved {
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

	// Sort by LGTM as humans are likely to want to approve
	// those first. After it merges the above check will win
	// and LGTM won't matter
	aLGTM := a.HasLabel("lgtm")
	bLGTM := b.HasLabel("lgtm")
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
func (c *CherrypickQueue) getCurrentQueue() map[int]*github.MungeObject {
	c.Lock()
	defer c.Unlock()

	queue := map[int]*github.MungeObject{}
	for i, v := range c.lastQueue {
		queue[i] = v
	}
	for i, v := range c.queue {
		queue[i] = v
	}
	return queue
}

func (c *CherrypickQueue) serveQueue(res http.ResponseWriter, req *http.Request) {
	queue := c.getCurrentQueue()
	keyOrder := c.orderedQueue(queue)
	sortedQueue := []cherrypickStatus{}
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
		if merged {
			cps.ExtraInfo = append(cps.ExtraInfo, "Merged")
		}
		if !merged && obj.HasLabel("lgtm") {
			// Don't bother showing LGTM for merged things
			// it's just a distraction at that point
			cps.ExtraInfo = append(cps.ExtraInfo, "lgtm")
		}
		sortedQueue = append(sortedQueue, cps)
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
  <li>Labeld with "lgtm"
    <ul>
      <li>PRs with the "lgtm" label come before those without</li>
    </ul>
  <li>PR number</li>
</ol> `))
}
