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
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/tide"
	"k8s.io/test-infra/prow/tide/history"
)

type tidePools struct {
	Queries     []string
	TideQueries []config.TideQuery
	Pools       []tide.PoolForDeck
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

	tenantIDs sets.String
	cfg       func() *config.Config

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
	pools = ta.filterPools(pools)

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
	history = ta.filterHistory(history)

	ta.Lock()
	defer ta.Unlock()
	ta.history = history
	return nil
}

func (ta *tideAgent) matchingIDs(ids []string) bool {
	return len(ids) > 0 && ta.tenantIDs.HasAll(ids...)
}

func (ta *tideAgent) filterPools(pools []tide.Pool) []tide.Pool {
	filtered := make([]tide.Pool, 0, len(pools))
	for _, pool := range pools {
		// curIDs are the IDs associated with all PJs in the Pool
		// We want to add the ID associated with the OrgRepo for extra protection
		curIDs := sets.NewString(pool.TenantIDs...)
		orgRepoID := ta.cfg().GetProwJobDefault(pool.Org+"/"+pool.Repo, "*").TenantID
		needsHide := matches(pool.Org+"/"+pool.Repo, ta.hiddenRepos())
		if match := ta.filter(orgRepoID, curIDs, needsHide); match {
			filtered = append(filtered, pool)
		}
	}
	return filtered
}

func noTenantIDOrDefaultTenantID(ids []string) bool {
	for _, id := range ids {
		if id != "" && id != config.DefaultTenantID {
			return false
		}
	}
	return true
}

func recordIDs(records []history.Record) sets.String {
	res := sets.String{}
	for _, record := range records {
		res.Insert(record.TenantIDs...)
	}
	return res
}

func (ta *tideAgent) filterHistory(hist map[string][]history.Record) map[string][]history.Record {
	filtered := make(map[string][]history.Record, len(hist))
	for pool, records := range hist {
		orgRepo := strings.Split(pool, ":")[0]
		curIDs := recordIDs(records).Insert()
		orgRepoID := ta.cfg().GetProwJobDefault(orgRepo, "*").TenantID
		needsHide := matches(orgRepo, ta.hiddenRepos())
		if match := ta.filter(orgRepoID, curIDs, needsHide); match {
			filtered[pool] = records
		}
	}
	return filtered
}

func (ta *tideAgent) filter(orgRepoID string, curIDs sets.String, needsHide bool) bool {
	// If the orgrepo is associated with no tenantID OR the default tenantID we ignore it here.
	// This prevents already IDd History from getting the default ID assigned to them when their orgrepo is not associated with an OrgRepo.
	// History with no tenantID and with default tenantID behave the same, so adding the default ID just causes issues
	if orgRepoID != "" && orgRepoID != config.DefaultTenantID {
		curIDs.Insert(orgRepoID)
	}
	if len(ta.tenantIDs) > 0 {
		if ta.matchingIDs(curIDs.List()) {
			// Deck has tenantIDs and they match with the History
			return true
		}
	} else if needsHide {
		if ta.showHidden || ta.hiddenOnly {
			return true
		}
	} else if !ta.hiddenOnly && noTenantIDOrDefaultTenantID(curIDs.List()) {
		return true
	}
	return false

}

func (ta *tideAgent) filterQueries(queries []config.TideQuery) []config.TideQuery {
	filtered := make([]config.TideQuery, 0, len(queries))
	for _, qc := range queries {
		curIDs := qc.TenantIDs(*ta.cfg())
		var exposedRepos []string
		for _, repo := range qc.Repos {
			if !matches(repo, ta.hiddenRepos()) {
				exposedRepos = append(exposedRepos, repo)
			}
		}
		orgRepoID := ""
		if len(exposedRepos) < len(qc.Repos) { // If there are hidden repos
			if match := ta.filter(orgRepoID, sets.NewString(curIDs...), true); match {
				filtered = append(filtered, qc)
				continue
			} else { // If the hidden repos result in this query being filtered out, then we should remove them and try the rest
				qc.Repos = exposedRepos
			}
		}
		if len(qc.Repos) > 0 || len(qc.Orgs) > 0 {
			if match := ta.filter(orgRepoID, sets.NewString(curIDs...), false); match {
				filtered = append(filtered, qc)
			}
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
