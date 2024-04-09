/*
Copyright 2021 The Kubernetes Authors.

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
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"

	"sigs.k8s.io/prow/prow/cmd/generic-autobumper/bumper"
)

const (
	uuID              = 0
	groupName         = 1
	groupsFile        = "groups"
	projectConfigFile = "project.config"

	accessHeader          = `[access "refs/*"]`
	prowReadAccessFormat  = "read = group %s"
	prowLabelAccessFormat = "label-Verified = -1..+1 group %s"
	labelHeader           = `[label "Verified"]`
	labelEquals           = "label-Verified ="
)

var (
	labelLines = []string{
		"function = MaxWithBlock",
		"value = -1 Failed",
		"value = 0 No score",
		"value = +1 Verified",
		"copyAllScoresIfNoCodeChange = true",
		"defaultValue = 0",
	}

	accessRefsRegex = regexp.MustCompile(`^\[access "refs\/.*"\]`)
)

type options struct {
	host      string
	repo      string
	uuID      string
	groupName string
	dryRun    bool
	local     bool
}

func parseAndValidateOptions() (*options, error) {
	var o options
	flag.StringVar(&o.host, "host", "", "The gerrit host.")
	flag.StringVar(&o.repo, "repo", "", "The gerrit Repo.")
	flag.StringVar(&o.uuID, "uuid", "", "The UUID to be added to the file.")
	flag.StringVar(&o.groupName, "group", "", "The corresponding group name for the UUID.")
	flag.BoolVar(&o.dryRun, "dry_run", false, "If dry_run is true, PR will not be created")
	flag.BoolVar(&o.local, "local", false, "If local is true, changes will be made to local repo instead of new temp dir.")
	flag.Parse()

	if o.host == "" || o.repo == "" || o.uuID == "" || o.groupName == "" {
		return &o, errors.New("host, repo, uuid, and group are all required fields")
	}

	return &o, nil
}

func intMax(x, y int) int {
	if x > y {
		return x
	}
	return y
}

func maxIDLen(values []string) int {
	max := 0
	for _, item := range values {
		max = intMax(max, len(item))
	}
	return intMax(max, len("# UUID"))
}

func getFormatString(maxLine int) string {
	return "%-" + fmt.Sprintf("%d", maxLine) + "v\t%s\n"
}

func mapToGroups(groupsMap map[string]string, orderedUUIDs []string) string {
	maxLine := maxIDLen(orderedUUIDs)
	groups := fmt.Sprintf(getFormatString(maxLine), "# UUID", "Group Name")

	for _, id := range orderedUUIDs {
		if strings.HasPrefix(id, "#") {
			groups = groups + id + "\n"
		} else {
			groups = groups + fmt.Sprintf(getFormatString(maxLine), id, groupsMap[id])
		}
	}
	return groups
}

func groupsToMap(groupsFile string) (map[string]string, []string) {
	orderedKeys := []string{}
	groupsMap := map[string]string{}
	lines := strings.Split(groupsFile, "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, "# UUID") && line != "" {
			if strings.HasPrefix(line, "#") {
				orderedKeys = append(orderedKeys, line)
			} else {
				pair := strings.Split(line, "\t")
				orderedKeys = append(orderedKeys, strings.TrimSpace(pair[uuID]))
				groupsMap[strings.TrimSpace(pair[uuID])] = strings.TrimSpace(pair[groupName])
			}

		}
	}
	return groupsMap, orderedKeys
}

func ensureUUID(groupsFile, uuid, group string) (string, error) {
	groupsMap, orderedKeys := groupsToMap(groupsFile)

	// Group already exists
	if value, ok := groupsMap[uuid]; ok && group == value {
		return groupsFile, nil
	}
	// UUID already exists with different group
	if value, ok := groupsMap[uuid]; ok && group != value {
		return "", fmt.Errorf("UUID, %s, already in use for group %s", uuid, value)
	}
	// Group name already in use with different UUID
	for cur_id, groupName := range groupsMap {
		if groupName == group {
			return "", fmt.Errorf("%s already used as group name for %s", group, cur_id)
		}
	}

	groupsMap[uuid] = group
	orderedKeys = append(orderedKeys, uuid)
	return mapToGroups(groupsMap, orderedKeys), nil
}

func updateGroups(workDir, uuid, group string) error {
	data, err := os.ReadFile(path.Join(workDir, groupsFile))
	if err != nil {
		return fmt.Errorf("failed to read groups file: %w", err)
	}

	newData, err := ensureUUID(string(data), uuid, group)
	if err != nil {
		return fmt.Errorf("failed to ensure group exists: %w", err)
	}

	err = os.WriteFile(path.Join(workDir, groupsFile), []byte(newData), 0755)
	if err != nil {
		return fmt.Errorf("failed to write groups file: %w", err)
	}

	return nil
}

func configToMap(configFile string) (map[string][]string, []string) {
	configMap := map[string][]string{}
	orderedKeys := []string{}
	var curKey string
	if configFile == "" {
		return configMap, orderedKeys
	}
	for _, line := range strings.Split(configFile, "\n") {
		if strings.HasPrefix(line, "[") {
			curKey = line
			orderedKeys = append(orderedKeys, line)
		} else if line != "" {
			if curList, ok := configMap[curKey]; ok {
				configMap[curKey] = append(curList, line)
			} else {
				configMap[curKey] = []string{line}
			}
		}
	}
	return configMap, orderedKeys
}

func mapToConfig(configMap map[string][]string, orderedIDs []string) string {
	res := ""
	for _, header := range orderedIDs {
		res = res + header + "\n"
		for _, line := range configMap[header] {
			res = res + line + "\n"
		}
	}
	if res == "" {
		return res
	}
	return strings.TrimSpace(res) + "\n"
}

func contains(s []string, v string) bool {
	for _, item := range s {
		if strings.TrimSpace(item) == strings.TrimSpace(v) {
			return true
		}
	}
	return false
}

func getInheritedRepo(configMap map[string][]string) string {
	if section, ok := configMap["[access]"]; ok {
		for _, line := range section {
			if strings.Contains(line, "inheritFrom") {
				return strings.TrimSpace(strings.Split(line, "=")[1])
			}
		}
	}
	return ""
}

func addSection(header string, configMap map[string][]string, configOrder, neededLines []string) (map[string][]string, []string) {
	if _, ok := configMap[header]; !ok {
		configMap[header] = []string{}
		configOrder = append(configOrder, header)
	}
	for _, line := range neededLines {
		configMap[header] = append(configMap[header], "\t"+line)
	}

	return configMap, configOrder
}

func labelExists(configMap map[string][]string) bool {
	_, ok := configMap[labelHeader]
	return ok
}

func lineInMatchingHeaderFunc(regex *regexp.Regexp, line string) func(map[string][]string) bool {
	return func(configMap map[string][]string) bool {
		for header, lines := range configMap {
			match := regex.MatchString(header)
			if match {
				if contains(lines, line) {
					return true
				}
			}
		}
		return false
	}
}

// returns a function that checks if a line exists anywhere in the config that sets sets "label-Verified" = to some values for the given group Name
// this is a best-attempt at checking if the group is given access to the label in as unitrusive way.
func labelAccessExistsFunc(groupName string) func(map[string][]string) bool {
	return func(configMap map[string][]string) bool {
		for _, value := range configMap {
			for _, item := range value {
				if strings.HasPrefix(strings.TrimSpace(item), labelEquals) && strings.HasSuffix(strings.TrimSpace(item), fmt.Sprintf("group %s", groupName)) {
					return true
				}
			}
		}
		return false
	}
}

func verifyInTree(workDir, host, cur_branch string, configMap map[string][]string, verify func(map[string][]string) bool) (bool, error) {
	if verify(configMap) {
		return true, nil
	} else if inheritance := getInheritedRepo(configMap); inheritance != "" {
		parent_branch := cur_branch + "_parent"
		if err := fetchMetaConfig(host, inheritance, parent_branch, workDir); err != nil {
			// This likely won't happen, but if the fail is due to switching branches, we want to fail
			if strings.Contains(err.Error(), "failed to switch") {
				return false, fmt.Errorf("unable to fetch refs/meta/config for %s: %w", inheritance, err)
			}
			// If it failed to fetch refs/meta/config for parent, or checkout the FETCH_HEAD, just catch the error and return False
			return false, nil
		}
		data, err := os.ReadFile(path.Join(workDir, projectConfigFile))
		if err != nil {
			return false, fmt.Errorf("failed to read project.config file: %w", err)
		}
		newConfig, _ := configToMap(string(data))
		ret, err := verifyInTree(workDir, host, parent_branch, newConfig, verify)
		if err != nil {
			return false, fmt.Errorf("failed to check if lines in config for %s/%s: %w", host, inheritance, err)
		}
		if err := execInDir(os.Stdout, os.Stderr, workDir, "git", "checkout", cur_branch); err != nil {
			return false, fmt.Errorf("failed to checkout %s, %w", cur_branch, err)
		}
		if err := execInDir(os.Stdout, os.Stderr, workDir, "git", "branch", "-D", parent_branch); err != nil {
			return false, fmt.Errorf("failed to delete %s branch, %w", parent_branch, err)
		}
		return ret, nil
	}
	return false, nil
}

func ensureProjectConfig(workDir, config, host, cur_branch, groupName string) (string, error) {
	configMap, orderedKeys := configToMap(config)

	// Check that prow automation robot has access to refs/*
	accessLines := []string{}
	readAccessLine := fmt.Sprintf(prowReadAccessFormat, groupName)
	prowReadAccess, err := verifyInTree(workDir, host, cur_branch, configMap, lineInMatchingHeaderFunc(accessRefsRegex, readAccessLine))
	if err != nil {
		return "", fmt.Errorf("failed to check if needed lines in config: %w", err)
	}
	if !prowReadAccess {
		accessLines = append(accessLines, readAccessLine)
	}

	// Check that the line "label-verified" = ... group GROUPNAME exists under ANY header
	labelAccessLine := fmt.Sprintf(prowLabelAccessFormat, groupName)
	prowLabelAccess, err := verifyInTree(workDir, host, cur_branch, configMap, labelAccessExistsFunc(groupName))
	if err != nil {
		return "", fmt.Errorf("failed to check if needed lines in config: %w", err)
	}
	if !prowLabelAccess {
		accessLines = append(accessLines, labelAccessLine)
	}
	configMap, orderedKeys = addSection(accessHeader, configMap, orderedKeys, accessLines)

	// We need to be less exact with the Label-Verified header so we are just checking if it exists anywhere:
	labelExists, err := verifyInTree(workDir, host, cur_branch, configMap, labelExists)
	if err != nil {
		return "", fmt.Errorf("failed to check if needed lines in config: %w", err)
	}
	if !labelExists {
		configMap, orderedKeys = addSection(labelHeader, configMap, orderedKeys, labelLines)
	}
	return mapToConfig(configMap, orderedKeys), nil

}

func updatePojectConfig(workDir, host, cur_branch, groupName string) error {
	data, err := os.ReadFile(path.Join(workDir, projectConfigFile))
	if err != nil {
		return fmt.Errorf("failed to read project.config file: %w", err)
	}

	newData, err := ensureProjectConfig(workDir, string(data), host, cur_branch, groupName)
	if err != nil {
		return fmt.Errorf("failed to ensure updated project config: %w", err)
	}
	err = os.WriteFile(path.Join(workDir, projectConfigFile), []byte(newData), 0755)
	if err != nil {
		return fmt.Errorf("failed to write groups file: %w", err)
	}

	return nil
}

func fetchMetaConfig(host, repo, branch, workDir string) error {
	if err := execInDir(os.Stdout, os.Stderr, workDir, "git", "fetch", fmt.Sprintf("sso://%s/%s", host, repo), "refs/meta/config"); err != nil {
		return fmt.Errorf("failed to fetch refs/meta/config, %w", err)
	}
	if err := execInDir(os.Stdout, os.Stderr, workDir, "git", "checkout", "FETCH_HEAD"); err != nil {
		return fmt.Errorf("failed to checkout FETCH_HEAD, %w", err)
	}
	if err := execInDir(os.Stdout, os.Stderr, workDir, "git", "switch", "-c", branch); err != nil {
		return fmt.Errorf("failed to switch to new branch, %w", err)
	}

	return nil
}

func execInDir(stdout, stderr io.Writer, dir string, cmd string, args ...string) error {
	(&logrus.Logger{
		Out:       os.Stderr,
		Formatter: logrus.StandardLogger().Formatter,
		Hooks:     logrus.StandardLogger().Hooks,
		Level:     logrus.StandardLogger().Level,
	}).WithField("dir", dir).
		WithField("cmd", cmd).
		// The default formatting uses a space as separator, which is hard to read if an arg contains a space
		WithField("args", fmt.Sprintf("['%s']", strings.Join(args, "', '"))).
		Info("running command")

	c := exec.Command(cmd, args...)
	c.Dir = dir
	c.Stdout = stdout
	c.Stderr = stderr
	return c.Run()
}

func createCR(workDir string, dryRun bool) error {
	diff, err := getDiff(workDir)
	if err != nil {
		return err
	}
	if diff == "" {
		logrus.Info("No changes made. Returning without creating CR")
		return nil
	}
	commitMessage := fmt.Sprintf("Grant the Prow cluster read and label permissions\n\nChange-Id: I%s", bumper.GitHash(fmt.Sprintf("%d", rand.Int())))
	if err := execInDir(os.Stdout, os.Stderr, workDir, "git", "commit", "-a", "-v", "-m", commitMessage); err != nil {
		return fmt.Errorf("unable to commit: %w", err)
	}
	if !dryRun {
		if err := execInDir(os.Stdout, os.Stderr, workDir, "git", "push", "origin", "HEAD:refs/for/refs/meta/config"); err != nil {
			return fmt.Errorf("unable to push: %w", err)
		}
	}
	return nil
}

func getDiff(workDir string) (string, error) {
	var diffBuf bytes.Buffer
	var errBuf bytes.Buffer
	if err := execInDir(&diffBuf, &errBuf, workDir, "git", "diff"); err != nil {
		return "", fmt.Errorf("diffing previous bump: %v -- %s", err, errBuf.String())
	}
	return diffBuf.String(), nil
}

func getRepoClonedName(repo string) string {
	lst := strings.Split(repo, "/")
	return lst[len(lst)-1]
}

func main() {
	o, err := parseAndValidateOptions()
	if err != nil {
		logrus.Fatal(err)
	}

	var workDir string
	if o.local {
		workDir, err = os.Getwd()
		if err != nil {
			logrus.Fatal(err)
		}
	} else {
		workDir, err = os.MkdirTemp("", "gerrit_onboarding")
		if err != nil {
			logrus.Fatal(err)
		}
		defer os.RemoveAll(workDir)

		if err := execInDir(os.Stdout, os.Stderr, workDir, "git", "clone", fmt.Sprintf("sso://%s/%s", o.host, o.repo)); err != nil {
			logrus.Fatal(fmt.Errorf("failed to clone sso://%s/%s %w", o.host, o.repo, err))
		}

		workDir = path.Join(workDir, getRepoClonedName(o.repo))
	}

	branchName := fmt.Sprintf("gerritOnboarding_%d", rand.Int())

	if err = fetchMetaConfig(o.host, o.repo, branchName, workDir); err != nil {
		logrus.Fatal(err)
	}

	// It is important that we update projectConfig BEFORE we update groups, because updating
	// project config involves switching branches and we need to have no uncommitted changes to do that.
	if err = updatePojectConfig(workDir, o.host, branchName, o.groupName); err != nil {
		logrus.Fatal(err)
	}

	if err = updateGroups(workDir, o.uuID, o.groupName); err != nil {
		logrus.Fatal(err)
	}

	if err = createCR(workDir, o.dryRun); err != nil {
		logrus.Fatal(err)
	}

}
