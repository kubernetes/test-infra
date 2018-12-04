/*
Copyright 2018 The Kubernetes Authors.

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

package statusreconciler

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/maintenance/migratestatus/migrator"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/errorutil"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/trigger"
)

// NewController constructs a new controller to reconcile stauses on config change
func NewController(continueOnError bool, kubeClient *kube.Client, githubClient *github.Client, pluginAgent *plugins.ConfigAgent) *Controller {
	return &Controller{
		continueOnError: continueOnError,
		kubeClient:      kubeClient,
		githubClient:    githubClient,
		statusMigrator: &gitHubMigrator{
			githubClient:    githubClient,
			continueOnError: continueOnError,
		},
		trustedChecker: &githubTrustedChecker{
			githubClient: githubClient,
			pluginAgent:  pluginAgent,
		},
	}
}

type statusMigrator interface {
	retire(org, repo, context string) error
	migrate(org, repo, from, to string) error
}

type gitHubMigrator struct {
	githubClient    *github.Client
	continueOnError bool
}

func (m *gitHubMigrator) retire(org, repo, context string) error {
	return migrator.New(
		*migrator.RetireMode(context, "", ""),
		m.githubClient, org, repo, m.continueOnError,
	).Migrate()
}

func (m *gitHubMigrator) migrate(org, repo, from, to string) error {
	return migrator.New(
		*migrator.MoveMode(from, to, ""),
		m.githubClient, org, repo, m.continueOnError,
	).Migrate()
}

type kubeClient interface {
	CreateProwJob(j kube.ProwJob) (kube.ProwJob, error)
}

type githubClient interface {
	GetPullRequests(org, repo string) ([]github.PullRequest, error)
	GetRef(org, repo, ref string) (string, error)
}

type trustedChecker interface {
	trustedPullRequest(author, org, repo string, num int) (bool, error)
}

type githubTrustedChecker struct {
	githubClient *github.Client
	pluginAgent  *plugins.ConfigAgent
}

func (c *githubTrustedChecker) trustedPullRequest(author, org, repo string, num int) (bool, error) {
	_, trusted, err := trigger.TrustedPullRequest(
		c.githubClient,
		c.pluginAgent.Config().TriggerFor(org, repo),
		author, org, repo, num, nil,
	)
	return trusted, err
}

// Controller reconciles statuses on PRs when config changes impact blocking presubmits
type Controller struct {
	continueOnError bool
	kubeClient      kubeClient
	githubClient    githubClient
	statusMigrator  statusMigrator
	trustedChecker  trustedChecker
}

// Run monitors the incoming configuration changes to determine when statuses need to be
// reconciled on PRs in flight when blocking presubmits change
func (c *Controller) Run(stop <-chan os.Signal, changes <-chan config.ConfigDelta) {
	for {
		select {
		case change := <-changes:
			start := time.Now()
			if err := c.reconcile(change); err != nil {
				logrus.WithError(err).Error("Error reconciling statuses.")
			}
			logrus.WithField("duration", fmt.Sprintf("%v", time.Since(start))).Info("Statuses reconciled")
		case <-stop:
			logrus.Info("status-reconciler is shutting down...")
			return
		}
	}
}

func (c *Controller) reconcile(delta config.ConfigDelta) error {
	var errors []error
	if err := c.triggerNewPresubmits(addedBlockingPresubmits(delta.Before.Presubmits, delta.After.Presubmits)); err != nil {
		errors = append(errors, err)
		if !c.continueOnError {
			return errorutil.NewAggregate(errors...)
		}
	}

	if err := c.retireRemovedContexts(removedBlockingPresubmits(delta.Before.Presubmits, delta.After.Presubmits)); err != nil {
		errors = append(errors, err)
		if !c.continueOnError {
			return errorutil.NewAggregate(errors...)
		}
	}

	if err := c.updateMigratedContexts(migratedBlockingPresubmits(delta.Before.Presubmits, delta.After.Presubmits)); err != nil {
		errors = append(errors, err)
		if !c.continueOnError {
			return errorutil.NewAggregate(errors...)
		}
	}

	return errorutil.NewAggregate(errors...)
}

func (c *Controller) triggerNewPresubmits(addedPresubmits map[string][]config.Presubmit) error {
	var triggerErrors []error
	for orgrepo, presubmits := range addedPresubmits {
		if len(presubmits) == 0 {
			continue
		}
		parts := strings.SplitN(orgrepo, "/", 2)
		org, repo := parts[0], parts[1]
		prs, err := c.githubClient.GetPullRequests(org, repo)
		if err != nil {
			triggerErrors = append(triggerErrors, fmt.Errorf("failed to list pull requests for %s: %v", orgrepo, err))
			if !c.continueOnError {
				return errorutil.NewAggregate(triggerErrors...)
			}
			continue
		}
		for _, pr := range prs {
			if err := c.triggerIfTrusted(org, repo, pr, presubmits); err != nil {
				triggerErrors = append(triggerErrors, fmt.Errorf("failed to trigger jobs for %s#%d: %v", orgrepo, pr.Number, err))
				if !c.continueOnError {
					return errorutil.NewAggregate(triggerErrors...)
				}
				continue
			}
		}
	}
	return errorutil.NewAggregate(triggerErrors...)
}

func (c *Controller) triggerIfTrusted(org, repo string, pr github.PullRequest, presubmits []config.Presubmit) error {
	trusted, err := c.trustedChecker.trustedPullRequest(pr.User.Login, org, repo, pr.Number)
	if err != nil {
		return fmt.Errorf("failed to determine if %s/%s#%d is trusted: %v", org, repo, pr.Number, err)
	}
	if !trusted {
		return nil
	}
	baseSHA, err := c.githubClient.GetRef(org, repo, "heads/"+pr.Base.Ref)
	if err != nil {
		return fmt.Errorf("failed to determine base SHA for %s/%s#%d: %v", org, repo, pr.Number, err)
	}
	var triggerErrors []error
	for _, presubmit := range presubmits {
		pj := pjutil.NewPresubmit(pr, baseSHA, presubmit, "none")
		logrus.WithFields(pjutil.ProwJobFields(&pj)).Info("Triggering new ProwJob to create newly-required context.")
		if _, err := c.kubeClient.CreateProwJob(pj); err != nil {
			triggerErrors = append(triggerErrors, err)
			if !c.continueOnError {
				break
			}
		}
	}
	return errorutil.NewAggregate(triggerErrors...)
}

func (c *Controller) retireRemovedContexts(retiredPresubmits map[string][]config.Presubmit) error {
	var retireErrors []error
	for orgrepo, presubmits := range retiredPresubmits {
		parts := strings.SplitN(orgrepo, "/", 2)
		org, repo := parts[0], parts[1]
		for _, presubmit := range presubmits {
			logrus.WithFields(logrus.Fields{
				"org":     org,
				"repo":    repo,
				"context": presubmit.Context,
			}).Info("Retiring context.")
			if err := c.statusMigrator.retire(org, repo, presubmit.Context); err != nil {
				if c.continueOnError {
					retireErrors = append(retireErrors, err)
					continue
				}
				return err
			}
		}
	}
	return errorutil.NewAggregate(retireErrors...)
}

func (c *Controller) updateMigratedContexts(migrations map[string][]presubmitMigration) error {
	var migrateErrors []error
	for orgrepo, migrations := range migrations {
		parts := strings.SplitN(orgrepo, "/", 2)
		org, repo := parts[0], parts[1]
		for _, migration := range migrations {
			logrus.WithFields(logrus.Fields{
				"org":  org,
				"repo": repo,
				"from": migration.from.Context,
				"to":   migration.to.Context,
			}).Info("Migrating context.")
			if err := c.statusMigrator.migrate(org, repo, migration.from.Context, migration.to.Context); err != nil {
				if c.continueOnError {
					migrateErrors = append(migrateErrors, err)
					continue
				}
				return err
			}
		}
	}
	return errorutil.NewAggregate(migrateErrors...)
}

// addedBlockingPresubmits determines new blocking presubmits based on a
// config update. New blocking presubmits are either brand-new presubmits
// or extant presubmits that are now reporting. Previous presubmits that
// reported but were optional that are no longer optional require no action
// as their contexts will already exist on PRs.
func addedBlockingPresubmits(old, new map[string][]config.Presubmit) map[string][]config.Presubmit {
	added := map[string][]config.Presubmit{}

	for repo, oldPresubmits := range old {
		added[repo] = []config.Presubmit{}
		for _, newPresubmit := range new[repo] {
			if !newPresubmit.ContextRequired() {
				continue
			}
			var found bool
			for _, oldPresubmit := range oldPresubmits {
				if oldPresubmit.Name == newPresubmit.Name {
					if oldPresubmit.SkipReport && !newPresubmit.SkipReport {
						added[repo] = append(added[repo], newPresubmit)
						logrus.WithFields(logrus.Fields{
							"repo": repo,
							"name": oldPresubmit.Name,
						}).Debug("Identified a newly-reporting blocking presubmit.")
					}
					found = true
					break
				}
			}
			if !found {
				added[repo] = append(added[repo], newPresubmit)
				logrus.WithFields(logrus.Fields{
					"repo": repo,
					"name": newPresubmit.Name,
				}).Debug("Identified an added blocking presubmit.")
			}
		}
	}

	logrus.Infof("Identified %d added blocking presubmits.", len(added))
	return added
}

// removedBlockingPresubmits determines stale blocking presubmits based on a
// config update. Presubmits that are no longer blocking due to no longer
// reporting or being optional require no action as Tide will honor those
// statuses correctly.
func removedBlockingPresubmits(old, new map[string][]config.Presubmit) map[string][]config.Presubmit {
	removed := map[string][]config.Presubmit{}

	for repo, oldPresubmits := range old {
		removed[repo] = []config.Presubmit{}
		for _, oldPresubmit := range oldPresubmits {
			if !oldPresubmit.ContextRequired() {
				continue
			}
			var found bool
			for _, newPresubmit := range new[repo] {
				if oldPresubmit.Name == newPresubmit.Name {
					found = true
					break
				}
			}
			if !found {
				removed[repo] = append(removed[repo], oldPresubmit)
				logrus.WithFields(logrus.Fields{
					"repo": repo,
					"name": oldPresubmit.Name,
				}).Debug("Identified a removed blocking presubmit.")
			}
		}
	}

	logrus.Infof("Identified %d removed blocking presubmits.", len(removed))
	return removed
}

type presubmitMigration struct {
	from, to config.Presubmit
}

// migratedBlockingPresubmits determines blocking presubmits that have had
// their status contexts migrated. This is a best-effort evaluation as we
// can only track a presubmit between configuration versions by its name.
// A presubmit "migration" that had its underlying job and context changed
// will be treated as a deletion and creation.
func migratedBlockingPresubmits(old, new map[string][]config.Presubmit) map[string][]presubmitMigration {
	migrated := map[string][]presubmitMigration{}

	for repo, oldPresubmits := range old {
		migrated[repo] = []presubmitMigration{}
		for _, newPresubmit := range new[repo] {
			if !newPresubmit.ContextRequired() {
				continue
			}
			for _, oldPresubmit := range oldPresubmits {
				if oldPresubmit.Context != newPresubmit.Context && oldPresubmit.Name == newPresubmit.Name {
					migrated[repo] = append(migrated[repo], presubmitMigration{from: oldPresubmit, to: newPresubmit})
					logrus.WithFields(logrus.Fields{
						"repo": repo,
						"name": oldPresubmit.Name,
						"from": oldPresubmit.Context,
						"to":   newPresubmit.Context,
					}).Debug("Identified a migrated blocking presubmit.")
				}
			}
		}
	}

	logrus.Infof("Identified %d migrated blocking presubmits.", len(migrated))
	return migrated
}
