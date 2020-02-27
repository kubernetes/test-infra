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
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

var httpTransport *http.Transport

func init() {
	httpTransport = new(http.Transport)
	httpTransport.Proxy = http.ProxyFromEnvironment
	httpTransport.RegisterProtocol("file", http.NewFileTransport(http.Dir("/")))
}

// Essentially curl url | writer including request headers
func httpReadWithHeaders(url string, headers map[string]string, writer io.Writer) error {
	log.Printf("curl %s", url)
	c := &http.Client{Transport: httpTransport}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	for k, v := range headers {
		req.Header.Add(k, v)
	}
	r, err := c.Do(req)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if r.StatusCode >= 400 {
		return fmt.Errorf("%v returned %d", url, r.StatusCode)
	}
	_, err = io.Copy(writer, r.Body)
	if err != nil {
		return err
	}
	return nil
}

// Essentially curl url | writer
func httpRead(url string, writer io.Writer) error {
	log.Printf("curl %s", url)
	c := &http.Client{Transport: httpTransport}
	r, err := c.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if r.StatusCode >= 400 {
		return fmt.Errorf("%v returned %d", url, r.StatusCode)
	}
	_, err = io.Copy(writer, r.Body)
	if err != nil {
		return err
	}
	return nil
}

type instanceGroup struct {
	Name              string `json:"name"`
	CreationTimestamp string `json:"creationTimestamp"`
}

// getLatestClusterUpTime returns latest created instanceGroup timestamp from gcloud parsing results
func getLatestClusterUpTime(gcloudJSON string) (time.Time, error) {
	igs := []instanceGroup{}
	if err := json.Unmarshal([]byte(gcloudJSON), &igs); err != nil {
		return time.Time{}, fmt.Errorf("error when unmarshal json: %v", err)
	}

	latest := time.Time{}

	for _, ig := range igs {
		created, err := time.Parse(time.RFC3339, ig.CreationTimestamp)
		if err != nil {
			return time.Time{}, fmt.Errorf("error when parse time from %s: %v", ig.CreationTimestamp, err)
		}

		if created.After(latest) {
			latest = created
		}
	}

	// this returns time.Time{} if no ig exists, which will always force a new cluster
	return latest, nil
}

// (only works on gke)
// getLatestGKEVersion will return newest validMasterVersions.
// Pass in releasePrefix to get latest valid version of a specific release.
// Empty releasePrefix means use latest across all available releases.
func getLatestGKEVersion(project, zone, region, releasePrefix string) (string, error) {
	cmd := []string{
		"container",
		"get-server-config",
		fmt.Sprintf("--project=%v", project),
		"--format=value(validMasterVersions)",
	}

	// --gkeCommandGroup is from gke.go
	if *gkeCommandGroup != "" {
		cmd = append([]string{*gkeCommandGroup}, cmd...)
	}

	// zone can be empty for regional cluster
	if zone != "" {
		cmd = append(cmd, fmt.Sprintf("--zone=%v", zone))
	} else if region != "" {
		cmd = append(cmd, fmt.Sprintf("--region=%v", region))
	}

	res, err := control.Output(exec.Command("gcloud", cmd...))
	if err != nil {
		return "", err
	}
	versions := strings.Split(strings.TrimSpace(string(res)), ";")
	latestValid := ""
	for _, version := range versions {
		if strings.HasPrefix(version, releasePrefix) {
			latestValid = version
			break
		}
	}
	if latestValid == "" {
		return "", fmt.Errorf("cannot find valid gke release %s version from: %s", releasePrefix, string(res))
	}
	return "v" + latestValid, nil
}

// (only works on gke)
// getChannelGKEVersion will return master version from a GKE release channel.
func getChannelGKEVersion(project, zone, region, gkeChannel string) (string, error) {
	cmd := []string{
		"container",
		"get-server-config",
		fmt.Sprintf("--project=%v", project),
		"--format=json(channels)",
	}

	/*
		sample output:
		{
		  "channels": [
		    {
		      "channel": "RAPID",
		      "defaultVersion": "1.14.3-gke.9"
		    },
		    {
		      "channel": "REGULAR",
		      "defaultVersion": "1.12.8-gke.10"
		    },
		    {
		      "channel": "STABLE",
		      "defaultVersion": "1.12.8-gke.10"
		    }
		  ]
		}
	*/

	type channel struct {
		Channel        string `json:"channel"`
		DefaultVersion string `json:"defaultVersion"`
	}

	type channels struct {
		Channels []channel `json:"channels"`
	}

	// --gkeCommandGroup is from gke.go
	if *gkeCommandGroup != "" {
		cmd = append([]string{*gkeCommandGroup}, cmd...)
	}

	// zone can be empty for regional cluster
	if zone != "" {
		cmd = append(cmd, fmt.Sprintf("--zone=%v", zone))
	} else if region != "" {
		cmd = append(cmd, fmt.Sprintf("--region=%v", region))
	}

	res, err := control.Output(exec.Command("gcloud", cmd...))
	if err != nil {
		return "", err
	}

	var c channels
	if err := json.Unmarshal(res, &c); err != nil {
		return "", err
	}

	for _, channel := range c.Channels {
		if strings.EqualFold(channel.Channel, gkeChannel) {
			return "v" + channel.DefaultVersion, nil
		}
	}

	return "", fmt.Errorf("cannot find a valid version for channel %s", gkeChannel)
}

// gcsWrite uploads contents to the dest location in GCS.
// It currently shells out to gsutil, but this could change in future.
func gcsWrite(dest string, contents []byte) error {
	f, err := ioutil.TempFile("", "")
	if err != nil {
		return fmt.Errorf("error creating temp file: %v", err)
	}

	defer func() {
		if err := os.Remove(f.Name()); err != nil {
			log.Printf("error removing temp file: %v", err)
		}
	}()

	if _, err := f.Write(contents); err != nil {
		return fmt.Errorf("error writing temp file: %v", err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("error closing temp file: %v", err)
	}

	return control.FinishRunning(exec.Command("gsutil", "cp", f.Name(), dest))
}

func setKubeShhBastionEnv(gcpProject, gcpZone, sshProxyInstanceName string) error {
	value, err := control.Output(exec.Command(
		"gcloud", "compute", "instances", "describe",
		sshProxyInstanceName,
		"--project="+gcpProject,
		"--zone="+gcpZone,
		"--format=get(networkInterfaces[0].accessConfigs[0].natIP)"))
	if err != nil {
		return fmt.Errorf("failed to get the external IP address of the '%s' instance: %v",
			sshProxyInstanceName, err)
	}
	address := strings.TrimSpace(string(value))
	if address == "" {
		return fmt.Errorf("instance '%s' doesn't have an external IP address", sshProxyInstanceName)
	}
	address += ":22"
	if err := os.Setenv("KUBE_SSH_BASTION", address); err != nil {
		return err
	}
	log.Printf("KUBE_SSH_BASTION set to: %v\n", address)
	return nil
}
