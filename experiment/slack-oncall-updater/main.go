/*
Copyright 2019 The Kubernetes Authors.

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
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
)

func getJSON(url string, v interface{}) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch %q: %v", url, err)
	}
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(v)
	if err != nil {
		return fmt.Errorf("failed to decode json from %q: %v", url, err)
	}
	return nil
}

func getCurrentOncaller() (string, error) {
	oncaller := struct {
		OnCall struct {
			TestInfra string `json:"testinfra"`
		} `json:"Oncall"`
	}{}

	err := getJSON("https://storage.googleapis.com/kubernetes-jenkins/oncall.json", &oncaller)
	if err != nil {
		return "", err
	}

	return oncaller.OnCall.TestInfra, nil
}

func mapGitHubToSlack(github string) (string, error) {
	// Maps (lowercase!) GitHub usernames to Slack user IDs.
	// Add yourself when you join the oncall rotation.
	// You can mostly easily find your user ID by visiting your Slack profile, clicking "...",
	// and clicking "Copy user ID".
	mapping := map[string]string{
		"amwat":          "U9B1P2UGP",
		"chases2":        "UJ9R0FWD6",
		"cjwagner":       "U4QFZFMCM",
		"clarketm":       "UKMR3JF29",
		"bentheelder":    "U1P7T516X",
		"fejta":          "U0E2KHQ13",
		"katharine":      "UBTBNJ6GL",
		"krzyzacy":       "U22Q65CTG",
		"mirandachrist":  "UJR3XHHNF",
		"michelle192837": "U3TRY5WV7",
		"tony-yang":      "UK3MVSP3J",
	}
	id, ok := mapping[strings.ToLower(github)]
	if !ok {
		return "", fmt.Errorf("couldn't find a Slack ID for GitHub user %q", github)
	}
	return id, nil
}

func updateGroupMembership(token, groupID, userID string) error {
	result := struct {
		Ok    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}{}

	err := getJSON("https://slack.com/api/usergroups.users.update?token="+token+"&users="+userID+"&usergroup="+groupID, &result)
	if err != nil {
		return fmt.Errorf("couldn't make membership request: %v", err)
	}

	if !result.Ok {
		return fmt.Errorf("couldn't update membership: %s", result.Error)
	}

	return nil
}

func getTokenFromPath(path string) (string, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("couldn't open file: %v", err)
	}
	return strings.TrimSpace(string(content)), nil
}

type options struct {
	tokenPath string
	groupID   string
}

func parseFlags() options {
	o := options{}
	flag.StringVar(&o.tokenPath, "token-path", "/etc/slack-token", "Path to a file containing the slack token")
	flag.StringVar(&o.groupID, "group-id", "", "Slack group ID")
	flag.Parse()
	return o
}

func main() {
	o := parseFlags()

	token, err := getTokenFromPath(o.tokenPath)
	if err != nil {
		log.Fatalf("Failed to get Slack token: %v", err)
	}

	oncall, err := getCurrentOncaller()
	if err != nil {
		log.Fatalf("Failed to find current oncaller: %v", err)
	}
	log.Printf("Current oncaller: %s\n", oncall)

	userID, err := mapGitHubToSlack(oncall)
	if err != nil {
		log.Fatalf("Failed to get Slack ID: %v", err)
	}
	log.Printf("%s's slack ID: %s\n", oncall, userID)

	log.Printf("Adding slack user %s to slack usergroup %s", userID, o.groupID)
	if err := updateGroupMembership(token, o.groupID, userID); err != nil {
		log.Fatalf("Failed to update usergroup membership: %v", err)
	}
	log.Printf("Done!")
}
