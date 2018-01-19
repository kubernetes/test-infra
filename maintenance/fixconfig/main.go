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

/*
fixconfig automatically fixes the prow config to have automatically generated
security repo presubmits transformed from the kubernetes presubmits

NOTE: this makes a few assumptions
- $PWD/prow/config.yaml is where the config lives (unless you supply --config=)
- `presubmits:` exists
- `  kubernetes-security/kubernetes:` exists in presubmits
- some other `  org/repo:` exists in presubmits *after* `  kubernetes-security/kubernetes:`
- the original contents around this will be kept, but this section will be automatically rewritten
*/
package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/ghodss/yaml"
	flag "github.com/spf13/pflag"

	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
)

var configPath = flag.String("config", "", "path to prow/config.yaml, defaults to $PWD/prow/config.yaml")

func readConfig(path string) (raw []byte, parsed *config.Config, err error) {
	raw, err = ioutil.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	parsed = &config.Config{}
	err = yaml.Unmarshal(raw, parsed)
	if err != nil {
		return nil, nil, err
	}
	return raw, parsed, nil
}

func getSecurityRepoJobsIndex(configBytes []byte) (start, end int, err error) {
	// find security-repo config begining
	// first find presubmits
	presubmitIdx := bytes.Index(configBytes, ([]byte)("presubmits:"))
	// then find k-s/k:
	startRegex := regexp.MustCompile("(?m)^  kubernetes-security/kubernetes:$")
	loc := startRegex.FindIndex(configBytes[presubmitIdx:])
	if loc == nil {
		return 0, 0, fmt.Errorf("failed to find start of security repo presubmits")
	}
	start = presubmitIdx + loc[1]
	// must be like `  org/repo:`
	loc = regexp.MustCompile("(?m)^  [^ #-][^ #]+/.+:$").FindIndex(configBytes[start:])
	if loc == nil {
		return 0, 0, fmt.Errorf("failed to find end of security repo presubmits")
	}
	// loc[0] is the beginning of the match
	end = start + loc[0]
	return start, end, nil
}

// strip cache ssd related settings
func stripCache(j *config.Presubmit) {
	container := &j.Spec.Containers[0]
	// strip cache disk related args etc
	filteredArgs := []string{}
	for _, arg := range container.Args {
		if strings.HasPrefix(arg, "--git-cache") {
			continue
		}
		filteredArgs = append(filteredArgs, arg)
	}
	container.Args = filteredArgs
	// filter cache related env
	filteredEnv := []kube.EnvVar{}
	for _, env := range container.Env {
		// don't keep bazel cache directory env
		if env.Name == "TEST_TMPDIR" {
			continue
		}
		filteredEnv = append(filteredEnv, env)
	}
	container.Env = filteredEnv
	// filter cache disk volumes, swap DIND volume for
	filteredVolumes := []kube.Volume{}
	removedVolumeNames := sets.String{}
	for _, volume := range j.Spec.Volumes {
		if volume.VolumeSource.HostPath != nil &&
			strings.HasPrefix(volume.VolumeSource.HostPath.Path, "/mnt/disks/ssd0") {
			removedVolumeNames.Insert(volume.Name)
			continue
		}
		filteredVolumes = append(filteredVolumes, volume)
	}
	j.Spec.Volumes = filteredVolumes
	// filter out mounts for filtered out volumes
	filteredVolumeMounts := []kube.VolumeMount{}
	for _, volumeMount := range container.VolumeMounts {
		if removedVolumeNames.Has(volumeMount.Name) {
			continue
		}
		filteredVolumeMounts = append(filteredVolumeMounts, volumeMount)
	}
	container.VolumeMounts = filteredVolumeMounts
	// remove """cache port"""
	container.Ports = []kube.Port{}
}

// run after stripCache to make sure we still at least mount an emptyDir to
// /docker-graph for dind enabled jobs
func ensureDockerGraphVolume(j *config.Presubmit) {
	// make sure this is a docker-in-docker job first
	dindEnabled := false
	container := &j.Spec.Containers[0]
	for _, env := range container.Env {
		if env.Name == "DOCKER_IN_DOCKER_ENABLED" && env.Value == "true" {
			dindEnabled = true
			break
		}
	}
	if !dindEnabled {
		return
	}

	// filter out old /docker-graph volume mounts of any sort
	const dockerGraphMountPath = "/docker-graph"
	oldDockerGraphVolumeMount := ""
	removedVolumeNames := sets.String{}
	filteredVolumeMounts := []kube.VolumeMount{}
	for _, volumeMount := range container.VolumeMounts {
		if volumeMount.MountPath == dockerGraphMountPath {
			removedVolumeNames.Insert(volumeMount.Name)
			continue
		}
		filteredVolumeMounts = append(filteredVolumeMounts, volumeMount)
	}
	container.VolumeMounts = filteredVolumeMounts

	// remove old volumes associated with old mounts if any
	if removedVolumeNames.Len() > 0 {
		filteredVolumes := []kube.Volume{}
		for _, volume := range j.Spec.Volumes {
			if volume.Name == oldDockerGraphVolumeMount {
				continue
			}
			filteredVolumes = append(filteredVolumes, volume)
		}
		j.Spec.Volumes = filteredVolumes
	}

	// add new auto generated volume mount
	const dockerGraphVolumeMount = "auto-generated-docker-graph-volume-mount"
	container.VolumeMounts = append(container.VolumeMounts, kube.VolumeMount{
		Name:      dockerGraphVolumeMount,
		MountPath: dockerGraphMountPath,
	})

	// add matching auto generated emptyDir volume
	volumeSource := kube.VolumeSource{}
	volumeSource.EmptyDir = &kube.EmptyDirVolumeSource{}
	volume := kube.Volume{
		Name:         dockerGraphVolumeMount,
		VolumeSource: volumeSource,
	}
	j.Spec.Volumes = append(j.Spec.Volumes, volume)
}

// convert a kubernetes/kubernetes job to a kubernetes-security/kubernetes job
// xref: prow/config/config_test.go replace(...)
func convertJobToSecurityJob(j *config.Presubmit) {
	// fix name and triggers for all jobs
	j.Name = strings.Replace(j.Name, "pull-kubernetes", "pull-security-kubernetes", -1)
	j.RerunCommand = strings.Replace(j.RerunCommand, "pull-kubernetes", "pull-security-kubernetes", -1)
	j.Trigger = strings.Replace(j.Trigger, "pull-kubernetes", "pull-security-kubernetes", -1)
	j.Context = strings.Replace(j.Context, "pull-kubernetes", "pull-security-kubernetes", -1)
	// handle k8s job args, volumes etc
	if j.Agent == "kubernetes" {
		j.Cluster = "security"
		container := &j.Spec.Containers[0]
		for i, arg := range container.Args {
			// handle --repo substitution for main repo
			if strings.HasPrefix(arg, "--repo=k8s.io/kubernetes") || strings.HasPrefix(arg, "--repo=k8s.io/$(REPO_NAME)") {
				container.Args[i] = strings.Replace(arg, "k8s.io/", "github.com/kubernetes-security/", 1)

				// handle upload bucket
			} else if strings.HasPrefix(arg, "--upload=") {
				container.Args[i] = "--upload=gs://kubernetes-security-jenkins/pr-logs"
			}
		}
		container.Args = append(container.Args, "--ssh=/etc/ssh-security/ssh-security")
		container.VolumeMounts = append(
			container.VolumeMounts,
			kube.VolumeMount{
				Name:      "ssh-security",
				MountPath: "/etc/ssh-security",
			},
		)
		j.Spec.Volumes = append(
			j.Spec.Volumes,
			kube.Volume{
				Name: "ssh-security",
				VolumeSource: kube.VolumeSource{
					Secret: &kube.SecretSource{
						Name:        "ssh-security",
						DefaultMode: 0400,
					},
				},
			},
		)
		// remove cache-ssd related args
		stripCache(j)
		// strip cache may remove the /docker-graph mount if it is on the cache
		// ssd, make sure we still have an emptyDir instead for dind jobs
		ensureDockerGraphVolume(j)
	}
	// done with this job, check for run_after_success
	for i := range j.RunAfterSuccess {
		convertJobToSecurityJob(&j.RunAfterSuccess[i])
	}
}

func yamlBytesStripNulls(yamlBytes []byte) []byte {
	nullRE := regexp.MustCompile("(?m)[\n]+^[^\n]+: null$")
	return nullRE.ReplaceAll(yamlBytes, []byte{})
}

func yamlBytesToEntry(yamlBytes []byte, indent int) []byte {
	var buff bytes.Buffer
	// spaces of length indent
	prefix := bytes.Repeat([]byte{32}, indent)
	// `- ` before the first field of a yaml entry
	prefix[len(prefix)-2] = byte(45)
	buff.Write(prefix)
	// put back space
	prefix[len(prefix)-2] = byte(32)
	for i, b := range yamlBytes {
		buff.WriteByte(b)
		// indent after newline, except the last one
		if b == byte(10) && i+1 != len(yamlBytes) {
			buff.Write(prefix)
		}
	}
	return buff.Bytes()
}

func copyFile(srcPath, destPath string) error {
	// fallback to copying the file instead
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	dst, err := os.OpenFile(destPath, os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, src)
	if err != nil {
		return err
	}
	dst.Sync()
	dst.Close()
	src.Close()
	return nil
}

func main() {
	flag.Parse()
	// default to $PWD/prow/config.yaml
	if *configPath == "" {
		pwd, err := os.Getwd()
		if err != nil {
			log.Fatalf("Failed to get $PWD: %v", err)
		}
		*configPath = pwd + "/prow/config.yaml"
	}
	// read in current prow config
	originalBytes, parsed, err := readConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to read config file: %v", err)
	}
	// find security repo section
	securityRepoStart, securityRepoEnd, err := getSecurityRepoJobsIndex(originalBytes)
	if err != nil {
		log.Fatalf("Failed to find security repo section: %v", err)
	}

	// create temp file to write updated config
	f, err := ioutil.TempFile("", "prow-config")
	if err != nil {
		log.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(f.Name())

	// write the original bytes before the security repo section
	_, err = f.Write(originalBytes[:securityRepoStart])
	if err != nil {
		log.Fatalf("Failed to write temp file: %v", err)
	}
	f.Sync()
	io.WriteString(f, "\n")

	// convert each kubernetes/kubernetes presubmit to a
	// kubernetes-security/kubernetes presubmit and write to the file
	for _, job := range parsed.Presubmits["kubernetes/kubernetes"] {
		convertJobToSecurityJob(&job)
		jobBytes, err := yaml.Marshal(job)
		if err != nil {
			log.Fatalf("Failed to marshal job: %v", err)
		}
		// write, properly indented, and stripped of `foo: null`
		jobBytes = yamlBytesStripNulls(jobBytes)
		f.Write(yamlBytesToEntry(jobBytes, 4))
	}

	// write the original bytes after the security repo section
	_, err = f.Write(originalBytes[securityRepoEnd:])
	if err != nil {
		log.Fatalf("Failed to write temp file: %v", err)
	}
	f.Sync()

	// move file to replace original
	f.Close()
	err = os.Rename(f.Name(), *configPath)
	if err != nil {
		// fallback to copying the file instead
		err = copyFile(f.Name(), *configPath)
		if err != nil {
			log.Fatalf("Failed to replace config with updated version: %v", err)
		}
	}
}
