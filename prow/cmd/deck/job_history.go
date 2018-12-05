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
	"sort"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
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
	emptyID        = int64(-1) // indicates no build id was specified
)

var (
	prefixRe = regexp.MustCompile("gs://.*?/")
	linkRe   = regexp.MustCompile("/([0-9]+)\\.txt$")
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
	OlderLink    string
	NewerLink    string
	LatestLink   string
	Name         string
	ResultsShown int
	ResultsTotal int
	Builds       []buildData
}

func readObject(obj *storage.ObjectHandle) ([]byte, error) {
	rc, err := obj.NewReader(context.Background())
	if err != nil {
		return []byte{}, fmt.Errorf("failed to get reader for GCS object: %v", err)
	}
	return ioutil.ReadAll(rc)
}

func readLatestBuild(bkt *storage.BucketHandle, root string) (int64, error) {
	path := path.Join(root, latestBuildFile)
	data, err := readObject(bkt.Object(path))
	if err != nil {
		return -1, fmt.Errorf("failed to read %s: %v", path, err)
	}
	n, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		return -1, fmt.Errorf("failed to parse %s: %v", path, err)
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

func spyglassLink(bkt *storage.BucketHandle, bucketName, root, id string) (string, error) {
	p, err := getPath(bkt, root, id, "")
	if err != nil {
		return "", fmt.Errorf("failed to get path: %v", err)
	}
	return path.Join(spyglassPrefix, bucketName, p), nil
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

// reads specified JSON file in to `data`
func readJSON(bkt *storage.BucketHandle, root, id, fname string, data interface{}) error {
	p, err := getPath(bkt, root, id, fname)
	if err != nil {
		return fmt.Errorf("failed to get path: %v", err)
	}
	rawData, err := readObject(bkt.Object(p))
	if err != nil {
		return fmt.Errorf("failed to read %s for build %s: %v", fname, id, err)
	}
	err = json.Unmarshal(rawData, &data)
	if err != nil {
		return fmt.Errorf("failed to parse %s for build %s: %v", fname, id, err)
	}
	return nil
}

// Lists the GCS "directory paths" immediately under prefix.
func listSubDirs(bkt *storage.BucketHandle, prefix string) ([]string, error) {
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	dirs := []string{}
	it := bkt.Objects(context.Background(), &storage.Query{
		Prefix:    prefix,
		Delimiter: "/",
	})
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return dirs, err
		}
		if attrs.Prefix != "" {
			dirs = append(dirs, attrs.Prefix)
		}
	}
	return dirs, nil
}

// Lists all GCS keys with given prefix.
func listAll(bkt *storage.BucketHandle, prefix string) ([]string, error) {
	keys := []string{}
	it := bkt.Objects(context.Background(), &storage.Query{
		Prefix: prefix,
	})
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return keys, err
		}
		keys = append(keys, attrs.Name)
	}
	return keys, nil
}

// Gets all build ids for a job.
func listBuildIDs(bkt *storage.BucketHandle, root string) ([]int64, error) {
	ids := []int64{}
	if strings.HasPrefix(root, logsPrefix) {
		dirs, err := listSubDirs(bkt, root)
		if err != nil {
			return ids, fmt.Errorf("failed to list GCS directories: %v", err)
		}
		for _, dir := range dirs {
			i, err := strconv.ParseInt(path.Base(dir), 10, 64)
			if err == nil {
				ids = append(ids, i)
			} else {
				logrus.Warningf("unrecognized directory name (expected int64): %s", dir)
			}
		}
	} else {
		keys, err := listAll(bkt, root)
		if err != nil {
			return ids, fmt.Errorf("failed to list GCS keys: %v", err)
		}
		for _, key := range keys {
			matches := linkRe.FindStringSubmatch(key)
			if len(matches) == 2 {
				i, err := strconv.ParseInt(matches[1], 10, 64)
				if err == nil {
					ids = append(ids, i)
				} else {
					logrus.Warningf("unrecognized file name (expected <int64>.txt): %s", key)
				}
			}
		}
	}
	return ids, nil
}

func jobHistURL(url *url.URL) (string, string, int64, error) {
	p := strings.TrimPrefix(url.Path, "/job-history/")
	s := strings.SplitN(p, "/", 2)
	if len(s) < 2 {
		return "", "", emptyID, fmt.Errorf("invalid path (expected /job-history/<gcs-path>): %v", url.Path)
	}
	bucketName := s[0]
	root := s[1]
	if bucketName == "" {
		return bucketName, root, emptyID, fmt.Errorf("missing GCS bucket name: %v", url.Path)
	}
	if root == "" {
		return bucketName, root, emptyID, fmt.Errorf("invalid GCS path for job: %v", url.Path)
	}

	buildID := emptyID
	if idVals := url.Query()[idParam]; len(idVals) >= 1 && idVals[0] != "" {
		var err error
		buildID, err = strconv.ParseInt(idVals[0], 10, 64)
		if err != nil {
			return bucketName, root, buildID, fmt.Errorf("invalid value for %s: %v", idParam, err)
		}
		if buildID < 0 {
			return bucketName, root, buildID, fmt.Errorf("invalid value %s = %d", idParam, buildID)
		}
	}

	return bucketName, root, buildID, nil
}

func linkID(url *url.URL, id int64) string {
	u := *url
	q := u.Query()
	var val string
	if id != emptyID {
		val = strconv.FormatInt(id, 10)
	}
	q.Set(idParam, val)
	u.RawQuery = q.Encode()
	return u.String()
}

func getBuildData(bkt *storage.BucketHandle, bucketName, root string, buildID int64, index int) (buildData, error) {
	b := buildData{
		index:  index,
		ID:     strconv.FormatInt(buildID, 10),
		Result: "Unknown",
	}
	link, err := spyglassLink(bkt, bucketName, root, b.ID)
	if err != nil {
		return b, fmt.Errorf("failed to get spyglass link: %v", err)
	}
	b.SpyglassLink = link
	started := jobs.Started{}
	err = readJSON(bkt, root, b.ID, "started.json", &started)
	if err != nil {
		return b, fmt.Errorf("failed to get job metadata: %v", err)
	}
	b.Result = "Unfinished"
	b.Started = time.Unix(started.Timestamp, 0)
	finished := jobs.Finished{}
	err = readJSON(bkt, root, b.ID, "finished.json", &finished)
	if err != nil {
		logrus.Warningf("failed to read finished.json (job might be unfinished): %v", err)
	}
	if finished.Timestamp != 0 {
		b.Duration = time.Unix(finished.Timestamp, 0).Sub(b.Started)
	}
	if finished.Result != "" {
		b.Result = finished.Result
	}
	return b, nil
}

// assumes a to be sorted in descending order
// returns a subslice of a along with its indices (inclusive)
func cropResults(a []int64, max int64) ([]int64, int, int) {
	res := []int64{}
	firstIndex := -1
	lastIndex := 0
	for i, v := range a {
		if v <= max {
			res = append(res, v)
			if firstIndex == -1 {
				firstIndex = i
			}
			lastIndex = i
			if len(res) >= resultsPerPage {
				break
			}
		}
	}
	return res, firstIndex, lastIndex
}

// golang <3
type int64slice []int64

func (a int64slice) Len() int           { return len(a) }
func (a int64slice) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a int64slice) Less(i, j int) bool { return a[i] < a[j] }

// Gets job history from the GCS bucket specified in config.
func getJobHistory(url *url.URL, config *config.Config, gcsClient *storage.Client) (jobHistoryTemplate, error) {
	start := time.Now()
	tmpl := jobHistoryTemplate{}

	bucketName, root, top, err := jobHistURL(url)
	if err != nil {
		return tmpl, fmt.Errorf("invalid url %s: %v", url.String(), err)
	}
	tmpl.Name = root
	bkt := gcsClient.Bucket(bucketName)

	latest, err := readLatestBuild(bkt, root)
	if err != nil {
		return tmpl, fmt.Errorf("failed to locate build data: %v", err)
	}
	if top == emptyID || top > latest {
		top = latest
	}
	if top != latest {
		tmpl.LatestLink = linkID(url, emptyID)
	}

	buildIDs, err := listBuildIDs(bkt, root)
	if err != nil {
		return tmpl, fmt.Errorf("failed to get build ids: %v", err)
	}
	sort.Sort(sort.Reverse(int64slice(buildIDs)))

	shownIDs, firstIndex, lastIndex := cropResults(buildIDs, top)
	if firstIndex > 0 {
		nextIndex := firstIndex - resultsPerPage
		// here emptyID indicates the most recent build, which will not necessarily be buildIDs[0]
		next := emptyID
		if nextIndex >= 0 {
			next = buildIDs[nextIndex]
		}
		tmpl.NewerLink = linkID(url, next)
	}
	if lastIndex < len(buildIDs)-1 {
		tmpl.OlderLink = linkID(url, buildIDs[lastIndex+1])
	}

	tmpl.Builds = make([]buildData, len(shownIDs))
	tmpl.ResultsShown = len(shownIDs)
	tmpl.ResultsTotal = len(buildIDs)

	bch := make(chan buildData)
	for i, buildID := range shownIDs {
		go func(i int, buildID int64) {
			bd, err := getBuildData(bkt, bucketName, root, buildID, i)
			if err != nil {
				logrus.Warningf("build %d information incomplete: %v", buildID, err)
			}
			bch <- bd
		}(i, buildID)
	}
	for i := 0; i < len(shownIDs); i++ {
		b := <-bch
		tmpl.Builds[b.index] = b
	}

	elapsed := time.Now().Sub(start)
	logrus.Infof("loaded %s in %v", url.Path, elapsed)
	return tmpl, nil
}
