/*
Copyright 2016 The Kubernetes Authors.

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

package features

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/test-infra/mungegithub/github"
	"k8s.io/test-infra/mungegithub/options"

	parseYaml "github.com/ghodss/yaml"
	"github.com/golang/glog"
)

const (
	ownerFilename = "OWNERS" // file which contains approvers and reviewers
	// RepoFeatureName is how mungers should indicate this is required
	RepoFeatureName = "gitrepos"
	// Github's api uses "" (empty) string as basedir by convention but it's clearer to use "/"
	baseDirConvention = ""
)

type assignmentConfig struct {
	Approvers []string `json:"approvers" yaml:"approvers"`
	Reviewers []string `json:"reviewers" yaml:"reviewers"`
	Labels    []string `json:"labels" yaml:"labels"`
}

type aliasData struct {
	// Contains the mapping between aliases and lists of members.
	AliasMap map[string][]string `json:"aliases"`
}

type aliasReader interface {
	read() ([]byte, error)
}

func (o *RepoInfo) read() ([]byte, error) {
	return ioutil.ReadFile(o.aliasFile)
}

// RepoInfo provides information about users in OWNERS files in a git repo
type RepoInfo struct {
	baseDir      string
	enableMdYaml bool
	useReviewers bool

	aliasFile   string
	aliasData   *aliasData
	aliasReader aliasReader

	projectDir        string
	approvers         map[string]sets.String
	reviewers         map[string]sets.String
	labels            map[string]sets.String
	allPossibleLabels sets.String
	config            *github.Config
}

func init() {
	RegisterFeature(&RepoInfo{})
}

// Name is just going to return the name mungers use to request this feature
func (o *RepoInfo) Name() string {
	return RepoFeatureName
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

func (o *RepoInfo) walkFunc(path string, info os.FileInfo, err error) error {
	if err != nil {
		glog.Errorf("%v", err)
		return nil
	}
	filename := filepath.Base(path)
	if info.Mode().IsDir() {
		switch filename {
		case ".git":
			return filepath.SkipDir
		case "_output":
			return filepath.SkipDir
		}
	}
	if !info.Mode().IsRegular() {
		return nil
	}

	c := &assignmentConfig{}

	// '.md' files may contain assignees at the top of the file in a yaml header
	// Flag guarded because this is only enabled in some repos
	if o.enableMdYaml && filename != ownerFilename && strings.HasSuffix(filename, "md") {
		// Parse the yaml header from the file if it exists and marshal into the config
		if err := decodeAssignmentConfig(path, c); err != nil {
			glog.Errorf("%v", err)
			return err
		}

		// Set assignees for this file using the relative path if they were found
		path, err = filepath.Rel(o.projectDir, path)
		if err != nil {
			glog.Errorf("Unable to find relative path between %q and %q: %v", o.projectDir, path, err)
			return err
		}
		c.normalizeUsers()
		o.approvers[path] = sets.NewString(c.Approvers...)
		o.reviewers[path] = sets.NewString(c.Reviewers...)
		return nil
	}

	if filename != ownerFilename {
		return nil
	}

	file, err := os.Open(path)
	if err != nil {
		glog.Errorf("Could not open %q: %v", path, err)
		return nil
	}
	defer file.Close()

	if err := yaml.NewYAMLToJSONDecoder(file).Decode(c); err != nil {
		glog.Errorf("Could not decode %q: %v", path, err)
		return nil
	}

	path, err = filepath.Rel(o.projectDir, path)
	path = filepath.Dir(path)
	if err != nil {
		glog.Errorf("Unable to find relative path between %q and %q: %v", o.projectDir, path, err)
		return err
	}
	path = canonicalize(path)
	c.normalizeUsers()
	o.approvers[path] = sets.NewString(c.Approvers...)
	o.reviewers[path] = sets.NewString(c.Reviewers...)
	if len(c.Labels) > 0 {
		o.labels[path] = sets.NewString(c.Labels...)
		o.allPossibleLabels.Insert(c.Labels...)
	}
	return nil
}

// caseNormalizeAll normalizes all entries in
// the assignment configuration so that we can do case-
// insensitive operations on the entries
func (c *assignmentConfig) normalizeUsers() {
	c.Approvers = caseNormalizeAll(c.Approvers)
	c.Reviewers = caseNormalizeAll(c.Reviewers)
}

func caseNormalizeAll(users []string) []string {
	normalizedUsers := users[:0]
	for _, user := range users {
		normalizedUsers = append(normalizedUsers, caseNormalize(user))
	}
	return normalizedUsers
}

func caseNormalize(user string) string {
	return strings.ToLower(user)
}

// decodeAssignmentConfig will parse the yaml header if it exists and unmarshal it into an assignmentConfig.
// If no yaml header is found, do nothing
// Returns an error if the file cannot be read or the yaml header is found but cannot be unmarshalled
var mdStructuredHeaderRegex = regexp.MustCompile("^---\n(.|\n)*\n---")

func decodeAssignmentConfig(path string, config *assignmentConfig) error {
	fileBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	// Parse the yaml header from the top of the file.  Will return an empty string if regex does not match.
	meta := mdStructuredHeaderRegex.FindString(string(fileBytes))

	// Unmarshal the yaml header into the config
	return parseYaml.Unmarshal([]byte(meta), &config)
}

func (o *RepoInfo) updateRepoAliases() error {
	if len(o.aliasFile) == 0 {
		return nil
	}

	// read and check the alias-file.
	fileContents, err := o.aliasReader.read()
	if os.IsNotExist(err) {
		glog.Infof("Missing alias-file (%s), using empty alias structure.", o.aliasFile)
		o.aliasData = &aliasData{}
		return nil
	}
	if err != nil {
		return fmt.Errorf("Unable to read alias file: %v", err)
	}

	var data aliasData
	if err := parseYaml.Unmarshal(fileContents, &data); err != nil {
		return fmt.Errorf("Failed to decode the alias file: %v", err)
	}

	o.aliasData = normalizeAliases(data)
	return nil
}

// normalizeAliases normalizes all entries in the alias data
// so that we can do case-insensitive resolution of aliases
func normalizeAliases(rawData aliasData) *aliasData {
	normalizedData := aliasData{AliasMap: map[string][]string{}}
	for alias, users := range rawData.AliasMap {
		normalizedData.AliasMap[caseNormalize(alias)] = caseNormalizeAll(users)
	}
	return &normalizedData
}

// expandAllAliases expands all of the aliases in the
// lists of approvers and reviewers for paths we track
func (o *RepoInfo) expandAllAliases() {
	for filePath := range o.approvers {
		o.resolveAliases(o.approvers[filePath])
	}

	for filePath := range o.reviewers {
		o.resolveAliases(o.reviewers[filePath])
	}
}

// resolveAliases returns the members that need to
// be added and removed from a set to yield a set with
// all aliases expanded
func (o *RepoInfo) resolveAliases(users sets.String) {
	for _, owner := range users.List() {
		if aliases := o.resolveAlias(owner); len(aliases) > 0 {
			users.Delete(owner)
			users.Insert(aliases...)
		}
	}
}

// resolveAlias resolves the identity or identities for
// an alias. If we have no record of the alias, an empty
// slice is returned
func (o *RepoInfo) resolveAlias(owner string) []string {
	if val, ok := o.aliasData.AliasMap[owner]; ok {
		return val
	}
	return []string{}
}

func (o *RepoInfo) updateRepoUsers() error {
	out, err := o.GitCommand([]string{"rev-parse", "HEAD"})
	if err != nil {
		glog.Errorf("Unable get sha of HEAD:\n%s\n%v", string(out), err)
		return err
	}
	sha := out

	o.approvers = map[string]sets.String{}
	o.reviewers = map[string]sets.String{}
	o.labels = map[string]sets.String{}
	o.allPossibleLabels = sets.String{}
	err = filepath.Walk(o.projectDir, o.walkFunc)
	if err != nil {
		glog.Errorf("Got error %v", err)
	}
	o.expandAllAliases()
	if err := o.pruneRepoUsers(); err != nil {
		return err
	}
	glog.Infof("Loaded config from %s:%s", o.projectDir, sha)
	glog.V(5).Infof("approvers: %v", o.approvers)
	glog.V(5).Infof("reviewers: %v", o.reviewers)
	return nil
}

// Initialize will initialize the munger
func (o *RepoInfo) Initialize(config *github.Config) error {
	o.config = config
	o.projectDir = path.Join(o.baseDir, o.config.Project)

	if len(o.baseDir) == 0 {
		glog.Fatalf("--repo-dir is required with selected munger(s)")
	}
	finfo, err := os.Stat(o.baseDir)
	if err != nil {
		return fmt.Errorf("Unable to stat --repo-dir: %v", err)
	}
	if !finfo.IsDir() {
		return fmt.Errorf("--repo-dir is not a directory")
	}

	if o.fsckRepo() != nil {
		if err := o.cleanUp(o.projectDir); err != nil {
			return fmt.Errorf("Unable to remove old clone directory at %v: %v", o.projectDir, err)
		}
	}
	if !o.exists() {
		if cloneUrl, err := o.cloneRepo(); err != nil {
			return fmt.Errorf("Unable to clone %v: %v", cloneUrl, err)
		}
	}

	o.aliasData = &aliasData{}
	o.aliasReader = o
	if err := o.updateRepoAliases(); err != nil {
		return err
	}
	return o.updateRepoUsers()
}

func (o *RepoInfo) cleanUp(path string) error {
	return os.RemoveAll(path)
}

func (o *RepoInfo) exists() bool {
	_, err := os.Stat(o.projectDir)
	return !os.IsNotExist(err)
}

func (o *RepoInfo) fsckRepo() error {
	if !o.exists() {
		return fmt.Errorf("fsck repo failed: %q is missing", o.projectDir)
	}
	output, err := o.gitCommandDir([]string{"fsck"}, o.projectDir)
	if err != nil {
		glog.Errorf("fsck repo failed: %s", output)
	}
	return err
}

func (o *RepoInfo) cloneRepo() (string, error) {
	cloneUrl := fmt.Sprintf("https://github.com/%s/%s.git", o.config.Org, o.config.Project)
	output, err := o.gitCommandDir([]string{"clone", cloneUrl, o.projectDir}, o.baseDir)
	if err != nil {
		glog.Errorf("Failed to clone github repo: %s", output)
	}
	return cloneUrl, err
}

func (o *RepoInfo) pruneRepoUsers() error {
	whitelist, err := o.config.Collaborators()
	if err != nil {
		return err
	}

	for repo, users := range o.approvers {
		o.approvers[repo] = whitelist.Intersection(users)
	}
	for repo, users := range o.reviewers {
		o.reviewers[repo] = whitelist.Intersection(users)
	}
	return nil
}

// EachLoop is called at the start of every munge loop
func (o *RepoInfo) EachLoop() error {
	if out, err := o.GitCommand([]string{"remote", "update"}); err != nil {
		glog.Errorf("Unable to git remote update:\n%s\n%v", out, err)
	}
	if out, err := o.GitCommand([]string{"pull"}); err != nil {
		glog.Errorf("Unable to run git pull:\n%s\n%v", string(out), err)
		return err
	}

	if err := o.updateRepoAliases(); err != nil {
		return err
	}
	return o.updateRepoUsers()
}

// RegisterOptions registers options for this feature; returns any that require a restart when changed.
func (o *RepoInfo) RegisterOptions(opts *options.Options) sets.String {
	opts.RegisterString(&o.baseDir, "repo-dir", "", "Path to perform checkout of repository")
	opts.RegisterBool(&o.enableMdYaml, "enable-md-yaml", false, "If true, look for assignees in md yaml headers.")
	opts.RegisterBool(&o.useReviewers, "use-reviewers", false, "Use \"reviewers\" rather than \"approvers\" for review")
	opts.RegisterString(&o.aliasFile, "alias-file", "", "File wherein team members and aliases exist.")
	return sets.NewString("repo-dir")
}

// GitCommand will execute the git command with the `args` within the project directory.
func (o *RepoInfo) GitCommand(args []string) ([]byte, error) {
	return o.gitCommandDir(args, o.projectDir)
}

// GitCommandDir will execute the git command with the `args` within the 'dir' directory.
func (o *RepoInfo) gitCommandDir(args []string, cmdDir string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = cmdDir
	return cmd.CombinedOutput()
}

// findOwnersForPath returns the OWNERS file path furthest down the tree for a specified file
// By default we use the reviewers section of owners flag but this can be configured by setting approvers to true
func findOwnersForPath(path string, ownerMap map[string]sets.String) string {
	d := path

	for {
		n, ok := ownerMap[d]
		if ok && len(n) != 0 {
			return d
		}
		if d == baseDirConvention {
			break
		}
		d = filepath.Dir(d)
		d = canonicalize(d)
	}
	return ""
}

// FindApproversForPath returns the OWNERS file path furthest down the tree for a specified file
// that contains an approvers section
func (o *RepoInfo) FindApproverOwnersForPath(path string) string {
	return findOwnersForPath(path, o.approvers)
}

// FindReviewersForPath returns the OWNERS file path furthest down the tree for a specified file
// that contains a reviewers section
func (o *RepoInfo) FindReviewersForPath(path string) string {
	return findOwnersForPath(path, o.reviewers)
}

// AllPossibleOwnerLabels returns all labels found in any owners files
func (o *RepoInfo) AllPossibleOwnerLabels() sets.String {
	return sets.NewString(o.allPossibleLabels.List()...)
}

// FindLabelsForPath returns a set of labels which should be applied to PRs
// modifying files under the given path.
func (o *RepoInfo) FindLabelsForPath(path string) sets.String {
	s := sets.String{}

	d := path
	for {
		l, ok := o.labels[d]
		if ok && len(l) > 0 {
			s = s.Union(l)
		}
		if d == baseDirConvention {
			break
		}
		d = filepath.Dir(d)
		d = canonicalize(d)
	}
	return s
}

// peopleForPath returns a set of users who are assignees to the
// requested file. The path variable should be a full path to a filename
// and not directory as the final directory will be discounted if enableMdYaml is true
// leafOnly indicates whether only the OWNERS deepest in the tree (closest to the file)
// should be returned or if all OWNERS in filepath should be returned
func peopleForPath(path string, people map[string]sets.String, leafOnly bool, enableMdYaml bool) sets.String {
	d := path
	if !enableMdYaml {
		// if path is a directory, this will remove the leaf directory, and returns "." for topmost dir
		d = filepath.Dir(d)
		d = canonicalize(path)
	}

	out := sets.NewString()
	for {
		s, ok := people[d]
		if ok {
			out = out.Union(s)
			if leafOnly && out.Len() > 0 {
				break
			}
		}
		if d == baseDirConvention {
			break
		}
		d = filepath.Dir(d)
		d = canonicalize(d)
	}
	return out
}

// LeafApprovers returns a set of users who are the closest approvers to the
// requested file. If pkg/OWNERS has user1 and pkg/util/OWNERS has user2 this
// will only return user2 for the path pkg/util/sets/file.go
func (o *RepoInfo) LeafApprovers(path string) sets.String {
	return peopleForPath(path, o.approvers, true, o.enableMdYaml)
}

// Approvers returns ALL of the users who are approvers for the
// requested file (including approvers in parent dirs' OWNERS).
// If pkg/OWNERS has user1 and pkg/util/OWNERS has user2 this
// will return both user1 and user2 for the path pkg/util/sets/file.go
func (o *RepoInfo) Approvers(path string) sets.String {
	return peopleForPath(path, o.approvers, false, o.enableMdYaml)
}

// LeafReviewers returns a set of users who are the closest reviewers to the
// requested file. If pkg/OWNERS has user1 and pkg/util/OWNERS has user2 this
// will only return user2 for the path pkg/util/sets/file.go
func (o *RepoInfo) LeafReviewers(path string) sets.String {
	if !o.useReviewers {
		return o.LeafApprovers(path)
	}
	return peopleForPath(path, o.reviewers, true, o.enableMdYaml)
}

// Reviewers returns ALL of the users who are reviewers for the
// requested file (including reviewers in parent dirs' OWNERS).
// If pkg/OWNERS has user1 and pkg/util/OWNERS has user2 this
// will return both user1 and user2 for the path pkg/util/sets/file.go
func (o *RepoInfo) Reviewers(path string) sets.String {
	if !o.useReviewers {
		return o.Approvers(path)
	}
	return peopleForPath(path, o.reviewers, false, o.enableMdYaml)
}
