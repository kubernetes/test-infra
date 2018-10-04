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
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/config"
)

const (
	resultsPerPage = 20
	idParam        = "buildId"

	// ** Job history assumes the GCS layout specified here:
	// https://github.com/kubernetes/test-infra/tree/master/gubernator#gcs-bucket-layout
	logsPrefix    = "logs"
	symLinkPrefix = "pr-logs/directory"
)

type BuildData struct {
	index        int
	SpyglassLink string
	ID           string
	Started      time.Time
	Duration     time.Duration
	Result       string
}

type JobHistoryTemplate struct {
	OlderLink  string
	NewerLink  string
	LatestLink string
	Name       string
	Builds     []BuildData
}

type started struct {
	Timestamp int64 `json:"timestamp"`
}

type finished struct {
	Timestamp int64  `json:"timestamp"`
	Result    string `json:"result"`
}

func readLatestBuild(obj *storage.ObjectHandle) (int, error) {
	rc, err := obj.NewReader(context.Background())
	if err != nil {
		return -1, fmt.Errorf("Failed to get reader for latest build number: %v", err)
	}
	data, err := ioutil.ReadAll(rc)
	if err != nil {
		return -1, fmt.Errorf("Failed to read latest build number: %v", err)
	}
	n64, err := strconv.Atoi(string(data))
	if err != nil {
		return -1, fmt.Errorf("Failed to parse latest build number: %v", err)
	}
	return int(n64), nil
}

// resolve sym links into the actual log directory for a particular test run
func resolveSymLink(bkt *storage.BucketHandle, symLink string) (string, error) {
	linkObj := bkt.Object(symLink)
	rc, err := linkObj.NewReader(context.Background())
	if err != nil {
		return "", err
	}
	data, err := ioutil.ReadAll(rc)
	if err != nil {
		return "", err
	}
	bAttrs, err := bkt.Attrs(context.Background())
	if err != nil {
		return "", err
	}
	// strip gs://<bucket-name> from global address `u`
	u := string(data)
	i := strings.Index(u, bAttrs.Name)
	dir := u[i+len(bAttrs.Name)+1:] // +1 for leading slash
	return dir, nil
}

func getDir(bkt *storage.BucketHandle, root, id string) string {
	bAttrs, err := bkt.Attrs(context.Background())
	if err != nil {
		return ""
	}
	bktName := bAttrs.Name
	if strings.HasPrefix(root, logsPrefix) {
		return path.Join(bktName, root, id)
	}
	symLink := path.Join(root, id+".txt")
	dir, err := resolveSymLink(bkt, symLink)
	if err != nil {
		return ""
	}
	return path.Join(bktName, dir)
}

func getObject(bkt *storage.BucketHandle, root, id, fname string) *storage.ObjectHandle {
	if strings.HasPrefix(root, logsPrefix) {
		p := path.Join(root, id, fname)
		return bkt.Object(p)
	}
	symLink := path.Join(root, id+".txt")
	dir, err := resolveSymLink(bkt, symLink)
	if err != nil {
		return &storage.ObjectHandle{}
	}
	return bkt.Object(path.Join(dir, fname))
}

func fileExists(bkt *storage.BucketHandle, root, id, fname string) bool {
	obj := getObject(bkt, root, id, fname)
	_, err := obj.Attrs(context.Background())
	return err == nil
}

func readStarted(bkt *storage.BucketHandle, root, id string) (started, error) {
	s := started{}
	sobj := getObject(bkt, root, id, "started.json")
	sr, err := sobj.NewReader(context.Background())
	if err != nil {
		return s, fmt.Errorf("Failed to get reader for started.json for build %s: %v", id, err)
	}
	sdata, err := ioutil.ReadAll(sr)
	if err != nil {
		return s, fmt.Errorf("Failed to read started.json for build %s: %v", id, err)
	}
	err = json.Unmarshal(sdata, &s)
	if err != nil {
		return s, fmt.Errorf("Failed to parse started.json for build %s: %v", id, err)
	}
	return s, nil
}

func readFinished(bkt *storage.BucketHandle, root, id string) (finished, error) {
	f := finished{}
	fobj := getObject(bkt, root, id, "finished.json")
	fr, err := fobj.NewReader(context.Background())
	if err != nil {
		return f, fmt.Errorf("Failed to get reader for finished.json for build %s: %v", id, err)
	}
	fdata, err := ioutil.ReadAll(fr)
	if err != nil {
		return f, fmt.Errorf("Failed to read finished.json for build %s: %v", id, err)
	}
	err = json.Unmarshal(fdata, &f)
	if err != nil {
		return f, fmt.Errorf("Failed to parse finished.json for build %s: %v", id, err)
	}
	return f, nil
}

// Gets job history from the GCS bucket specified in config.
func getJobHistory(url *url.URL, config *config.Config, gcsClient *storage.Client) (JobHistoryTemplate, error) {
	start := time.Now()

	jobName := strings.TrimPrefix(url.Path, "/job-history/")
	tmpl := JobHistoryTemplate{
		Name:   jobName,
		Builds: make([]BuildData, resultsPerPage),
	}

	var latest int
	bucketName := config.ProwConfig.Plank.DefaultDecorationConfig.GCSConfiguration.Bucket
	bkt := gcsClient.Bucket(bucketName)
	var root string
	found := false
	for _, r := range []string{logsPrefix, symLinkPrefix} {
		root = path.Join(r, jobName)
		loc := path.Join(root, "latest-build.txt")
		obj := bkt.Object(loc)
		n, err := readLatestBuild(obj)
		if err == nil {
			latest = n
			found = true
			break
		}
	}
	if !found {
		return tmpl, fmt.Errorf("Failed to locate build data")
	}
	var top, bottom int // build ids of the top (inclusive) and bottom (exclusive) results
	if idVals := url.Query()[idParam]; len(idVals) >= 1 {
		var err error
		if top, err = strconv.Atoi(idVals[0]); err != nil {
			return tmpl, fmt.Errorf("Invalid value for %s: %v", idParam, err)
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
		q.Set(idParam, strconv.Itoa(latest))
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

	bch := make(chan BuildData)
	for i := top; i > bottom; i-- {
		go func(i int) {
			build := BuildData{
				index: top - i,
				ID:    strconv.Itoa(i),
			}
			dir := getDir(bkt, root, build.ID)
			if dir != "" {
				build.SpyglassLink = path.Join("/view/gcs", dir)
			}
			started, err := readStarted(bkt, root, build.ID)
			if err == nil {
				build.Started = time.Unix(started.Timestamp, 0)
				finished, err := readFinished(bkt, root, build.ID)
				if err == nil {
					build.Duration = time.Unix(finished.Timestamp, 0).Sub(build.Started)
					build.Result = finished.Result
				} else {
					build.Result = "Unfinished"
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
