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
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"k8s.io/test-infra/kubetest/util"
)

type extractMode int

const (
	none       extractMode = iota
	localBazel             // local bazel
	local                  // local
	gci                    // gci/FAMILY, gci/FAMILY?project=IMAGE_PROJECT:k8s-map-bucket=BUCKET_NAME
	gciCi                  // gci/FAMILY/CI_VERSION
	gke                    // gke(deprecated), gke-default, gke-latest, gke-channel-CHANNEL_NAME
	ci                     // ci/latest, ci/latest-1.5
	ciFast                 // ci/latest-fast, ci/latest-1.19-fast
	rc                     // release/latest, release/latest-1.5
	stable                 // release/stable, release/stable-1.5
	version                // v1.5.0, v1.5.0-beta.2
	gcs                    // gs://bucket/prefix/v1.6.0-alpha.0
	load                   // Load a --save cluster
	bazel                  // A pre/postsubmit bazel build version, prefixed with bazel/
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
		`^(bazel)$`: localBazel,
		`^(local)`:  local,
		`^gke-?(default|channel-(rapid|regular|stable)|latest(-\d+.\d+(.\d+(-gke)?)?)?)?$`: gke,
		`^gci/([\w-]+(?:\?{1}(?::?[\w-]+=[\w-]+)+)?)$`:                                     gci,
		`^gci/([\w-]+(?:\?{1}(?::?[\w-]+=[\w-]+)+)?)/(.+)$`:                                gciCi,
		`^ci/(.+)$`:                   ci,
		`^ci/(.+)-fast$`:              ciFast,
		`^release/(latest.*)$`:        rc,
		`^release/(stable.*)$`:        stable,
		`^(v\d+\.\d+\.\d+[\w.\-+]*)$`: version,
		`^(gs://.*)$`:                 gcs,
		`^(bazel/.*)$`:                bazel,
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
		if mode == ci && strings.HasSuffix(value, "-fast") {
			// do not match ci mode if will also match ciFast
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
		log.Printf("Matched extraction strategy: %s", search)
		return nil
	}
	return fmt.Errorf("Unknown extraction strategy: %v", value)

}

func (l *extractStrategies) Type() string {
	return "exactStrategies"
}

// True when this kubetest invocation wants to download and extract a release.
func (l *extractStrategies) Enabled() bool {
	return len(*l) > 0
}

func (e extractStrategy) name() string {
	return filepath.Base(e.option)
}

func (l extractStrategies) Extract(project, zone, region string, extractSrc bool) error {
	// rm -rf kubernetes*
	files, err := ioutil.ReadDir(".")
	if err != nil {
		return err
	}
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

	for i, e := range l {
		if i > 0 {
			// TODO(fejta): new strategy so we support more than 2 --extracts
			if err := os.Rename("kubernetes", "kubernetes_skew"); err != nil {
				return err
			}
		}
		if err := e.Extract(project, zone, region, extractSrc); err != nil {
			return err
		}
	}

	return nil
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

// Download named binaries for kubernetes
func getNamedBinaries(url, version, tarball string, retry int) error {
	f, err := os.Create(tarball)
	if err != nil {
		return err
	}
	defer f.Close()
	full := fmt.Sprintf("%s/%s/%s", url, version, tarball)

	for i := 0; i < retry; i++ {
		log.Printf("downloading %v from %v", tarball, full)
		if err := httpRead(full, f); err == nil {
			break
		}
		err = fmt.Errorf("url=%s version=%s failed get %v: %v", url, version, tarball, err)
		if i == retry-1 {
			return err
		}
		log.Println(err)
		sleep(time.Duration(i) * time.Second)
	}

	f.Close()
	o, err := control.Output(exec.Command("md5sum", f.Name()))
	if err != nil {
		return err
	}
	log.Printf("md5sum: %s", o)

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("unable to get current directory: %v", err)
	}
	log.Printf("Extracting tar file %v into directory %v", f.Name(), cwd)

	if err = control.FinishRunning(exec.Command("tar", "-xzf", f.Name())); err != nil {
		return err
	}
	return nil
}

var (
	sleep = time.Sleep
)

// Calls KUBERNETES_RELEASE_URL=url KUBERNETES_RELEASE=version get-kube.sh.
// This will download version from the specified url subdir and extract
// the tarballs.
var getKube = func(url, version string, getSrc bool) error {
	// TODO(krzyzacy): migrate rest of the get-kube.sh logic into kubetest, using getNamedBinaries
	// get/extract the src tarball first since bazel needs a clean tree
	if getSrc {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		if cwd != "kubernetes" {
			if err = os.Mkdir("kubernetes", 0755); err != nil {
				return err
			}
			if err = os.Chdir("kubernetes"); err != nil {
				return err
			}
		}

		if err := os.Setenv("KUBE_GIT_VERSION", version); err != nil {
			return err
		}

		if err := getNamedBinaries(url, version, "kubernetes-src.tar.gz", 3); err != nil {
			return err
		}
	}

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
	if err := os.Setenv("CLUSTER_API_VERSION", version[1:]); err != nil {
		return err
	}
	log.Printf("U=%s R=%s get-kube.sh", url, version)
	for i := 0; i < 5; i++ {
		err = control.FinishRunning(exec.Command(k))
		if err == nil {
			break
		}
		err = fmt.Errorf("U=%s R=%s get-kube.sh failed: %v", url, version, err)
		log.Println(err)
		sleep(time.Duration(i) * time.Second)
	}

	return err
}

// wrapper for gsutil cat
var gsutilCat = func(url string) ([]byte, error) {
	return control.Output(exec.Command("gsutil", "cat", url))
}

func setReleaseFromGcs(prefix, suffix string, getSrc bool) error {
	url := fmt.Sprintf("https://storage.googleapis.com/%v", prefix)
	catURL := fmt.Sprintf("gs://%v/%v.txt", prefix, suffix)
	release, err := gsutilCat(catURL)
	if err != nil {
		return fmt.Errorf("Failed to set release from %s (%v)", catURL, err)
	}
	return getKube(url, strings.TrimSpace(string(release)), getSrc)
}

var httpCat = func(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Unexpected HTTP status code: %d", resp.StatusCode)
	}

	release, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return release, nil
}

func setReleaseFromHTTP(prefix, suffix string) (string, string, error) {
	url := fmt.Sprintf("https://storage.googleapis.com/%s", prefix)
	catURL := fmt.Sprintf("%s/%s.txt", url, suffix)
	release, err := httpCat(catURL)
	if err != nil {
		return "", "", fmt.Errorf("Failed to set release from %s (%v)", catURL, err)
	}
	return url, strings.TrimSpace(string(release)), nil
}

var parseGciExtractOption = func(option string) (string, map[string]string) {
	tokens := strings.Split(option, "?")
	family := tokens[0]
	paramsMap := map[string]string{
		// default values
		"project":        "container-vm-image-staging",
		"k8s-map-bucket": "container-vm-image-staging",
	}
	if len(tokens) == 2 {
		params := strings.Split(tokens[1], ":")
		for _, param := range params {
			kv := strings.Split(param, "=")
			paramsMap[kv[0]] = kv[1]
		}
	}
	return family, paramsMap
}

var gcloudGetImageName = func(family string, project string) ([]byte, error) {
	return control.Output(exec.Command("gcloud", "compute", "images", "describe-from-family", family, fmt.Sprintf("--project=%v", project), "--format=value(name)"))
}

func setupGciVars(f string, p string) (string, error) {
	b, err := gcloudGetImageName(f, p)
	if err != nil {
		return "", err
	}
	i := strings.TrimSpace(string(b))
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
	if f == "gci-canary-test" {
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

func setReleaseFromGci(image string, k8sMapBucket string, getSrc bool) error {
	catURL := fmt.Sprintf("gs://%s/k8s-version-map/%s", k8sMapBucket, image)
	b, err := gsutilCat(catURL)
	if err != nil {
		return fmt.Errorf("Failed to set release from %s (%v)", catURL, err)
	}
	r := fmt.Sprintf("v%s", b)
	return getKube("https://storage.googleapis.com/kubernetes-release/release", strings.TrimSpace(r), getSrc)
}

func (e extractStrategy) Extract(project, zone, region string, extractSrc bool) error {
	switch e.mode {
	case localBazel:
		vFile := util.K8s("kubernetes", "bazel-bin", "version")
		vByte, err := ioutil.ReadFile(vFile)
		if err != nil {
			return err
		}
		version := strings.TrimSpace(string(vByte))
		log.Printf("extracting version %v\n", version)
		root := util.K8s("kubernetes", "bazel-bin", "build")
		src := filepath.Join(root, "release-tars")
		dst := filepath.Join(root, version)
		log.Printf("copying files from %v to %v\n", src, dst)
		if err := os.Rename(src, dst); err != nil {
			return err
		}
		return getKube(fmt.Sprintf("file://%s", root), version, extractSrc)
	case local:
		url := util.K8s("kubernetes", "_output", "gcs-stage")
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
		return getKube(fmt.Sprintf("file://%s", url), release, extractSrc)
	case gci, gciCi:
		family, gciExtractParams := parseGciExtractOption(e.option)
		project := gciExtractParams["project"]
		k8sMapBucket := gciExtractParams["k8s-map-bucket"]
		if i, err := setupGciVars(family, project); err != nil {
			return err
		} else if e.ciVersion != "" {
			return setReleaseFromGcs("kubernetes-release-dev/ci", e.ciVersion, extractSrc)
		} else {
			return setReleaseFromGci(i, k8sMapBucket, extractSrc)
		}
	case gke:
		// TODO(fejta): prod v staging v test
		if project == "" {
			return fmt.Errorf("--gcp-project unset")
		}
		if e.value == "gke" {
			log.Print("*** --extract=gke is deprecated, migrate to --extract=gke-default ***")
		}
		if strings.HasPrefix(e.option, "latest") {
			// get latest supported master version
			releasePrefix := ""
			if strings.HasPrefix(e.option, "latest-") {
				releasePrefix = strings.TrimPrefix(e.option, "latest-")
			}
			version, err := getLatestGKEVersion(project, zone, region, releasePrefix)
			if err != nil {
				return fmt.Errorf("failed to get latest gke version: %s", err)
			}
			return getKube("https://storage.googleapis.com/gke-release-staging/kubernetes/release", version, extractSrc)
		}

		if strings.HasPrefix(e.option, "channel") {
			// get latest supported master version
			version, err := getChannelGKEVersion(project, zone, region, e.ciVersion)
			if err != nil {
				return fmt.Errorf("failed to get gke version from channel %s: %s", e.ciVersion, err)
			}
			return getKube("https://storage.googleapis.com/gke-release-staging/kubernetes/release", version, extractSrc)
		}

		// TODO(krzyzacy): clean up gke-default logic
		if zone == "" {
			return fmt.Errorf("--gcp-zone unset")
		}

		// get default cluster version for default extract strategy
		ci, err := control.Output(exec.Command("gcloud", "container", "get-server-config", fmt.Sprintf("--project=%v", project), fmt.Sprintf("--zone=%v", zone), "--format=value(defaultClusterVersion)"))
		if err != nil {
			return err
		}
		re := regexp.MustCompile(`(\d+\.\d+)(\..+)?$`) // 1.11.7-beta.0 -> 1.11
		mat := re.FindStringSubmatch(strings.TrimSpace(string(ci)))
		if mat == nil {
			return fmt.Errorf("failed to parse version from %s", ci)
		}
		// When JENKINS_USE_SERVER_VERSION=y, we launch the default version as determined
		// by GKE, but pull the latest version of that branch for tests. e.g. if the default
		// version is 1.5.3, we would pull test binaries at ci/latest-1.5.txt, but launch
		// the default (1.5.3). We have to unset CLUSTER_API_VERSION here to allow GKE to
		// launch the default.
		// TODO(fejta): clean up this logic. Setting/unsetting the same env var is gross.
		defer os.Unsetenv("CLUSTER_API_VERSION")
		return setReleaseFromGcs("kubernetes-release-dev/ci", "latest-"+mat[1], extractSrc)
	case ci:
		if strings.HasPrefix(e.option, "gke-") {
			return setReleaseFromGcs("gke-release-staging/kubernetes/release", e.option, extractSrc)
		}

		url, release, err := setReleaseFromHTTP("kubernetes-release-dev/ci", e.option)
		if err != nil {
			return err
		}
		return getKube(url, release, extractSrc)
	case ciFast:
		// ciFast latest version marker is published to
		// 'kubernetes-release-dev/ci/<version>-fast.txt' but the actual source
		// is at 'kubernetes-release-dev/ci/fast/<version>/kubernetes.tar.gz'
		url, release, err := setReleaseFromHTTP("kubernetes-release-dev/ci", fmt.Sprintf("%s-fast", e.option))
		if err != nil {
			return err
		}
		return getKube(fmt.Sprintf("%s/fast", url), release, extractSrc)
	case rc, stable:
		url, release, err := setReleaseFromHTTP("kubernetes-release/release", e.option)
		if err != nil {
			return err
		}
		return getKube(url, release, extractSrc)
	case version:
		var url string
		release := e.option
		re := regexp.MustCompile(`(v\d+\.\d+\.\d+-gke.\d+)$`) // v1.8.0-gke.0
		if re.FindStringSubmatch(release) != nil {
			url = "https://storage.googleapis.com/gke-release-staging/kubernetes/release"
		} else if strings.Contains(release, "+") {
			url = "https://storage.googleapis.com/kubernetes-release-dev/ci"
		} else {
			url = "https://storage.googleapis.com/kubernetes-release/release"
		}
		return getKube(url, release, extractSrc)
	case gcs:
		// strip gs://foo/bar(.txt) -> foo/bar(.txt)
		withoutGS := e.option[5:]
		if strings.HasSuffix(e.option, ".txt") {
			// foo/bar.txt -> bar
			suffix := strings.TrimSuffix(path.Base(withoutGS), filepath.Ext(withoutGS))
			return setReleaseFromGcs(path.Dir(withoutGS), suffix, extractSrc)
		}
		url := "https://storage.googleapis.com" + "/" + path.Dir(withoutGS)
		return getKube(url, path.Base(withoutGS), extractSrc)
	case load:
		return loadState(e.option, extractSrc)
	case bazel:
		return getKube("", e.option, extractSrc)
	}
	return fmt.Errorf("Unrecognized extraction: %v(%v)", e.mode, e.value)
}

func loadKubeconfig(save string) error {
	cURL, err := util.JoinURL(save, "kube-config")
	if err != nil {
		return fmt.Errorf("bad load url %s: %v", save, err)
	}
	if err := os.MkdirAll(util.Home(".kube"), 0775); err != nil {
		return err
	}
	return control.FinishRunning(exec.Command("gsutil", "cp", cURL, util.Home(".kube", "config")))
}

func loadState(save string, getSrc bool) error {
	log.Printf("Restore state from %s", save)

	uURL, err := util.JoinURL(save, "release-url.txt")
	if err != nil {
		return fmt.Errorf("bad load url %s: %v", save, err)
	}
	rURL, err := util.JoinURL(save, "release.txt")
	if err != nil {
		return fmt.Errorf("bad load url %s: %v", save, err)
	}

	if err := loadKubeconfig(save); err != nil {
		return fmt.Errorf("failed loading kubeconfig: %v", err)
	}

	url, err := gsutilCat(uURL)
	if err != nil {
		return err
	}
	release, err := gsutilCat(rURL)
	if err != nil {
		return err
	}
	return getKube(string(url), string(release), getSrc)
}

func saveState(save string) error {
	url := os.Getenv("KUBERNETES_RELEASE_URL") // TODO(fejta): pass this in to saveState
	version := os.Getenv("KUBERNETES_RELEASE")
	log.Printf("Save U=%s R=%s to %s", url, version, save)
	cURL, err := util.JoinURL(save, "kube-config")
	if err != nil {
		return fmt.Errorf("bad save url %s: %v", save, err)
	}
	uURL, err := util.JoinURL(save, "release-url.txt")
	if err != nil {
		return fmt.Errorf("bad save url %s: %v", save, err)
	}
	rURL, err := util.JoinURL(save, "release.txt")
	if err != nil {
		return fmt.Errorf("bad save url %s: %v", save, err)
	}

	if err := control.FinishRunning(exec.Command("gsutil", "cp", util.Home(".kube", "config"), cURL)); err != nil {
		return fmt.Errorf("failed to save .kube/config to %s: %v", cURL, err)
	}
	if cmd, err := control.InputCommand(url, "gsutil", "cp", "-", uURL); err != nil {
		return fmt.Errorf("failed to write url %s to %s: %v", url, uURL, err)
	} else if err = control.FinishRunning(cmd); err != nil {
		return fmt.Errorf("failed to upload url %s to %s: %v", url, uURL, err)
	}

	if cmd, err := control.InputCommand(version, "gsutil", "cp", "-", rURL); err != nil {
		return fmt.Errorf("failed to write release %s to %s: %v", version, rURL, err)
	} else if err = control.FinishRunning(cmd); err != nil {
		return fmt.Errorf("failed to upload release %s to %s: %v", version, rURL, err)
	}
	return nil
}
