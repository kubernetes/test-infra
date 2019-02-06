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
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"k8s.io/test-infra/kubetest/util"
)

type extractFederationStrategy struct {
	mode      extractMode
	option    string
	ciVersion string
	value     string
}

type extractFederationStrategies []extractFederationStrategy

func (l *extractFederationStrategies) String() string {
	s := []string{}
	for _, e := range *l {
		s = append(s, e.value)
	}
	return strings.Join(s, ",")
}

// Converts --extract-federation=release/stable, etc into an extractFederationStrategy{}
func (l *extractFederationStrategies) Set(value string) error {
	var strategies = map[string]extractMode{
		`^(local)`:                    local,
		`^ci/(.+)$`:                   ci,
		`^release/(latest.*)$`:        rc,
		`^release/(stable.*)$`:        stable,
		`^(v\d+\.\d+\.\d+[\w.\-+]*)$`: version,
		`^(gs://.*)$`:                 gcs,
	}

	if len(*l) == 2 {
		return fmt.Errorf("may only define at most 2 --extract-federation strategies: %v %v", *l, value)
	}
	for search, mode := range strategies {
		re := regexp.MustCompile(search)
		mat := re.FindStringSubmatch(value)
		if mat == nil {
			continue
		}
		e := extractFederationStrategy{
			mode:   mode,
			option: mat[1],
			value:  value,
		}
		if len(mat) > 2 {
			e.ciVersion = mat[2]
		}
		*l = append(*l, e)
		return nil
	}
	return fmt.Errorf("unknown federation extraction strategy: %v", value)

}

func (l *extractFederationStrategies) Type() string {
	return "extractFederationStrategies"
}

// True when this kubetest invocation wants to download and extract a federation release.
func (l *extractFederationStrategies) Enabled() bool {
	return len(*l) > 0
}

func (e extractFederationStrategy) name() string {
	return filepath.Base(e.option)
}

func (l extractFederationStrategies) Extract(project, zone string) error {
	// rm -rf federation*
	files, err := ioutil.ReadDir(".")
	if err != nil {
		return err
	}
	for _, file := range files {
		name := file.Name()
		if !strings.HasPrefix(name, "federation") {
			continue
		}
		log.Printf("rm %s", name)
		if err = os.RemoveAll(name); err != nil {
			return err
		}
	}

	for i, e := range l {
		if i > 0 {
			if err := os.Rename("federation", "federation_skew"); err != nil {
				return err
			}
		}
		if err := e.Extract(project, zone); err != nil {
			return err
		}
	}

	return nil
}

// Find get-federation.sh at PWD, in PATH or else download it.
func ensureFederation() (string, error) {
	// Does get-federation.sh exist in pwd?
	i, err := os.Stat("./get-federation.sh")
	if err == nil && !i.IsDir() && i.Mode()&0111 > 0 {
		return "./get-federation.sh", nil
	}

	// How about in the path?
	p, err := exec.LookPath("get-federation.sh")
	if err == nil {
		return p, nil
	}

	// Download it to a temp file
	f, err := ioutil.TempFile("", "get-federation")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := httpRead("https://raw.githubusercontent.com/kubernetes/federation/master/cluster/get-federation.sh", f); err != nil {
		return "", err
	}
	i, err = f.Stat()
	if err != nil {
		return "", err
	}
	if err := os.Chmod(f.Name(), i.Mode()|0111); err != nil {
		return "", err
	}
	return f.Name(), nil
}

// Calls FEDERATION_RELEASE_URL=url FEDERATION_RELEASE=version get-federation.sh.
// This will download version from the specified url subdir and extract
// the tarballs.
var getFederation = func(url, version string) error {
	k, err := ensureFederation()
	if err != nil {
		return err
	}
	if err := os.Setenv("FEDERATION_RELEASE_URL", url); err != nil {
		return err
	}

	if err := os.Setenv("FEDERATION_RELEASE", version); err != nil {
		return err
	}
	if err := os.Setenv("FEDERATION_SKIP_CONFIRM", "y"); err != nil {
		return err
	}
	if err := os.Setenv("FEDERATION_DOWNLOAD_TESTS", "y"); err != nil {
		return err
	}
	log.Printf("url=%s version=%s get-federation.sh", url, version)
	for i := 0; i < 3; i++ {
		err = control.FinishRunning(exec.Command(k))
		if err == nil {
			break
		}
		err = fmt.Errorf("url=%s version=%s get-federation.sh failed: %v", url, version, err)
		if i == 2 {
			return err
		}
		log.Println(err)
		sleep(time.Duration(i) * time.Second)
	}
	return nil
}

func setFederationReleaseFromGcs(ci bool, suffix string) error {
	var prefix string
	if ci {
		prefix = "kubernetes-federation-dev/ci"
	} else {
		prefix = "kubernetes-federation-release/release"
	}

	url := fmt.Sprintf("https://storage.googleapis.com/%v", prefix)
	cat := fmt.Sprintf("gs://%v/%v.txt", prefix, suffix)
	release, err := control.Output(exec.Command("gsutil", "cat", cat))
	if err != nil {
		return err
	}
	return getFederation(url, strings.TrimSpace(string(release)))
}

func (e extractFederationStrategy) Extract(project, zone string) error {
	switch e.mode {
	case local:
		url := util.K8s("federation", "_output", "gcs-stage")
		files, err := ioutil.ReadDir(url)
		if err != nil {
			return err
		}
		var release string
		for _, file := range files {
			r := file.Name()
			if strings.HasPrefix(r, "v") {
				release = r
				break
			}
		}
		if len(release) == 0 {
			return fmt.Errorf("no releases found in %v", url)
		}
		return getFederation(fmt.Sprintf("file://%s", url), release)
	case ci:
		return setFederationReleaseFromGcs(true, e.option)
	case rc, stable:
		return setFederationReleaseFromGcs(false, e.option)
	case version:
		var url string
		release := e.option
		if strings.Contains(release, "+") {
			url = "https://storage.googleapis.com/kubernetes-federation-dev/ci"
		} else {
			url = "https://storage.googleapis.com/kubernetes-federation-release/release"
		}
		return getFederation(url, release)
	case gcs:
		// strip gs://foo -> /foo
		withoutGS := strings.TrimPrefix(e.option, "gs://")
		url := "https://storage.googleapis.com" + path.Dir(withoutGS)
		return getFederation(url, path.Base(withoutGS))
	case bazel:
		return getFederation("", e.option)
	}
	return fmt.Errorf("unrecognized federation extraction: %v(%v)", e.mode, e.value)
}
