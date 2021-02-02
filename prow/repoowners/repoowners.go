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

package repoowners

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/yaml"

	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pkg/layeredsets"
	"k8s.io/test-infra/prow/plugins/ownersconfig"

	prowConf "k8s.io/test-infra/prow/config"
)

const (
	// GitHub's api uses "" (empty) string as basedir by convention but it's clearer to use "/"
	baseDirConvention = ""
)

type dirOptions struct {
	NoParentOwners bool `json:"no_parent_owners,omitempty"`
}

// Config holds roles+usernames and labels for a directory considered as a unit of independent code
type Config struct {
	Approvers         []string `json:"approvers,omitempty"`
	Reviewers         []string `json:"reviewers,omitempty"`
	RequiredReviewers []string `json:"required_reviewers,omitempty"`
	Labels            []string `json:"labels,omitempty"`
}

// SimpleConfig holds options and Config applied to everything under the containing directory
type SimpleConfig struct {
	Options dirOptions `json:"options,omitempty"`
	Config  `json:",inline"`
}

// Empty checks if a SimpleConfig could be considered empty
func (s *SimpleConfig) Empty() bool {
	return len(s.Approvers) == 0 && len(s.Reviewers) == 0 && len(s.RequiredReviewers) == 0 && len(s.Labels) == 0
}

// FullConfig contains Filters which apply specific Config to files matching its regexp
type FullConfig struct {
	Options dirOptions        `json:"options,omitempty"`
	Filters map[string]Config `json:"filters,omitempty"`
}

type githubClient interface {
	ListCollaborators(org, repo string) ([]github.User, error)
	GetRef(org, repo, ref string) (string, error)
}

func newCache() *cache {
	return &cache{
		lockMapLock: &sync.Mutex{},
		lockMap:     map[string]*sync.Mutex{},
		dataLock:    &sync.Mutex{},
		data:        map[string]cacheEntry{},
	}
}

type cache struct {
	// These are used to lock access to individual keys to avoid wasted tokens
	// on concurrent requests. This has no effect when using ghproxy, as ghproxy
	// serializes identical requests anyways. This should be removed once ghproxy
	// is made mandatory.
	lockMapLock *sync.Mutex
	lockMap     map[string]*sync.Mutex

	dataLock *sync.Mutex
	data     map[string]cacheEntry
}

// getEntry returns the data for the key, a boolean indicating if data existed and a lock.
// The lock is already locked, it must be unlocked by the caller.
func (c *cache) getEntry(key string) (cacheEntry, bool, *sync.Mutex) {
	c.lockMapLock.Lock()
	entryLock, ok := c.lockMap[key]
	if !ok {
		c.lockMap[key] = &sync.Mutex{}
		entryLock = c.lockMap[key]
	}
	c.lockMapLock.Unlock()

	entryLock.Lock()
	c.dataLock.Lock()
	defer c.dataLock.Unlock()
	entry, ok := c.data[key]
	return entry, ok, entryLock
}

func (c *cache) setEntry(key string, data cacheEntry) {
	c.dataLock.Lock()
	c.data[key] = data
	c.dataLock.Unlock()
}

type cacheEntry struct {
	sha     string
	aliases RepoAliases
	owners  *RepoOwners
}

func (entry cacheEntry) matchesMDYAML(mdYAML bool) bool {
	return entry.owners.enableMDYAML == mdYAML
}

func (entry cacheEntry) fullyLoaded() bool {
	return entry.sha != "" && entry.aliases != nil && entry.owners != nil
}

// Interface is an interface to work with OWNERS files.
type Interface interface {
	LoadRepoAliases(org, repo, base string) (RepoAliases, error)
	LoadRepoOwners(org, repo, base string) (RepoOwner, error)

	WithFields(fields logrus.Fields) Interface
	WithGitHubClient(client github.Client) Interface
}

// Client is an implementation of the Interface.
var _ Interface = &Client{}

// Client is the repoowners client
type Client struct {
	logger *logrus.Entry
	ghc    githubClient
	*delegate
}

type delegate struct {
	git git.ClientFactory

	mdYAMLEnabled      func(org, repo string) bool
	skipCollaborators  func(org, repo string) bool
	ownersDirBlacklist func() prowConf.OwnersDirBlacklist
	filenames          ownersconfig.Resolver

	cache *cache
}

// WithFields clones the client, keeping the underlying delegate the same but adding
// fields to the logging context
func (c *Client) WithFields(fields logrus.Fields) Interface {
	return &Client{
		logger:   c.logger.WithFields(fields),
		delegate: c.delegate,
	}
}

// WithGitHubClient clones the client, keeping the underlying delegate the same but adding
// a new GitHub Client. This is useful when making use a context-local client
func (c *Client) WithGitHubClient(client github.Client) Interface {
	return &Client{
		logger:   c.logger,
		ghc:      client,
		delegate: c.delegate,
	}
}

// NewClient is the constructor for Client
func NewClient(
	gc git.ClientFactory,
	ghc github.Client,
	mdYAMLEnabled func(org, repo string) bool,
	skipCollaborators func(org, repo string) bool,
	ownersDirBlacklist func() prowConf.OwnersDirBlacklist,
	filenames ownersconfig.Resolver,
) *Client {
	return &Client{
		logger: logrus.WithField("client", "repoowners"),
		ghc:    ghc,
		delegate: &delegate{
			git:   gc,
			cache: newCache(),

			mdYAMLEnabled:      mdYAMLEnabled,
			skipCollaborators:  skipCollaborators,
			ownersDirBlacklist: ownersDirBlacklist,
			filenames:          filenames,
		},
	}
}

// RepoAliases defines groups of people to be used in OWNERS files
type RepoAliases map[string]sets.String

// RepoOwner is an interface to work with repoowners
type RepoOwner interface {
	FindApproverOwnersForFile(path string) string
	FindReviewersOwnersForFile(path string) string
	FindLabelsForFile(path string) sets.String
	IsNoParentOwners(path string) bool
	LeafApprovers(path string) sets.String
	Approvers(path string) layeredsets.String
	LeafReviewers(path string) sets.String
	Reviewers(path string) layeredsets.String
	RequiredReviewers(path string) sets.String
	ParseSimpleConfig(path string) (SimpleConfig, error)
	ParseFullConfig(path string) (FullConfig, error)
	TopLevelApprovers() sets.String
	Filenames() ownersconfig.Filenames
}

var _ RepoOwner = &RepoOwners{}

// RepoOwners contains the parsed OWNERS config.
type RepoOwners struct {
	RepoAliases

	approvers         map[string]map[*regexp.Regexp]sets.String
	reviewers         map[string]map[*regexp.Regexp]sets.String
	requiredReviewers map[string]map[*regexp.Regexp]sets.String
	labels            map[string]map[*regexp.Regexp]sets.String
	options           map[string]dirOptions

	baseDir      string
	enableMDYAML bool
	dirBlacklist []*regexp.Regexp
	filenames    ownersconfig.Filenames

	log *logrus.Entry
}

func (r *RepoOwners) Filenames() ownersconfig.Filenames {
	return r.filenames
}

// LoadRepoAliases returns an up-to-date RepoAliases struct for the specified repo.
// If the repo does not have an aliases file then an empty alias map is returned with no error.
// Note: The returned RepoAliases should be treated as read only.
func (c *Client) LoadRepoAliases(org, repo, base string) (RepoAliases, error) {
	log := c.logger.WithFields(logrus.Fields{"org": org, "repo": repo, "base": base})
	cloneRef := fmt.Sprintf("%s/%s", org, repo)
	fullName := fmt.Sprintf("%s:%s", cloneRef, base)

	sha, err := c.ghc.GetRef(org, repo, fmt.Sprintf("heads/%s", base))
	if err != nil {
		return nil, fmt.Errorf("failed to get current SHA for %s: %v", fullName, err)
	}

	entry, ok, entryLock := c.cache.getEntry(fullName)
	defer entryLock.Unlock()
	if !ok || entry.sha != sha {
		// entry is non-existent or stale.
		gitRepo, err := c.git.ClientFor(org, repo)
		if err != nil {
			return nil, fmt.Errorf("failed to clone %s: %v", cloneRef, err)
		}
		defer gitRepo.Clean()
		if err := gitRepo.Checkout(base); err != nil {
			return nil, err
		}

		entry.aliases = loadAliasesFrom(gitRepo.Directory(), c.filenames(org, repo).OwnersAliases, log)
		entry.sha = sha
		c.cache.setEntry(fullName, entry)
	}

	return entry.aliases, nil
}

// LoadRepoOwners returns an up-to-date RepoOwners struct for the specified repo.
// Note: The returned *RepoOwners should be treated as read only.
func (c *Client) LoadRepoOwners(org, repo, base string) (RepoOwner, error) {
	log := c.logger.WithFields(logrus.Fields{"org": org, "repo": repo, "base": base})
	cloneRef := fmt.Sprintf("%s/%s", org, repo)
	fullName := fmt.Sprintf("%s:%s", cloneRef, base)

	start := time.Now()
	sha, err := c.ghc.GetRef(org, repo, fmt.Sprintf("heads/%s", base))
	if err != nil {
		return nil, fmt.Errorf("failed to get current SHA for %s: %v", fullName, err)
	}
	log.WithField("duration", time.Since(start).String()).Debugf("Completed ghc.GetRef(%s, %s, %s)", org, repo, fmt.Sprintf("heads/%s", base))

	entry, err := c.cacheEntryFor(org, repo, base, cloneRef, fullName, sha, log)
	if err != nil {
		return nil, err
	}

	start = time.Now()
	if c.skipCollaborators(org, repo) {
		log.WithField("duration", time.Since(start).String()).Debugf("Completed c.skipCollaborators(%s, %s)", org, repo)
		log.Debugf("Skipping collaborator checks for %s/%s", org, repo)
		return entry.owners, nil
	}
	log.WithField("duration", time.Since(start).String()).Debugf("Completed c.skipCollaborators(%s, %s)", org, repo)

	var owners *RepoOwners
	// Filter collaborators. We must filter the RepoOwners struct even if it came from the cache
	// because the list of collaborators could have changed without the git SHA changing.
	start = time.Now()
	collaborators, err := c.ghc.ListCollaborators(org, repo)
	log.WithField("duration", time.Since(start).String()).Debugf("Completed ghc.ListCollaborators(%s, %s)", org, repo)
	if err != nil {
		log.WithError(err).Errorf("Failed to list collaborators while loading RepoOwners. Skipping collaborator filtering.")
		owners = entry.owners
	} else {
		start = time.Now()
		owners = entry.owners.filterCollaborators(collaborators)
		log.WithField("duration", time.Since(start).String()).Debugf("Completed owners.filterCollaborators(collaborators)")
	}
	return owners, nil
}

func (c *Client) cacheEntryFor(org, repo, base, cloneRef, fullName, sha string, log *logrus.Entry) (cacheEntry, error) {
	mdYaml := c.mdYAMLEnabled(org, repo)
	lockStart := time.Now()
	defer func() {
		log.WithField("duration", time.Since(lockStart).String()).Debug("Locked section of loadRepoOwners completed")
	}()
	entry, ok, entryLock := c.cache.getEntry(fullName)
	defer entryLock.Unlock()
	filenames := c.filenames(org, repo)
	if !ok || entry.sha != sha || entry.owners == nil || !entry.matchesMDYAML(mdYaml) {
		start := time.Now()
		gitRepo, err := c.git.ClientFor(org, repo)
		if err != nil {
			return cacheEntry{}, fmt.Errorf("failed to clone %s: %v", cloneRef, err)
		}
		log.WithField("duration", time.Since(start).String()).Debugf("Completed git.ClientFor(%s, %s)", org, repo)
		defer gitRepo.Clean()

		reusable := entry.fullyLoaded() && entry.matchesMDYAML(mdYaml)
		// In most sha changed cases, the files associated with the owners are unchanged.
		// The cached entry can continue to be used, so need do git diff
		if reusable {
			start = time.Now()
			changes, err := gitRepo.Diff(sha, entry.sha)
			if err != nil {
				return cacheEntry{}, fmt.Errorf("failed to diff %s with %s", sha, entry.sha)
			}
			log.WithField("duration", time.Since(start).String()).Debugf("Completed git.Diff(%s, %s)", sha, entry.sha)
			start = time.Now()
			for _, change := range changes {
				if mdYaml && strings.HasSuffix(change, ".md") ||
					strings.HasSuffix(change, filenames.OwnersAliases) ||
					strings.HasSuffix(change, filenames.Owners) {
					reusable = false
					log.WithField("duration", time.Since(start).String()).Debugf("Completed owners change verification loop")
					break
				}
			}
			log.WithField("duration", time.Since(start).String()).Debugf("Completed owners change verification loop")
		}
		if reusable {
			entry.sha = sha
		} else {
			start = time.Now()
			if err := gitRepo.Checkout(base); err != nil {
				return cacheEntry{}, err
			}
			log.WithField("duration", time.Since(start).String()).Debugf("Completed gitRepo.Checkout(%s)", base)

			start = time.Now()
			if entry.aliases == nil || entry.sha != sha {
				// aliases must be loaded
				entry.aliases = loadAliasesFrom(gitRepo.Directory(), filenames.OwnersAliases, log)
			}
			log.WithField("duration", time.Since(start).String()).Debugf("Completed loadAliasesFrom(%s, log)", gitRepo.Directory())

			start = time.Now()
			ignoreDirPatterns := c.ownersDirBlacklist().ListIgnoredDirs(org, repo)
			var dirIgnorelist []*regexp.Regexp
			for _, pattern := range ignoreDirPatterns {
				re, err := regexp.Compile(pattern)
				if err != nil {
					log.WithError(err).Errorf("Invalid OWNERS dir blacklist regexp %q.", pattern)
					continue
				}
				dirIgnorelist = append(dirIgnorelist, re)
			}
			log.WithField("duration", time.Since(start).String()).Debugf("Completed dirIgnorelist loading")

			start = time.Now()
			entry.owners, err = loadOwnersFrom(gitRepo.Directory(), mdYaml, entry.aliases, dirIgnorelist, filenames, log)
			if err != nil {
				return cacheEntry{}, fmt.Errorf("failed to load RepoOwners for %s: %v", fullName, err)
			}
			log.WithField("duration", time.Since(start).String()).Debugf("Completed loadOwnersFrom(%s, %t, entry.aliases, dirIgnorelist, log)", gitRepo.Directory(), mdYaml)
			entry.sha = sha
			c.cache.setEntry(fullName, entry)
		}
	}
	return entry, nil
}

// ExpandAlias returns members of an alias
func (a RepoAliases) ExpandAlias(alias string) sets.String {
	if a == nil {
		return nil
	}
	return a[github.NormLogin(alias)]
}

// ExpandAliases returns members of multiple aliases, duplicates are pruned
func (a RepoAliases) ExpandAliases(logins sets.String) sets.String {
	if a == nil {
		return logins
	}
	// Make logins a copy of the original set to avoid modifying the original.
	logins = logins.Union(nil)
	for _, login := range logins.List() {
		if expanded, ok := a[github.NormLogin(login)]; ok {
			logins.Delete(login)
			logins = logins.Union(expanded)
		}
	}
	return logins
}

// ExpandAllAliases returns members of all aliases mentioned, duplicates are pruned
func (a RepoAliases) ExpandAllAliases() sets.String {
	if a == nil {
		return nil
	}

	var result, users sets.String
	for alias := range a {
		users = a.ExpandAlias(alias)
		result = result.Union(users)
	}
	return result
}

func loadAliasesFrom(baseDir, filename string, log *logrus.Entry) RepoAliases {
	path := filepath.Join(baseDir, filename)
	b, err := ioutil.ReadFile(path)
	if os.IsNotExist(err) {
		log.WithError(err).Infof("No alias file exists at %q. Using empty alias map.", path)
		return nil
	} else if err != nil {
		log.WithError(err).Warnf("Failed to read alias file %q. Using empty alias map.", path)
		return nil
	}
	result, err := ParseAliasesConfig(b)
	if err != nil {
		log.WithError(err).Errorf("Failed to unmarshal aliases from %q. Using empty alias map.", path)
	}
	log.Infof("Loaded %d aliases from %q.", len(result), path)
	return result
}

func loadOwnersFrom(baseDir string, mdYaml bool, aliases RepoAliases, dirIgnorelist []*regexp.Regexp, filenames ownersconfig.Filenames, log *logrus.Entry) (*RepoOwners, error) {
	o := &RepoOwners{
		RepoAliases:  aliases,
		baseDir:      baseDir,
		enableMDYAML: mdYaml,
		filenames:    filenames,
		log:          log,

		approvers:         make(map[string]map[*regexp.Regexp]sets.String),
		reviewers:         make(map[string]map[*regexp.Regexp]sets.String),
		requiredReviewers: make(map[string]map[*regexp.Regexp]sets.String),
		labels:            make(map[string]map[*regexp.Regexp]sets.String),
		options:           make(map[string]dirOptions),

		dirBlacklist: dirIgnorelist,
	}

	return o, filepath.Walk(o.baseDir, o.walkFunc)
}

// by default, github's api doesn't root the project directory at "/" and instead uses the empty string for the base dir
// of the project. And the built-in dir function returns "." for empty strings, so for consistency, we use this
// canonicalize to get the directories of files in a consistent format with NO "/" at the root (a/b/c/ -> a/b/c)
func canonicalize(path string) string {
	if path == "." {
		return baseDirConvention
	}
	return strings.TrimSuffix(path, "/")
}

func (o *RepoOwners) walkFunc(path string, info os.FileInfo, err error) error {
	log := o.log.WithField("path", path)
	if err != nil {
		log.WithError(err).Error("Error while walking OWNERS files.")
		return nil
	}
	filename := filepath.Base(path)
	relPath, err := filepath.Rel(o.baseDir, path)
	if err != nil {
		log.WithError(err).Errorf("Unable to find relative path between baseDir: %q and path.", o.baseDir)
		return err
	}
	relPathDir := canonicalize(filepath.Dir(relPath))

	if info.Mode().IsDir() {
		for _, re := range o.dirBlacklist {
			if re.MatchString(relPath) {
				return filepath.SkipDir
			}
		}
	}
	if !info.Mode().IsRegular() {
		return nil
	}

	// '.md' files may contain assignees at the top of the file in a yaml header
	// Note that these assignees only apply to the file itself.
	if o.enableMDYAML && strings.HasSuffix(filename, ".md") {
		// Parse the yaml header from the file if it exists and marshal into the config
		simple := &SimpleConfig{}
		if err := decodeOwnersMdConfig(path, simple); err != nil {
			log.WithError(err).Info("Error decoding OWNERS config from '*.md' file.")
			return nil
		}

		// Set owners for this file (not the directory) using the relative path if they were found
		o.applyConfigToPath(relPath, nil, &simple.Config)
		o.applyOptionsToPath(relPath, simple.Options)
		return nil
	}

	if filename != o.filenames.Owners {
		return nil
	}

	simple, err := o.ParseSimpleConfig(path)
	if err == filepath.SkipDir {
		return err
	}
	if err != nil || simple.Empty() {
		c, err := o.ParseFullConfig(path)
		if err == filepath.SkipDir {
			return err
		}
		if err != nil {
			log.WithError(err).Debugf("Failed to unmarshal %s into either Simple or FullConfig.", path)
		} else {
			// it's a FullConfig
			for pattern, config := range c.Filters {
				var re *regexp.Regexp
				if pattern != ".*" {
					if re, err = regexp.Compile(pattern); err != nil {
						log.WithError(err).Debugf("Invalid regexp %q.", pattern)
						continue
					}
				}
				o.applyConfigToPath(relPathDir, re, &config)
			}
			o.applyOptionsToPath(relPathDir, c.Options)
		}
	} else {
		// it's a SimpleConfig
		o.applyConfigToPath(relPathDir, nil, &simple.Config)
		o.applyOptionsToPath(relPathDir, simple.Options)
	}
	return nil
}

// ParseFullConfig will unmarshal the content of the OWNERS file at the path into a FullConfig.
// If the OWNERS directory is ignorelisted, it returns filepath.SkipDir.
// Returns an error if the content cannot be unmarshalled.
func (o *RepoOwners) ParseFullConfig(path string) (FullConfig, error) {
	// if path is in an ignored directory, ignore it
	dir := filepath.Dir(path)
	for _, re := range o.dirBlacklist {
		if re.MatchString(dir) {
			return FullConfig{}, filepath.SkipDir
		}
	}

	b, err := ioutil.ReadFile(path)
	if err != nil {
		return FullConfig{}, err
	}
	return LoadFullConfig(b)
}

// ParseSimpleConfig will unmarshal the content of the OWNERS file at the path into a SimpleConfig.
// If the OWNERS directory is ignorelisted, it returns filepath.SkipDir.
// Returns an error if the content cannot be unmarshalled.
func (o *RepoOwners) ParseSimpleConfig(path string) (SimpleConfig, error) {
	// if path is in a an ignored directory, ignore it
	dir := filepath.Dir(path)
	for _, re := range o.dirBlacklist {
		if re.MatchString(dir) {
			return SimpleConfig{}, filepath.SkipDir
		}
	}

	b, err := ioutil.ReadFile(path)
	if err != nil {
		return SimpleConfig{}, err
	}
	return LoadSimpleConfig(b)
}

// LoadSimpleConfig loads SimpleConfig from bytes `b`
func LoadSimpleConfig(b []byte) (SimpleConfig, error) {
	simple := new(SimpleConfig)
	err := yaml.Unmarshal(b, simple)
	return *simple, err
}

// SaveSimpleConfig writes SimpleConfig to `path`
func SaveSimpleConfig(simple SimpleConfig, path string) error {
	b, err := yaml.Marshal(simple)
	if err != nil {
		return nil
	}
	return ioutil.WriteFile(path, b, 0644)
}

// LoadFullConfig loads FullConfig from bytes `b`
func LoadFullConfig(b []byte) (FullConfig, error) {
	full := new(FullConfig)
	err := yaml.Unmarshal(b, full)
	return *full, err
}

// SaveFullConfig writes FullConfig to `path`
func SaveFullConfig(full FullConfig, path string) error {
	b, err := yaml.Marshal(full)
	if err != nil {
		return nil
	}
	return ioutil.WriteFile(path, b, 0644)
}

// ParseAliasesConfig will unmarshal an OWNERS_ALIASES file's content into RepoAliases.
// Returns an error if the content cannot be unmarshalled.
func ParseAliasesConfig(b []byte) (RepoAliases, error) {
	result := make(RepoAliases)

	config := &struct {
		Data map[string][]string `json:"aliases,omitempty"`
	}{}
	if err := yaml.Unmarshal(b, config); err != nil {
		return result, err
	}

	for alias, expanded := range config.Data {
		result[github.NormLogin(alias)] = NormLogins(expanded)
	}
	return result, nil
}

var mdStructuredHeaderRegex = regexp.MustCompile("^---\n(.|\n)*\n---")

// decodeOwnersMdConfig will parse the yaml header if it exists and unmarshal it into a singleOwnersConfig.
// If no yaml header is found, do nothing
// Returns an error if the file cannot be read or the yaml header is found but cannot be unmarshalled.
func decodeOwnersMdConfig(path string, config *SimpleConfig) error {
	fileBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	// Parse the yaml header from the top of the file.  Will return an empty string if regex does not match.
	meta := mdStructuredHeaderRegex.FindString(string(fileBytes))

	// Unmarshal the yaml header into the config
	return yaml.Unmarshal([]byte(meta), &config)
}

// NormLogins normalizes logins
func NormLogins(logins []string) sets.String {
	normed := sets.NewString()
	for _, login := range logins {
		normed.Insert(github.NormLogin(login))
	}
	return normed
}

var defaultDirOptions = dirOptions{}

func (o *RepoOwners) applyConfigToPath(path string, re *regexp.Regexp, config *Config) {
	if len(config.Approvers) > 0 {
		if o.approvers[path] == nil {
			o.approvers[path] = make(map[*regexp.Regexp]sets.String)
		}
		o.approvers[path][re] = o.ExpandAliases(NormLogins(config.Approvers))
	}
	if len(config.Reviewers) > 0 {
		if o.reviewers[path] == nil {
			o.reviewers[path] = make(map[*regexp.Regexp]sets.String)
		}
		o.reviewers[path][re] = o.ExpandAliases(NormLogins(config.Reviewers))
	}
	if len(config.RequiredReviewers) > 0 {
		if o.requiredReviewers[path] == nil {
			o.requiredReviewers[path] = make(map[*regexp.Regexp]sets.String)
		}
		o.requiredReviewers[path][re] = o.ExpandAliases(NormLogins(config.RequiredReviewers))
	}
	if len(config.Labels) > 0 {
		if o.labels[path] == nil {
			o.labels[path] = make(map[*regexp.Regexp]sets.String)
		}
		o.labels[path][re] = sets.NewString(config.Labels...)
	}
}

func (o *RepoOwners) applyOptionsToPath(path string, opts dirOptions) {
	if opts != defaultDirOptions {
		o.options[path] = opts
	}
}

func (o *RepoOwners) filterCollaborators(toKeep []github.User) *RepoOwners {
	collabs := sets.NewString()
	for _, keeper := range toKeep {
		collabs.Insert(github.NormLogin(keeper.Login))
	}

	filter := func(ownerMap map[string]map[*regexp.Regexp]sets.String) map[string]map[*regexp.Regexp]sets.String {
		filtered := make(map[string]map[*regexp.Regexp]sets.String)
		for path, reMap := range ownerMap {
			filtered[path] = make(map[*regexp.Regexp]sets.String)
			for re, unfiltered := range reMap {
				filtered[path][re] = unfiltered.Intersection(collabs)
			}
		}
		return filtered
	}

	result := *o
	result.approvers = filter(o.approvers)
	result.reviewers = filter(o.reviewers)
	return &result
}

// findOwnersForFile returns the OWNERS file path furthest down the tree for a specified file
// using ownerMap to check for entries
func findOwnersForFile(log *logrus.Entry, path string, ownerMap map[string]map[*regexp.Regexp]sets.String) string {
	d := path

	for ; d != baseDirConvention; d = canonicalize(filepath.Dir(d)) {
		relative, err := filepath.Rel(d, path)
		if err != nil {
			log.WithError(err).WithField("path", path).Errorf("Unable to find relative path between %q and path.", d)
			return ""
		}
		for re, n := range ownerMap[d] {
			if re != nil && !re.MatchString(relative) {
				continue
			}
			if len(n) != 0 {
				return d
			}
		}
	}
	return ""
}

// FindApproverOwnersForFile returns the OWNERS file path furthest down the tree for a specified file
// that contains an approvers section
func (o *RepoOwners) FindApproverOwnersForFile(path string) string {
	return findOwnersForFile(o.log, path, o.approvers)
}

// FindReviewersOwnersForFile returns the OWNERS file path furthest down the tree for a specified file
// that contains a reviewers section
func (o *RepoOwners) FindReviewersOwnersForFile(path string) string {
	return findOwnersForFile(o.log, path, o.reviewers)
}

// FindLabelsForFile returns a set of labels which should be applied to PRs
// modifying files under the given path.
func (o *RepoOwners) FindLabelsForFile(path string) sets.String {
	return o.entriesForFile(path, o.labels, false).Set()
}

// IsNoParentOwners checks if an OWNERS file path refers to an OWNERS file with NoParentOwners enabled.
func (o *RepoOwners) IsNoParentOwners(path string) bool {
	return o.options[path].NoParentOwners
}

// entriesForFile returns a set of users who are assignees to the
// requested file. The path variable should be a full path to a filename
// and not directory as the final directory will be discounted if enableMDYAML is true
// leafOnly indicates whether only the OWNERS deepest in the tree (closest to the file)
// should be returned or if all OWNERS in filepath should be returned
func (o *RepoOwners) entriesForFile(path string, people map[string]map[*regexp.Regexp]sets.String, leafOnly bool) layeredsets.String {
	d := path
	if !o.enableMDYAML || !strings.HasSuffix(path, ".md") {
		d = canonicalize(d)
	}

	out := layeredsets.NewString()
	var layerID int
	for {
		relative, err := filepath.Rel(d, path)
		if err != nil {
			o.log.WithError(err).WithField("path", path).Errorf("Unable to find relative path between %q and path.", d)
			return nil
		}
		for re, s := range people[d] {
			if re == nil || re.MatchString(relative) {
				out.Insert(layerID, s.List()...)
			}
		}
		if leafOnly && out.Len() > 0 {
			break
		}
		if d == baseDirConvention {
			break
		}
		if o.options[d].NoParentOwners {
			break
		}
		d = filepath.Dir(d)
		d = canonicalize(d)
		layerID++
	}
	return out
}

// LeafApprovers returns a set of users who are the closest approvers to the
// requested file. If pkg/OWNERS has user1 and pkg/util/OWNERS has user2 this
// will only return user2 for the path pkg/util/sets/file.go
func (o *RepoOwners) LeafApprovers(path string) sets.String {
	return o.entriesForFile(path, o.approvers, true).Set()
}

// Approvers returns ALL of the users who are approvers for the
// requested file (including approvers in parent dirs' OWNERS).
// If pkg/OWNERS has user1 and pkg/util/OWNERS has user2 this
// will return both user1 and user2 for the path pkg/util/sets/file.go
func (o *RepoOwners) Approvers(path string) layeredsets.String {
	return o.entriesForFile(path, o.approvers, false)
}

// LeafReviewers returns a set of users who are the closest reviewers to the
// requested file. If pkg/OWNERS has user1 and pkg/util/OWNERS has user2 this
// will only return user2 for the path pkg/util/sets/file.go
func (o *RepoOwners) LeafReviewers(path string) sets.String {
	return o.entriesForFile(path, o.reviewers, true).Set()
}

// Reviewers returns ALL of the users who are reviewers for the
// requested file (including reviewers in parent dirs' OWNERS).
// If pkg/OWNERS has user1 and pkg/util/OWNERS has user2 this
// will return both user1 and user2 for the path pkg/util/sets/file.go
func (o *RepoOwners) Reviewers(path string) layeredsets.String {
	return o.entriesForFile(path, o.reviewers, false)
}

// RequiredReviewers returns ALL of the users who are required_reviewers for the
// requested file (including required_reviewers in parent dirs' OWNERS).
// If pkg/OWNERS has user1 and pkg/util/OWNERS has user2 this
// will return both user1 and user2 for the path pkg/util/sets/file.go
func (o *RepoOwners) RequiredReviewers(path string) sets.String {
	return o.entriesForFile(path, o.requiredReviewers, false).Set()
}

func (o *RepoOwners) TopLevelApprovers() sets.String {
	return o.entriesForFile(".", o.approvers, true).Set()
}
