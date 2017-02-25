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
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

type extractMode int

const (
	none    extractMode = iota
	local               // local
	gci                 // gci/FAMILY
	gciCi               // gci/FAMILY/CI_VERSION
	gke                 // gke, gke-staging, gke-test
	ci                  // ci/latest, ci/latest-1.5
	rc                  // release/latest, release/latest-1.5
	stable              // release/stable, release/stable-1.5
	version             // v1.5.0, v1.5.0-beta.2
	gcs                 // gs://bucket/prefix/v1.6.0-alpha.0
)

type extractStrategy struct {
	mode      extractMode
	option    string
	ciVersion string
	value     string
}

type extractStrategies []extractStrategy

func (l *extractStrategies) String() string {
	s := []string{}
	for _, e := range *l {
		s = append(s, e.value)
	}
	return strings.Join(s, ",")
}

// Converts --extract=release/stable, etc into an extractStrategy{}
func (l *extractStrategies) Set(value string) error {
	var strategies = map[string]extractMode{
		`^(local)`:                  local,
		`^gke-?(staging|test)?$`:    gke,
		`^gci/([\w-]+)$`:            gci,
		`^gci/([\w-]+)/(.+)$`:       gciCi,
		`^ci/(.+)$`:                 ci,
		`^release/(latest.*)$`:      rc,
		`^release/(stable.*)$`:      stable,
		`^(v\d+\.\d+\.\d+[\w.-]*)$`: version,
		`^(gs://.*)$`:               gcs,
	}

	if len(*l) == 2 {
		return fmt.Errorf("May only define at most 2 --extract strategies: %v %v", *l, value)
	}
	for search, mode := range strategies {
		re := regexp.MustCompile(search)
		mat := re.FindStringSubmatch(value)
		if mat == nil {
			continue
		}
		e := extractStrategy{
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
	return fmt.Errorf("Unknown extraction strategy: %v", value)

}

// True when this kubetest invocation wants to download and extract a release.
func (l *extractStrategies) Enabled() bool {
	return len(*l) > 0
}

func (e extractStrategy) name() string {
	return filepath.Base(e.option)
}

func (l extractStrategies) Extract() error {
	// rm -rf kubernetes*
	if files, err := ioutil.ReadDir("."); err != nil {
		return err
	} else {
		for _, file := range files {
			name := file.Name()
			if !strings.HasPrefix(name, "kubernetes") {
				continue
			}
			log.Printf("rm %s", name)
			if err = os.RemoveAll(name); err != nil {
				return err
			}
		}
	}

	for i, e := range l {
		if i > 0 {
			// TODO(fejta): new strategy so we support more than 2 --extracts
			if err := os.Rename("kubernetes", "kubernetes_skew"); err != nil {
				return err
			}
		}
		if err := e.Extract(); err != nil {
			return err
		}
	}

	return os.Chdir("kubernetes")
}

// Find get-kube.sh at PWD, in PATH or else download it.
func ensureKube() (string, error) {
	// Does get-kube.sh exist in pwd?
	i, err := os.Stat("./get-kube.sh")
	if err == nil && !i.IsDir() && i.Mode()&0111 > 0 {
		return "./get-kube.sh", nil
	}

	// How about in the path?
	p, err := exec.LookPath("get-kube.sh")
	if err == nil {
		return p, nil
	}

	// Download it to a temp file
	f, err := ioutil.TempFile("", "get-kube")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := httpRead("https://get.k8s.io", f); err != nil {
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

// Download test binaries for kubernetes versions before 1.5.
func getTestBinaries(url, version string) error {
	f, err := os.Create("kubernetes-test.tar.gz")
	if err != nil {
		return err
	}
	defer f.Close()
	full := fmt.Sprintf("%v/%v/kubernetes-test.tar.gz", url, version)
	if err := httpRead(full, f); err != nil {
		return err
	}
	f.Close()
	o, err := combinedOutput(exec.Command("md5sum", f.Name()))
	if err != nil {
		return err
	}
	log.Printf("md5sum: %s", o)
	if err = finishRunning(exec.Command("tar", "-xzf", f.Name())); err != nil {
		return err
	}
	return nil
}

// Calls KUBERNETES_RELASE_URL=url KUBERNETES_RELEASE=version get-kube.sh.
// This will download version from the specified url subdir and extract
// the tarballs.
func getKube(url, version string) error {
	k, err := ensureKube()
	if err != nil {
		return err
	}
	if err := os.Setenv("KUBERNETES_RELEASE_URL", url); err != nil {
		return err
	}

	if err := os.Setenv("KUBERNETES_RELEASE", version); err != nil {
		return err
	}
	if err := os.Setenv("KUBERNETES_SKIP_CONFIRM", "y"); err != nil {
		return err
	}
	if err := os.Setenv("KUBERNETES_SKIP_CREATE_CLUSTER", "y"); err != nil {
		return err
	}
	if err := os.Setenv("KUBERNETES_DOWNLOAD_TESTS", "y"); err != nil {
		return err
	}
	// kube-up in cluster/gke/util.sh depends on this
	if err := os.Setenv("CLUSTER_API_VERSION", version[1:len(version)]); err != nil {
		return err
	}
	log.Printf("U=%s R=%s get-kube.sh", url, version)
	if err := finishRunning(exec.Command(k)); err != nil {
		return err
	}
	i, err := os.Stat("./kubernetes/cluster/get-kube-binaries.sh")
	if err != nil || i.IsDir() {
		log.Printf("Grabbing test binaries since R=%s < 1.5", version)
		if err = getTestBinaries(url, version); err != nil {
			return err
		}
	}
	return nil
}

func setReleaseFromGcs(ci bool, suffix string) error {
	var prefix string
	if ci {
		prefix = "kubernetes-release-dev/ci"
	} else {
		prefix = "kubernetes-release/release"
	}

	url := fmt.Sprintf("https://storage.googleapis.com/%v", prefix)
	cat := fmt.Sprintf("gs://%v/%v.txt", prefix, suffix)
	release, err := combinedOutput(exec.Command("gsutil", "cat", cat))
	if err != nil {
		return err
	}
	return getKube(url, strings.TrimSpace(string(release)))
}

func setupGciVars(family string) (string, error) {
	p := "container-vm-image-staging"
	b, err := combinedOutput(exec.Command("gcloud", "compute", "images", "describe-from-family", family, fmt.Sprintf("--project=%v", p), "--format=value(name)"))
	if err != nil {
		return "", err
	}
	i := string(b)
	g := "gci"
	m := map[string]string{
		"KUBE_GCE_MASTER_PROJECT":     p,
		"KUBE_GCE_MASTER_IMAGE":       i,
		"KUBE_MASTER_OS_DISTRIBUTION": g,

		"KUBE_GCE_NODE_PROJECT":     p,
		"KUBE_GCE_NODE_IMAGE":       i,
		"KUBE_NODE_OS_DISTRIBUTION": g,

		"BUILD_METADATA_GCE_MASTER_IMAGE": i,
		"BUILD_METADATA_GCE_NODE_IMAGE":   i,

		"KUBE_OS_DISTRIBUTION": g,
	}
	if family == "gci-canary-test" {
		var b bytes.Buffer
		if err := httpRead("https://api.github.com/repos/docker/docker/releases", &b); err != nil {
			return "", err
		}
		var v []map[string]interface{}
		if err := json.NewDecoder(&b).Decode(&v); err != nil {
			return "", err
		}
		// We want 1.13.0
		m["KUBE_GCI_DOCKER_VERSION"] = v[0]["name"].(string)[1:]
	}
	for k, v := range m {
		log.Printf("export %s=%s", k, v)
		if err := os.Setenv(k, v); err != nil {
			return "", err
		}
	}
	return i, nil
}

func setReleaseFromGci(image string) error {
	u := fmt.Sprintf("gs://container-vm-image-staging/k8s-version-map/%s", image)
	b, err := combinedOutput(exec.Command("gsutil", "cat", u))
	if err != nil {
		return err
	}
	r := fmt.Sprintf("v%s", b)
	return getKube("https://storage.googleapis.com/kubernetes-release/release", strings.TrimSpace(r))
}

func (e extractStrategy) Extract() error {
	switch e.mode {
	case local:
		url := "./_output/gcs-stage"
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
			return fmt.Errorf("No releases found in %v", url)
		}
		return getKube(url, release)
	case gci, gciCi:
		if i, err := setupGciVars(e.option); err != nil {
			return err
		} else if e.ciVersion != "" {
			return setReleaseFromGcs(true, e.ciVersion)
		} else {
			return setReleaseFromGci(i)
		}
	case gke:
		// TODO(fejta): prod v staging v test
		p := os.Getenv("PROJECT")
		if len(p) == 0 {
			return fmt.Errorf("PROJECT is unset")
		}
		z := os.Getenv("ZONE")
		if len(z) == 0 {
			return fmt.Errorf("ZONE is unset")
		}
		ci, err := combinedOutput(exec.Command("gcloud", "container", "get-server-config", fmt.Sprintf("--project=%v", p), fmt.Sprintf("--zone=%v", z), "--format=value(defaultClusterVersion)"))
		if err != nil {
			return err
		}
		return setReleaseFromGcs(true, strings.TrimSpace(string(ci)))
	case ci:
		return setReleaseFromGcs(true, e.option)
	case rc, stable:
		return setReleaseFromGcs(false, e.option)
	case version:
		var url string
		release := e.option
		if strings.Contains(release, "+") {
			url = "https://storage.googleapis.com/kubernetes-release-dev/ci"
		} else {
			url = "https://storage.googleapis.com/kubernetes-release/release"
		}
		return getKube(url, release)
	case gcs:
		return getKube(path.Dir(e.option), path.Base(e.option))
	}
	return fmt.Errorf("Unrecognized extraction: %v(%v)", e.mode, e.value)
}
