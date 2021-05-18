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
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/cmd/generic-autobumper/bumper"
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
}

func parseAndValidateOptions() (*options, error) {
	var o options
	flag.StringVar(&o.host, "host", "", "The gerrit host.")
	flag.StringVar(&o.repo, "repo", "", "The gerrit Repo.")
	flag.StringVar(&o.uuID, "uuid", "", "The UUID to be added to the file.")
	flag.StringVar(&o.groupName, "group", "", "The corresponding group name for the UUID.")
	flag.Parse()

	if o.host == "" || o.repo == "" || o.uuID == "" || o.groupName == "" {
		return &o, errors.New("all flags are required")
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

func updateGroups(uuid, group string) (bool, error) {
	data, err := ioutil.ReadFile(groupsFile)
	if err != nil {
		return false, fmt.Errorf("faield to read groups file: %w", err)
	}

	newData, err := ensureUUID(string(data), uuid, group)
	if err != nil {
		return false, fmt.Errorf("faield to ensure group exists: %w", err)
	}

	err = ioutil.WriteFile(groupsFile, []byte(newData), 0755)
	if err != nil {
		return false, fmt.Errorf("faield to write groups file: %w", err)
	}

	return newData != string(data), nil
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
		} else {
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

func verifyInTree(host, cur_branch string, configMap map[string][]string, verify func(map[string][]string) bool) (bool, error) {
	if verify(configMap) {
		return true, nil
	} else if inheritance := getInheritedRepo(configMap); inheritance != "" {
		parent_branch := cur_branch + "_parent"
		if err := fetchMetaConfig(host, inheritance, parent_branch); err != nil {
			return false, fmt.Errorf("unable to fetch refs/meta/config for %s: %w", inheritance, err)
		}
		data, err := ioutil.ReadFile(projectConfigFile)
		if err != nil {
			return false, fmt.Errorf("failed to read project.config file: %w", err)
		}
		newConfig, _ := configToMap(string(data))
		ret, err := verifyInTree(host, parent_branch, newConfig, verify)
		if err != nil {
			return false, fmt.Errorf("failed to check if lines in config for %s/%s: %w", host, inheritance, err)
		}
		if err := bumper.Call(os.Stdout, os.Stderr, "git", "checkout", cur_branch); err != nil {
			return false, fmt.Errorf("failed to checkout %s, %w", cur_branch, err)
		}
		if err := bumper.Call(os.Stdout, os.Stderr, "git", "branch", "-D", parent_branch); err != nil {
			return false, fmt.Errorf("failed to delete %s branch, %w", parent_branch, err)
		}
		return ret, nil
	}
	return false, nil
}

func ensureProjectConfig(config, host, cur_branch, groupName string) (string, error) {
	configMap, orderedKeys := configToMap(config)

	// Check that prow automation robot has access to refs/*
	accessLines := []string{}
	readAccessLine := fmt.Sprintf(prowReadAccessFormat, groupName)
	prowReadAccess, err := verifyInTree(host, cur_branch, configMap, lineInMatchingHeaderFunc(accessRefsRegex, readAccessLine))
	if err != nil {
		return "", fmt.Errorf("faield to check if needed lines in config: %w", err)
	}
	if !prowReadAccess {
		accessLines = append(accessLines, readAccessLine)
	}

	// Check that the line "label-verified" = ... group GROUPNAME exists under ANY header
	labelAccessLine := fmt.Sprintf(prowLabelAccessFormat, groupName)
	prowLabelAccess, err := verifyInTree(host, cur_branch, configMap, labelAccessExistsFunc(groupName))
	if err != nil {
		return "", fmt.Errorf("faield to check if needed lines in config: %w", err)
	}
	if !prowLabelAccess {
		accessLines = append(accessLines, labelAccessLine)
	}
	configMap, orderedKeys = addSection(accessHeader, configMap, orderedKeys, accessLines)

	// We need to be less exact with the Label-Verified header so we are just checking if it exists anywhere:
	labelExists, err := verifyInTree(host, cur_branch, configMap, labelExists)
	if err != nil {
		return "", fmt.Errorf("faield to check if needed lines in config: %w", err)
	}
	if !labelExists {
		configMap, orderedKeys = addSection(labelHeader, configMap, orderedKeys, labelLines)
	}
	return mapToConfig(configMap, orderedKeys), nil

}

func updatePojectConfig(host, cur_branch, groupName string) error {
	data, err := ioutil.ReadFile(projectConfigFile)
	if err != nil {
		return fmt.Errorf("faield to read project.config file: %w", err)
	}

	newData, err := ensureProjectConfig(string(data), host, cur_branch, groupName)
	if err != nil {
		return fmt.Errorf("failed to ensure updated project config: %w", err)
	}
	err = ioutil.WriteFile(projectConfigFile, []byte(newData), 0755)
	if err != nil {
		return fmt.Errorf("faield to write groups file: %w", err)
	}

	return nil
}

func fetchMetaConfig(host, repo, branch string) error {
	if err := bumper.Call(os.Stdout, os.Stderr, "git", "fetch", fmt.Sprintf("sso://%s/%s", host, repo), "refs/meta/config"); err != nil {
		return fmt.Errorf("failed to fetch refs/meta/config, %w", err)
	}
	if err := bumper.Call(os.Stdout, os.Stderr, "git", "checkout", "FETCH_HEAD"); err != nil {
		return fmt.Errorf("failed to checkout FETCH_HEAD, %w", err)
	}
	if err := bumper.Call(os.Stdout, os.Stderr, "git", "switch", "-c", branch); err != nil {
		return fmt.Errorf("failed to switch to new branch, %w", err)
	}

	return nil
}

func main() {
	o, err := parseAndValidateOptions()
	if err != nil {
		logrus.Fatal(err)
	}

	rand.Seed(time.Now().UTC().UnixNano())
	branchName := fmt.Sprintf("gerritOnboarding_%d", rand.Int())

	if err = fetchMetaConfig(o.host, o.repo, branchName); err != nil {
		logrus.Fatal(err)
	}

	groupsChanged, err := updateGroups(o.uuID, o.groupName)
	if err != nil {
		logrus.Fatal(err)
	}

	if groupsChanged {
		logrus.Info("groups file did not change. Skipping stash step")
		if err := bumper.Call(os.Stdout, os.Stderr, "git", "stash"); err != nil {
			logrus.Fatal(err)
		}
	}

	if err = updatePojectConfig(o.host, branchName, o.groupName); err != nil {
		logrus.Fatal(err)
	}

	if groupsChanged {
		if err := bumper.Call(os.Stdout, os.Stderr, "git", "stash", "apply"); err != nil {
			logrus.Fatal(err)
		}
	}

	//Create PR

}
