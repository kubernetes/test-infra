package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/cmd/generic-autobumper/bumper"
)

const (
	UUID                = 0
	GROUP_NAME          = 1
	GROUPS_FILE         = "groups"
	PROJECT_CONFIG_FILE = "project.config"

	ACCESS_HEADER            = "[access \"refs/*\"]"
	PROW_READ_ACCESS_FORMAT  = "read = group %s"
	PROW_LABEL_ACCESS_FORMAT = "label-Verified = -1..+1 group %s"
	LABEL_HEADER             = "[label \"Verified\"]"
)

type Options struct {
	Host       string
	Repo       string
	UUID       string
	GroupName  string
	BranchName string
}

func getAllLabelLines() []string {
	return []string{
		"function = MaxWithBlock",
		"value = -1 Failed",
		"value = 0 No score",
		"value = +1 Verified",
		"copyAllScoresIfNoCodeChange = true",
		"defaultValue = 0",
	}
}

func parseAndValidateOptions() (*Options, error) {
	var o Options
	flag.StringVar(&o.Host, "host", "", "The gerrit host.")
	flag.StringVar(&o.Repo, "repo", "", "The gerrit Repo.")
	flag.StringVar(&o.UUID, "uuid", "", "The UUID to be added to the file.")
	flag.StringVar(&o.GroupName, "group", "", "The corresponding group name for the UUID.")
	flag.StringVar(&o.BranchName, "branch", "", "The name of the branch where refs/meta/config will live")
	flag.Parse()

	if o.Host == "" || o.Repo == "" || o.UUID == "" || o.GroupName == "" || o.BranchName == "" {
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
	groups := fmt.Sprintf(getFormatString(maxLine), "# UUID", "Group Name") + "#\n"

	for _, id := range orderedUUIDs {
		groups = groups + fmt.Sprintf(getFormatString(maxLine), id, groupsMap[id])
	}
	return groups
}

func groupsToMap(groupsFile string) (map[string]string, []string) {
	orderedKeys := []string{}
	groupsMap := map[string]string{}
	lines := strings.Split(groupsFile, "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, "#") && line != "" {
			pair := strings.Split(line, "\t")
			orderedKeys = append(orderedKeys, strings.TrimSpace(pair[UUID]))
			groupsMap[strings.TrimSpace(pair[UUID])] = strings.TrimSpace(pair[GROUP_NAME])
		}
	}
	return groupsMap, orderedKeys
}

func ensureUUID(groupsFile, id, group string) string {
	groupsMap, orderedKeys := groupsToMap(groupsFile)
	if value, ok := groupsMap[id]; ok && group == value {
		return groupsFile
	}
	groupsMap[id] = group
	orderedKeys = append(orderedKeys, id)
	return mapToGroups(groupsMap, orderedKeys)
}

func updateGroups(uuid, group string) (bool, error) {
	data, err := ioutil.ReadFile(GROUPS_FILE)
	if err != nil {
		return false, fmt.Errorf("faield to read groups file: %w", err)
	}

	newData := ensureUUID(string(data), uuid, group)
	err = ioutil.WriteFile(GROUPS_FILE, []byte(newData), 0755)
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
	_, ok := configMap[LABEL_HEADER]
	return ok
}

func lineInRightHeaderFunc(header, line string) func(map[string][]string) bool {
	return func(configMap map[string][]string) bool {
		if value, ok := configMap[header]; ok && contains(value, line) {
			return true
		}
		return false
	}
}

func labelAccessExistsFunc(groupName string) func(map[string][]string) bool {
	return func(configMap map[string][]string) bool {
		for _, value := range configMap {
			for _, item := range value {
				if strings.HasPrefix(strings.TrimSpace(item), "label-Verified =") && strings.HasSuffix(strings.TrimSpace(item), fmt.Sprintf("group %s", groupName)) {
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
		data, err := ioutil.ReadFile(PROJECT_CONFIG_FILE)
		if err != nil {
			return false, fmt.Errorf("faield to read project.config file: %w", err)
		}
		newConfig, _ := configToMap(string(data))
		ret, err := verifyInTree(host, parent_branch, newConfig, verify)
		if err != nil {
			return false, fmt.Errorf("faield to check if lines in config for %s/%s: %w", host, inheritance, err)
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
	readAccessLine := fmt.Sprintf(PROW_READ_ACCESS_FORMAT, groupName)
	prowReadAccess, err := verifyInTree(host, cur_branch, configMap, lineInRightHeaderFunc(ACCESS_HEADER, readAccessLine))
	if err != nil {
		return "", fmt.Errorf("faield to check if needed lines in config: %w", err)
	}
	if !prowReadAccess {
		accessLines = append(accessLines, readAccessLine)
	}

	// Check that the line "label-verified" = ... group GROUPNAME exists under ANY header
	labelAccessLine := fmt.Sprintf(PROW_LABEL_ACCESS_FORMAT, groupName)
	prowLabelAccess, err := verifyInTree(host, cur_branch, configMap, labelAccessExistsFunc(groupName))
	if err != nil {
		return "", fmt.Errorf("faield to check if needed lines in config: %w", err)
	}
	if !prowLabelAccess {
		accessLines = append(accessLines, labelAccessLine)
	}
	configMap, orderedKeys = addSection(ACCESS_HEADER, configMap, orderedKeys, accessLines)

	// We need to be less exact with the Label-Verified header so we are just checking if it exists anywhere:
	labelExists, err := verifyInTree(host, cur_branch, configMap, labelExists)
	if err != nil {
		return "", fmt.Errorf("faield to check if needed lines in config: %w", err)
	}
	if !labelExists {
		configMap, orderedKeys = addSection(LABEL_HEADER, configMap, orderedKeys, getAllLabelLines())
	}
	return mapToConfig(configMap, orderedKeys), nil

}

func updatePojectConfig(host, cur_branch, groupName string) error {
	data, err := ioutil.ReadFile(PROJECT_CONFIG_FILE)
	if err != nil {
		return fmt.Errorf("faield to read project.config file: %w", err)
	}

	newData, err := ensureProjectConfig(string(data), host, cur_branch, groupName)
	if err != nil {
		return fmt.Errorf("failed to ensure updated project config: %w", err)
	}
	err = ioutil.WriteFile(PROJECT_CONFIG_FILE, []byte(newData), 0755)
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
		logrus.WithError(err).Fatal("Failed to run onboarding tool")
	}

	if err = fetchMetaConfig(o.Host, o.Repo, o.BranchName); err != nil {
		logrus.WithError(err).Fatal("Failed to run onboarding tool")
	}

	groupsChanged, err := updateGroups(o.UUID, o.GroupName)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to run onboarding tool")
	}

	if groupsChanged {
		logrus.Info("groups file did not change. Skipping stash step")
		if err := bumper.Call(os.Stdout, os.Stderr, "git", "stash"); err != nil {
			logrus.WithError(err).Fatal("Failed to run onboarding tool")
		}
	}

	if err = updatePojectConfig(o.Host, o.BranchName, o.GroupName); err != nil {
		logrus.WithError(err).Fatal("Failed to run onboarding tool")
	}

	if groupsChanged {
		if err := bumper.Call(os.Stdout, os.Stderr, "git", "stash", "apply"); err != nil {
			logrus.WithError(err).Fatal("Failed to run onboarding tool")
		}
	}

	//Create PR

}
