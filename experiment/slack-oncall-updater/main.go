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

// githubToSlack maps (lowercase!) GitHub usernames to Slack user IDs.
// Add yourself when you join the oncall rotation.
// You can mostly easily find your user ID by visiting your Slack profile, clicking "...",
// and clicking "Copy user ID".
var githubToSlack = map[string]string{
	"amwat":          "U9B1P2UGP",
	"bentheelder":    "U1P7T516X",
	"chaodaig":       "U010XUQ9VPE",
	"chases2":        "UJ9R0FWD6",
	"cjwagner":       "U4QFZFMCM",
	"e-blackwelder":  "U011FF4QHAN",
	"fejta":          "U0E2KHQ13",
	"katharine":      "UBTBNJ6GL",
	"michelle192837": "U3TRY5WV7",
	"mushuee":        "U013TPFJWC8",
	"spiffxp":        "U09R2FL93",
}

// rotationToSlack group maps the rotation in go.k8s.io/oncall to the slack
// group ID for the oncall alias group for this rotation
var rotationToSlackGroup = map[string]string{
	"testinfra":          "SGLF0GUQH",   // @test-infra-oncall
	"google-build-admin": "S017N31TLNN", // @google-build-admin
	// TODO: create this slack group?
	// NOTE: If anyone adds this group, you must also send a PR like:
	// https://github.com/kubernetes-sigs/slack-infra/pull/36
	// "scalability": "",
}

func getJSON(url string, v interface{}) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch %q: %v", url, err)
	}
	defer resp.Body.Close()
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(v)
	if err != nil {
		return fmt.Errorf("failed to decode json from %q: %v", url, err)
	}
	return nil
}

func getCurrentOncallers() (map[string]string, error) {
	oncallBlob := struct {
		Oncall map[string]string `json:"Oncall"`
	}{}

	err := getJSON("https://storage.googleapis.com/kubernetes-jenkins/oncall.json", &oncallBlob)
	if err != nil {
		return nil, err
	}

	return oncallBlob.Oncall, nil
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
}

func parseFlags() options {
	o := options{}
	flag.StringVar(&o.tokenPath, "token-path", "/etc/slack-token", "Path to a file containing the slack token")
	flag.Parse()
	return o
}

func main() {
	o := parseFlags()

	token, err := getTokenFromPath(o.tokenPath)
	if err != nil {
		log.Fatalf("Failed to get Slack token: %v", err)
	}

	oncallers, err := getCurrentOncallers()
	if err != nil {
		log.Fatalf("Failed to find current oncallers: %v", err)
	}
	log.Printf("Current oncallers: %s\n", oncallers)

	failed := false
	for rotation, user := range oncallers {
		// skip rotations that have not yet added their slack ID
		// this tool is not required
		groupID, exists := rotationToSlackGroup[rotation]
		if !exists {
			log.Printf("Rotation %q does not yet have a Group ID, skipping ...", rotation)
			continue
		}

		// get the slack ID for the current oncall in this rotation
		userID, exists := githubToSlack[strings.ToLower(user)]
		if !exists {
			log.Printf("Failed to get Slack ID for GitHub user %q", user)
			// continue with other rotations, but exit error afterwards
			failed = true
			continue
		}
		log.Printf("%s's slack ID: %s\n", user, userID)

		// update this rotation's slack instance
		log.Printf("Adding slack user %s to slack usergroup %s", userID, groupID)
		if err := updateGroupMembership(token, groupID, userID); err != nil {
			log.Fatalf("Failed to update usergroup membership: %v", err)
		}
	}
	// if one of the group updates failed, fail the run now
	if failed {
		log.Fatal("Failed to update some rotation(s)")
	}

	log.Printf("Done!")
}
