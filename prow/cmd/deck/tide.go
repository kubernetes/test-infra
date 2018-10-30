/*
Copyright 2017 The Kubernetes Authors.

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
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/tide"
)

type tideData struct {
	Queries     []string
	TideQueries []config.TideQuery
	Pools       []tide.Pool
}

type tideAgent struct {
	log          *logrus.Entry
	path         string
	updatePeriod func() time.Duration

	// Config for hiding repos
	hiddenRepos []string
	hiddenOnly  bool

	sync.Mutex
	pools []tide.Pool
}

func (ta *tideAgent) start() {
	startTime := time.Now()
	if err := ta.update(); err != nil {
		ta.log.WithError(err).Error("Updating pool for the first time.")
	}
	go func() {
		for {
			time.Sleep(time.Until(startTime.Add(ta.updatePeriod())))
			startTime = time.Now()
			if err := ta.update(); err != nil {
				ta.log.WithError(err).Error("Updating pool.")
			}
		}
	}()
}

func (ta *tideAgent) update() error {
	var pools []tide.Pool
	var resp *http.Response
	var err error
	for i := 0; i < 3; i++ {
		if err != nil {
			ta.log.WithError(err).Warning("Tide request failed. Retrying.")
			time.Sleep(5 * time.Second)
		}
		resp, err = http.Get(ta.path)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode > 299 {
				err = fmt.Errorf("response has status code %d", resp.StatusCode)
				continue
			}
			if err := json.NewDecoder(resp.Body).Decode(&pools); err != nil {
				return err
			}
			break
		}
	}
	if err != nil {
		return err
	}
	ta.Lock()
	defer ta.Unlock()
	ta.pools = pools
	return nil
}

func (ta *tideAgent) filterHidden(tideQueries []config.TideQuery, pools []tide.Pool) ([]config.TideQuery, []tide.Pool) {
	if len(ta.hiddenRepos) == 0 {
		return tideQueries, pools
	}

	filteredTideQueries := make([]config.TideQuery, 0, len(tideQueries))
	for _, qc := range tideQueries {
		includesHidden := false
		// This will exclude the query even if a single
		// repo in the query is included in hiddenRepos.
		for _, repo := range qc.Repos {
			if matches(repo, ta.hiddenRepos) {
				includesHidden = true
				break
			}
		}
		if (includesHidden && ta.hiddenOnly) ||
			(!includesHidden && !ta.hiddenOnly) {
			filteredTideQueries = append(filteredTideQueries, qc)
		} else {
			ta.log.Debugf("Ignoring query: %v", qc.Query())
		}
	}

	filteredPools := make([]tide.Pool, 0, len(pools))
	for _, pool := range pools {
		needsHide := matches(pool.Org+"/"+pool.Repo, ta.hiddenRepos)
		if (needsHide && ta.hiddenOnly) ||
			(!needsHide && !ta.hiddenOnly) {
			filteredPools = append(filteredPools, pool)
		} else {
			ta.log.Debugf("Ignoring pool for %s", pool.Org+"/"+pool.Repo)
		}
	}

	return filteredTideQueries, filteredPools
}

// matches returns whether the provided repo intersects
// with repos. repo has always the "org/repo" format but
// repos can include both orgs and repos.
func matches(repo string, repos []string) bool {
	org := strings.Split(repo, "/")[0]
	for _, r := range repos {
		if r == repo || r == org {
			return true
		}
	}
	return false
}
