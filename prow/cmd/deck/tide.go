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

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/tide"
	"k8s.io/test-infra/prow/tide/history"
)

type tidePools struct {
	Queries     []string
	TideQueries []config.TideQuery
	Pools       []tide.Pool
}

type tideHistory struct {
	History map[string][]history.Record
}

type tideAgent struct {
	log          *logrus.Entry
	path         string
	updatePeriod func() time.Duration

	// Config for hiding repos
	hiddenRepos func() []string
	hiddenOnly  bool
	showHidden  bool

	sync.Mutex
	pools   []tide.Pool
	history map[string][]history.Record
}

func (ta *tideAgent) start() {
	startTimePool := time.Now()
	if err := ta.updatePools(); err != nil {
		ta.log.WithError(err).Error("Updating pools the first time.")
	}
	startTimeHistory := time.Now()
	if err := ta.updateHistory(); err != nil {
		ta.log.WithError(err).Error("Updating history the first time.")
	}

	go func() {
		for {
			time.Sleep(time.Until(startTimePool.Add(ta.updatePeriod())))
			startTimePool = time.Now()
			if err := ta.updatePools(); err != nil {
				ta.log.WithError(err).Error("Updating pools.")
			}
		}
	}()
	go func() {
		for {
			time.Sleep(time.Until(startTimeHistory.Add(ta.updatePeriod())))
			startTimeHistory = time.Now()
			if err := ta.updateHistory(); err != nil {
				ta.log.WithError(err).Error("Updating history.")
			}
		}
	}()
}

func fetchTideData(log *logrus.Entry, path string, data interface{}) error {
	var prevErrs []error
	var err error
	backoff := 5 * time.Second
	for i := 0; i < 4; i++ {
		var resp *http.Response
		if err != nil {
			prevErrs = append(prevErrs, err)
			time.Sleep(backoff)
			backoff *= 4
		}
		resp, err = http.Get(path)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode > 299 {
				err = fmt.Errorf("response has status code %d", resp.StatusCode)
				continue
			}
			if err = json.NewDecoder(resp.Body).Decode(data); err != nil {
				break
			}
			break
		}
	}

	// Either combine previous errors with the returned error, or if we succeeded
	// log once about any errors we saw before succeeding.
	prevErr := utilerrors.NewAggregate(prevErrs)
	if err != nil {
		return utilerrors.NewAggregate([]error{err, prevErr})
	}
	if prevErr != nil {
		log.WithError(prevErr).Infof(
			"Failed %d retries fetching Tide data before success: %v.",
			len(prevErrs),
			prevErr,
		)
	}
	return nil
}

func (ta *tideAgent) updatePools() error {
	var pools []tide.Pool
	if err := fetchTideData(ta.log, ta.path, &pools); err != nil {
		return err
	}
	pools = ta.filterHiddenPools(pools)

	ta.Lock()
	defer ta.Unlock()
	ta.pools = pools
	return nil
}

func (ta *tideAgent) updateHistory() error {
	path := strings.TrimSuffix(ta.path, "/") + "/history"
	var history map[string][]history.Record
	if err := fetchTideData(ta.log, path, &history); err != nil {
		return err
	}
	history = ta.filterHiddenHistory(history)

	ta.Lock()
	defer ta.Unlock()
	ta.history = history
	return nil
}

func (ta *tideAgent) filterHiddenPools(pools []tide.Pool) []tide.Pool {
	if len(ta.hiddenRepos()) == 0 {
		return pools
	}

	filtered := make([]tide.Pool, 0, len(pools))
	for _, pool := range pools {
		needsHide := matches(pool.Org+"/"+pool.Repo, ta.hiddenRepos())
		var ignored []string
		if needsHide && ta.showHidden {
			filtered = append(filtered, pool)
		} else if needsHide == ta.hiddenOnly {
			filtered = append(filtered, pool)
		} else {
			ignored = append(ignored, pool.Org+"/"+pool.Repo)
		}
	}
	return filtered
}

func (ta *tideAgent) filterHiddenHistory(hist map[string][]history.Record) map[string][]history.Record {
	if len(ta.hiddenRepos()) == 0 {
		return hist
	}

	filtered := make(map[string][]history.Record, len(hist))
	for pool, records := range hist {
		needsHide := matches(strings.Split(pool, ":")[0], ta.hiddenRepos())
		if needsHide && ta.showHidden {
			filtered[pool] = records
		} else if needsHide == ta.hiddenOnly {
			filtered[pool] = records
		}
	}
	return filtered
}

func (ta *tideAgent) filterHiddenQueries(queries []config.TideQuery) []config.TideQuery {
	if len(ta.hiddenRepos()) == 0 {
		return queries
	}

	filtered := make([]config.TideQuery, 0, len(queries))
	for _, qc := range queries {
		includesHidden := false
		// This will exclude the query even if a single
		// repo in the query is included in hiddenRepos.
		for _, repo := range qc.Repos {
			if matches(repo, ta.hiddenRepos()) {
				includesHidden = true
				break
			}
		}
		if includesHidden && ta.showHidden {
			filtered = append(filtered, qc)
		} else if includesHidden == ta.hiddenOnly {
			filtered = append(filtered, qc)
		}
	}
	return filtered
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
