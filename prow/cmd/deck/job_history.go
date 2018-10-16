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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/deck/jobs"
)

const (
	resultsPerPage  = 20
	idParam         = "buildId"
	latestBuildFile = "latest-build.txt"

	// ** Job history assumes the GCS layout specified here:
	// https://github.com/kubernetes/test-infra/tree/master/gubernator#gcs-bucket-layout
	logsPrefix     = "logs"
	symLinkPrefix  = "pr-logs/directory"
	spyglassPrefix = "/view/gcs"
)

var (
	prefixRe = regexp.MustCompile("gs://.*?/")
)

type buildData struct {
	index        int
	SpyglassLink string
	ID           string
	Started      time.Time
	Duration     time.Duration
	Result       string
}

type jobHistoryTemplate struct {
	OlderLink  string
	NewerLink  string
	LatestLink string
	Name       string
	Builds     []buildData
}

func readObject(obj *storage.ObjectHandle) ([]byte, error) {
	rc, err := obj.NewReader(context.Background())
	if err != nil {
		return []byte{}, fmt.Errorf("failed to get reader for GCS object: %v", err)
	}
	return ioutil.ReadAll(rc)
}

func readLatestBuild(bkt *storage.BucketHandle, root string) (int, error) {
	path := path.Join(root, latestBuildFile)
	data, err := readObject(bkt.Object(path))
	if err != nil {
		return -1, fmt.Errorf("failed to read latest build number: %v", err)
	}
	n, err := strconv.Atoi(string(data))
	if err != nil {
		return -1, fmt.Errorf("failed to parse latest build number: %v", err)
	}
	return n, nil
}

// resolve sym links into the actual log directory for a particular test run
func resolveSymLink(bkt *storage.BucketHandle, symLink string) (string, error) {
	data, err := readObject(bkt.Object(symLink))
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %v", symLink, err)
	}
	// strip gs://<bucket-name> from global address `u`
	u := string(data)
	return prefixRe.ReplaceAllString(u, ""), nil
}

func spyglassLink(bkt *storage.BucketHandle, root, id string) (string, error) {
	bAttrs, err := bkt.Attrs(context.Background())
	if err != nil {
		return "", fmt.Errorf("failed to get bucket name: %v", err)
	}
	bktName := bAttrs.Name
	p, err := getPath(bkt, root, id, "")
	if err != nil {
		return "", fmt.Errorf("failed to get path: %v", err)
	}
	return path.Join(spyglassPrefix, bktName, p), nil
}

func getPath(bkt *storage.BucketHandle, root, id, fname string) (string, error) {
	if strings.HasPrefix(root, logsPrefix) {
		return path.Join(root, id, fname), nil
	}
	symLink := path.Join(root, id+".txt")
	dir, err := resolveSymLink(bkt, symLink)
	if err != nil {
		return "", fmt.Errorf("failed to resolve sym link: %v", err)
	}
	return path.Join(dir, fname), nil
}

func fileExists(bkt *storage.BucketHandle, root, id, fname string) bool {
	p, err := getPath(bkt, root, id, fname)
	if err != nil {
		return false
	}
	obj := bkt.Object(p)
	_, err = obj.Attrs(context.Background())
	return err == nil
}

func readStarted(bkt *storage.BucketHandle, root, id string) (jobs.Started, error) {
	s := jobs.Started{}
	p, err := getPath(bkt, root, id, "started.json")
	if err != nil {
		return s, fmt.Errorf("failed to get path: %v", err)
	}
	sdata, err := readObject(bkt.Object(p))
	if err != nil {
		return s, fmt.Errorf("failed to read started.json for build %s: %v", id, err)
	}
	err = json.Unmarshal(sdata, &s)
	if err != nil {
		return s, fmt.Errorf("failed to parse started.json for build %s: %v", id, err)
	}
	return s, nil
}

func readFinished(bkt *storage.BucketHandle, root, id string) (jobs.Finished, error) {
	f := jobs.Finished{}
	p, err := getPath(bkt, root, id, "finished.json")
	if err != nil {
		return f, fmt.Errorf("failed to get path: %v", err)
	}
	fdata, err := readObject(bkt.Object(p))
	if err != nil {
		return f, fmt.Errorf("failed to read finished.json for build %s: %v", id, err)
	}
	err = json.Unmarshal(fdata, &f)
	if err != nil {
		return f, fmt.Errorf("failed to parse finished.json for build %s: %v", id, err)
	}
	return f, nil
}

// Gets job history from the GCS bucket specified in config.
func getJobHistory(url *url.URL, config *config.Config, gcsClient *storage.Client) (jobHistoryTemplate, error) {
	start := time.Now()

	jobName := strings.TrimPrefix(url.Path, "/job-history/")
	tmpl := jobHistoryTemplate{
		Name:   jobName,
		Builds: make([]buildData, resultsPerPage),
	}

	var latest int
	bucketName := config.ProwConfig.Plank.DefaultDecorationConfig.GCSConfiguration.Bucket
	bkt := gcsClient.Bucket(bucketName)
	var root string
	found := false
	for _, r := range []string{logsPrefix, symLinkPrefix} {
		root = path.Join(r, jobName)
		n, err := readLatestBuild(bkt, root)
		if err == nil {
			latest = n
			found = true
			break
		}
	}
	if !found {
		return tmpl, fmt.Errorf("failed to locate build data")
	}
	var top, bottom int // build ids of the top (inclusive) and bottom (exclusive) results
	if idVals := url.Query()[idParam]; len(idVals) >= 1 {
		var err error
		if top, err = strconv.Atoi(idVals[0]); err != nil {
			return tmpl, fmt.Errorf("invalid value for %s: %v", idParam, err)
		}
	} else {
		top = latest
	}
	if top != latest {
		newer := top + resultsPerPage
		if newer > latest {
			newer = latest
		}
		u := *url
		q := u.Query()
		q.Set(idParam, strconv.Itoa(newer))
		u.RawQuery = q.Encode()
		tmpl.NewerLink = u.String()

		q.Del(idParam)
		u.RawQuery = q.Encode()
		tmpl.LatestLink = u.String()
	}
	bottom = top - resultsPerPage
	// concurrently check if there are no older results to display
	showOlder := false
	fch := make(chan bool)
	for i := bottom; i > bottom-resultsPerPage; i-- {
		go func(i int) {
			fch <- fileExists(bkt, root, strconv.Itoa(i), "started.json")
		}(i)
	}
	for i := 0; i < resultsPerPage; i++ {
		if <-fch {
			showOlder = true
			break
		}
	}
	if showOlder {
		u := *url
		q := u.Query()
		q.Set(idParam, strconv.Itoa(bottom))
		u.RawQuery = q.Encode()
		tmpl.OlderLink = u.String()
	}

	bch := make(chan buildData)
	for i := top; i > bottom; i-- {
		go func(i int) {
			build := buildData{
				index:  top - i,
				ID:     strconv.Itoa(i),
				Result: "Unfinished",
			}
			link, err := spyglassLink(bkt, root, build.ID)
			if err != nil {
				logrus.Warning(err)
				bch <- build
				return
			}
			build.SpyglassLink = link
			started, err := readStarted(bkt, root, build.ID)
			if err == nil {
				build.Started = time.Unix(started.Timestamp, 0)
				finished, _ := readFinished(bkt, root, build.ID)
				if finished.Timestamp != 0 {
					build.Duration = time.Unix(finished.Timestamp, 0).Sub(build.Started)
				}
				if finished.Result != "" {
					build.Result = finished.Result
				}
			} else {
				logrus.Warning(err)
			}
			bch <- build
		}(i)
	}
	for i := 0; i < resultsPerPage; i++ {
		b := <-bch
		tmpl.Builds[b.index] = b
	}

	elapsed := time.Now().Sub(start)
	logrus.Infof("got job history for %s in %v", jobName, elapsed)
	return tmpl, nil
}
