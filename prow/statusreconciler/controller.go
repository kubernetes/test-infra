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
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	prowv1 "k8s.io/test-infra/prow/client/clientset/versioned/typed/prowjobs/v1"
	"k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/pjutil"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/test-infra/maintenance/migratestatus/migrator"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/trigger"
)

// NewController constructs a new controller to reconcile stauses on config change
func NewController(continueOnError bool, addedPresubmitDenylist sets.String, addedPresubmitDenylistAll sets.String, opener io.Opener, configPath, jobConfigPath string, supplementalProwConfigDirs []string, statusURI string, prowJobClient prowv1.ProwJobInterface, githubClient github.Client, pluginAgent *plugins.ConfigAgent) *Controller {
	sc := &statusController{
		logger:                     logrus.WithField("client", "statusController"),
		opener:                     opener,
		statusURI:                  statusURI,
		configPath:                 configPath,
		jobConfigPath:              jobConfigPath,
		supplementalProwConfigDirs: supplementalProwConfigDirs,
	}

	return &Controller{
		continueOnError:           continueOnError,
		addedPresubmitDenylist:    addedPresubmitDenylist,
		addedPresubmitDenylistAll: addedPresubmitDenylistAll,
		prowJobTriggerer: &kubeProwJobTriggerer{
			prowJobClient: prowJobClient,
			githubClient:  githubClient,
			configGetter:  sc.Config,
			pluginAgent:   pluginAgent,
		},
		githubClient: githubClient,
		statusMigrator: &gitHubMigrator{
			githubClient:    githubClient,
			continueOnError: continueOnError,
		},
		trustedChecker: &githubTrustedChecker{
			githubClient: githubClient,
			pluginAgent:  pluginAgent,
		},
		statusClient: sc,
	}
}

type statusMigrator interface {
	retire(org, repo, context string, targetBranchFilter func(string) bool) error
	migrate(org, repo, from, to string, targetBranchFilter func(string) bool) error
}

type gitHubMigrator struct {
	githubClient    github.Client
	continueOnError bool
}

func (m *gitHubMigrator) retire(org, repo, context string, targetBranchFilter func(string) bool) error {
	return migrator.New(
		*migrator.RetireMode(context, "", ""),
		m.githubClient, org, repo, targetBranchFilter, m.continueOnError,
	).Migrate()
}

func (m *gitHubMigrator) migrate(org, repo, from, to string, targetBranchFilter func(string) bool) error {
	return migrator.New(
		*migrator.MoveMode(from, to, ""),
		m.githubClient, org, repo, targetBranchFilter, m.continueOnError,
	).Migrate()
}

type prowJobTriggerer interface {
	runAndSkip(pr *github.PullRequest, requestedJobs []config.Presubmit) error
}

type kubeProwJobTriggerer struct {
	prowJobClient prowv1.ProwJobInterface
	githubClient  github.Client
	configGetter  config.Getter
	pluginAgent   *plugins.ConfigAgent
}

func (t *kubeProwJobTriggerer) runAndSkip(pr *github.PullRequest, requestedJobs []config.Presubmit) error {
	org, repo := pr.Base.Repo.Owner.Login, pr.Base.Repo.Name
	baseSHA, err := t.githubClient.GetRef(org, repo, "heads/"+pr.Base.Ref)
	if err != nil {
		return fmt.Errorf("failed to get baseSHA: %v", err)
	}
	return trigger.RunRequested(
		trigger.Client{
			GitHubClient:  t.githubClient,
			ProwJobClient: t.prowJobClient,
			Config:        t.configGetter(),
			Logger:        logrus.WithField("client", "trigger"),
		},
		pr, baseSHA, requestedJobs, "none",
	)
}

type githubClient interface {
	GetPullRequests(org, repo string) ([]github.PullRequest, error)
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
}

type trustedChecker interface {
	trustedPullRequest(author, org, repo string, num int) (bool, error)
}

type githubTrustedChecker struct {
	githubClient github.Client
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
	continueOnError           bool
	addedPresubmitDenylist    sets.String
	addedPresubmitDenylistAll sets.String
	prowJobTriggerer          prowJobTriggerer
	githubClient              githubClient
	statusMigrator            statusMigrator
	trustedChecker            trustedChecker
	statusClient              statusClient
}

// Run monitors the incoming configuration changes to determine when statuses need to be
// reconciled on PRs in flight when blocking presubmits change
func (c *Controller) Run(ctx context.Context) {
	changes, err := c.statusClient.Load()
	if err != nil {
		logrus.WithError(err).Error("Error loading saved status.")
		return
	}

	for {
		select {
		case change := <-changes:
			start := time.Now()
			log := logrus.WithField("old_config_revision", change.Before.ConfigVersionSHA).WithField("config_revision", change.After.ConfigVersionSHA)
			if err := c.reconcile(change, log); err != nil {
				log.WithError(err).Error("Error reconciling statuses.")
			}
			log.WithField("duration", fmt.Sprintf("%v", time.Since(start))).Info("Statuses reconciled")
			c.statusClient.Save()
		case <-ctx.Done():
			logrus.Info("status-reconciler is shutting down...")
			return
		}
	}
}

func (c *Controller) reconcile(delta config.Delta, log *logrus.Entry) error {
	var errors []error
	if err := c.triggerNewPresubmits(addedBlockingPresubmits(delta.Before.PresubmitsStatic, delta.After.PresubmitsStatic, log)); err != nil {
		errors = append(errors, err)
		if !c.continueOnError {
			return utilerrors.NewAggregate(errors)
		}
	}

	if err := c.retireRemovedContexts(removedPresubmits(delta.Before.PresubmitsStatic, delta.After.PresubmitsStatic, log)); err != nil {
		errors = append(errors, err)
		if !c.continueOnError {
			return utilerrors.NewAggregate(errors)
		}
	}

	if err := c.updateMigratedContexts(migratedBlockingPresubmits(delta.Before.PresubmitsStatic, delta.After.PresubmitsStatic, log)); err != nil {
		errors = append(errors, err)
		if !c.continueOnError {
			return utilerrors.NewAggregate(errors)
		}
	}

	return utilerrors.NewAggregate(errors)
}

func (c *Controller) triggerNewPresubmits(addedPresubmits map[string][]config.Presubmit, log *logrus.Entry) error {
	var triggerErrors []error
	for orgrepo, presubmits := range addedPresubmits {
		if len(presubmits) == 0 {
			continue
		}
		parts := strings.SplitN(orgrepo, "/", 2)
		if n := len(parts); n != 2 {
			triggerErrors = append(triggerErrors, fmt.Errorf("string %q can not be interpreted as org/repo", orgrepo))
			continue
		}

		org, repo := parts[0], parts[1]
		if c.addedPresubmitDenylist.Has(org) || c.addedPresubmitDenylist.Has(orgrepo) ||
			c.addedPresubmitDenylistAll.Has(org) || c.addedPresubmitDenylistAll.Has(orgrepo) {
			continue
		}
		prs, err := c.githubClient.GetPullRequests(org, repo)
		if err != nil {
			triggerErrors = append(triggerErrors, fmt.Errorf("failed to list pull requests for %s: %v", orgrepo, err))
			if !c.continueOnError {
				return utilerrors.NewAggregate(triggerErrors)
			}
			continue
		}
		for _, pr := range prs {
			if pr.Mergable != nil && !*pr.Mergable {
				// the PR cannot be merged as it is, so the user will need to update the PR (and trigger
				// testing via the PR push event) or re-test if the HEAD of the branch they are targeting
				// changes (and re-trigger tests anyway) so we do not need to do anything in this case and
				// launching jobs that instantly fail due to merge conflicts is a waste of time
				continue
			}
			// we want to appropriately trigger and skip from the set of identified presubmits that were
			// added. we know all of the presubmits we are filtering need to be forced to run, so we can
			// enforce that with a custom filter
			filter := func(p config.Presubmit) (shouldRun bool, forcedToRun bool, defaultBehavior bool) {
				return true, false, true
			}
			org, repo, number, branch := pr.Base.Repo.Owner.Login, pr.Base.Repo.Name, pr.Number, pr.Base.Ref
			changes := config.NewGitHubDeferredChangedFilesProvider(c.githubClient, org, repo, number)
			logger := log.WithFields(logrus.Fields{"org": org, "repo": repo, "number": number, "branch": branch})
			toTrigger, err := pjutil.FilterPresubmits(filter, changes, branch, presubmits, logger)
			if err != nil {
				return err
			}
			if err := c.triggerIfTrusted(org, repo, pr, toTrigger); err != nil {
				triggerErrors = append(triggerErrors, fmt.Errorf("failed to trigger jobs for %s#%d: %v", orgrepo, pr.Number, err))
				if !c.continueOnError {
					return utilerrors.NewAggregate(triggerErrors)
				}
				continue
			}
		}
	}
	return utilerrors.NewAggregate(triggerErrors)
}

func (c *Controller) triggerIfTrusted(org, repo string, pr github.PullRequest, toTrigger []config.Presubmit) error {
	trusted, err := c.trustedChecker.trustedPullRequest(pr.User.Login, org, repo, pr.Number)
	if err != nil {
		return fmt.Errorf("failed to determine if %s/%s#%d is trusted: %v", org, repo, pr.Number, err)
	}
	if !trusted {
		return nil
	}
	var triggeredContexts []map[string]string
	for _, presubmit := range toTrigger {
		triggeredContexts = append(triggeredContexts, map[string]string{"job": presubmit.Name, "context": presubmit.Context})
	}
	logrus.WithFields(logrus.Fields{
		"to-trigger": triggeredContexts,
		"pr":         pr.Number,
		"org":        org,
		"repo":       repo,
	}).Info("Triggering and skipping new ProwJobs to create newly-required contexts.")
	return c.prowJobTriggerer.runAndSkip(&pr, toTrigger)
}

func (c *Controller) retireRemovedContexts(retiredPresubmits map[string][]config.Presubmit, log *logrus.Entry) error {
	var retireErrors []error
	for orgrepo, presubmits := range retiredPresubmits {
		parts := strings.SplitN(orgrepo, "/", 2)
		if n := len(parts); n != 2 {
			retireErrors = append(retireErrors, fmt.Errorf("string %q can not be interpreted as org/repo", orgrepo))
			continue
		}
		org, repo := parts[0], parts[1]
		if c.addedPresubmitDenylistAll.Has(org) || c.addedPresubmitDenylistAll.Has(orgrepo) {
			continue
		}
		for _, presubmit := range presubmits {
			log.WithFields(logrus.Fields{
				"org":     org,
				"repo":    repo,
				"context": presubmit.Context,
			}).Info("Retiring context.")
			if err := c.statusMigrator.retire(org, repo, presubmit.Context, presubmit.Brancher.ShouldRun); err != nil {
				if c.continueOnError {
					retireErrors = append(retireErrors, err)
					continue
				}
				return err
			}
		}
	}
	return utilerrors.NewAggregate(retireErrors)
}

func (c *Controller) updateMigratedContexts(migrations map[string][]presubmitMigration, log *logrus.Entry) error {
	var migrateErrors []error
	for orgrepo, migrations := range migrations {
		parts := strings.SplitN(orgrepo, "/", 2)
		if n := len(parts); n != 2 {
			migrateErrors = append(migrateErrors, fmt.Errorf("string %q can not be interpreted as org/repo", orgrepo))
			continue
		}
		org, repo := parts[0], parts[1]
		if c.addedPresubmitDenylistAll.Has(org) || c.addedPresubmitDenylistAll.Has(orgrepo) {
			continue
		}
		for _, migration := range migrations {
			log.WithFields(logrus.Fields{
				"org":  org,
				"repo": repo,
				"from": migration.from.Context,
				"to":   migration.to.Context,
			}).Info("Migrating context.")
			if err := c.statusMigrator.migrate(org, repo, migration.from.Context, migration.to.Context, migration.from.Brancher.ShouldRun); err != nil {
				if c.continueOnError {
					migrateErrors = append(migrateErrors, err)
					continue
				}
				return err
			}
		}
	}
	return utilerrors.NewAggregate(migrateErrors)
}

// addedBlockingPresubmits determines new blocking presubmits based on a
// config update. New blocking presubmits are either brand-new presubmits
// or extant presubmits that are now reporting. Previous presubmits that
// reported but were optional that are no longer optional require no action
// as their contexts will already exist on PRs.
func addedBlockingPresubmits(old, new map[string][]config.Presubmit, log *logrus.Entry) (map[string][]config.Presubmit, *logrus.Entry) {
	added := map[string][]config.Presubmit{}

	for repo, oldPresubmits := range old {
		added[repo] = []config.Presubmit{}
		for _, newPresubmit := range new[repo] {
			if !newPresubmit.ContextRequired() || newPresubmit.NeedsExplicitTrigger() {
				continue
			}
			var found bool
			for _, oldPresubmit := range oldPresubmits {
				if oldPresubmit.Name == newPresubmit.Name {
					if oldPresubmit.SkipReport && !newPresubmit.SkipReport {
						added[repo] = append(added[repo], newPresubmit)
						log.WithFields(logrus.Fields{
							"repo": repo,
							"name": oldPresubmit.Name,
						}).Debug("Identified a newly-reporting blocking presubmit.")
					}
					if oldPresubmit.RunIfChanged != newPresubmit.RunIfChanged {
						added[repo] = append(added[repo], newPresubmit)
						log.WithFields(logrus.Fields{
							"repo": repo,
							"name": oldPresubmit.Name,
						}).Debug("Identified a blocking presubmit running over a different set of files.")
					}
					found = true
					break
				}
			}
			if !found {
				added[repo] = append(added[repo], newPresubmit)
				log.WithFields(logrus.Fields{
					"repo": repo,
					"name": newPresubmit.Name,
				}).Debug("Identified an added blocking presubmit.")
			}
		}
	}

	var numAdded int
	for _, presubmits := range added {
		numAdded += len(presubmits)
	}
	log.Infof("Identified %d added blocking presubmits.", numAdded)
	return added, log
}

// removedPresubmits determines stale presubmits based on a config update.
func removedPresubmits(old, new map[string][]config.Presubmit, log *logrus.Entry) (map[string][]config.Presubmit, *logrus.Entry) {
	removed := map[string][]config.Presubmit{}
	for repo, oldPresubmits := range old {
		removed[repo] = []config.Presubmit{}
		for _, oldPresubmit := range oldPresubmits {
			var found bool
			for _, newPresubmit := range new[repo] {
				if oldPresubmit.Name == newPresubmit.Name {
					found = true
					break
				}
			}
			if !found {
				removed[repo] = append(removed[repo], oldPresubmit)
				log.WithFields(logrus.Fields{
					"repo": repo,
					"name": oldPresubmit.Name,
				}).Debug("Identified a removed blocking presubmit.")
			}
		}
	}

	var numRemoved int
	for _, presubmits := range removed {
		numRemoved += len(presubmits)
	}
	log.Infof("Identified %d removed blocking presubmits.", numRemoved)
	return removed, log
}

type presubmitMigration struct {
	from, to config.Presubmit
}

// migratedBlockingPresubmits determines blocking presubmits that have had
// their status contexts migrated. This is a best-effort evaluation as we
// can only track a presubmit between configuration versions by its name.
// A presubmit "migration" that had its underlying job and context changed
// will be treated as a deletion and creation.
func migratedBlockingPresubmits(old, new map[string][]config.Presubmit, log *logrus.Entry) (map[string][]presubmitMigration, *logrus.Entry) {
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
					log.WithFields(logrus.Fields{
						"repo": repo,
						"name": oldPresubmit.Name,
						"from": oldPresubmit.Context,
						"to":   newPresubmit.Context,
					}).Debug("Identified a migrated blocking presubmit.")
				}
			}
		}
	}

	var numMigrated int
	for _, presubmits := range migrated {
		numMigrated += len(presubmits)
	}
	log.Infof("Identified %d migrated blocking presubmits.", numMigrated)
	return migrated, log
}
