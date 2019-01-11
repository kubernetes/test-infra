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
	"k8s.io/test-infra/prow/pod-utils/gcs"
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
	jobName      string
	prefix       string
	SpyglassLink string
	ID           string
	Started      time.Time
	Duration     time.Duration
	Result       string
	commitHash   string
}

// storageBucket is an abstraction for unit testing
type storageBucket interface {
	getName() string
	listSubDirs(prefix string) ([]string, error)
	listAll(prefix string) ([]string, error)
	readObject(key string) ([]byte, error)
}

// gcsBucket is our real implementation of storageBucket
type gcsBucket struct {
	name string
	*storage.BucketHandle
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

func (bucket gcsBucket) readObject(key string) ([]byte, error) {
	obj := bucket.Object(key)
	rc, err := obj.NewReader(context.Background())
	if err != nil {
		return []byte{}, fmt.Errorf("failed to get reader for GCS object: %v", err)
	}
	return ioutil.ReadAll(rc)
}

func (bucket gcsBucket) getName() string {
	return bucket.name
}

func readLatestBuild(bucket storageBucket, root string) (int64, error) {
	key := path.Join(root, latestBuildFile)
	data, err := bucket.readObject(key)
	if err != nil {
		return -1, fmt.Errorf("failed to read %s: %v", key, err)
	}
	n, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		return -1, fmt.Errorf("failed to parse %s: %v", key, err)
	}
	return n, nil
}

// resolve sym links into the actual log directory for a particular test run
func (bucket gcsBucket) resolveSymLink(symLink string) (string, error) {
	data, err := bucket.readObject(symLink)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %v", symLink, err)
	}
	// strip gs://<bucket-name> from global address `u`
	u := string(data)
	return prefixRe.ReplaceAllString(u, ""), nil
}

func (bucket gcsBucket) spyglassLink(root, id string) (string, error) {
	p, err := bucket.getPath(root, id, "")
	if err != nil {
		return "", fmt.Errorf("failed to get path: %v", err)
	}
	return path.Join(spyglassPrefix, bucket.getName(), p), nil
}

func (bucket gcsBucket) getPath(root, id, fname string) (string, error) {
	if strings.HasPrefix(root, logsPrefix) {
		return path.Join(root, id, fname), nil
	}
	symLink := path.Join(root, id+".txt")
	dir, err := bucket.resolveSymLink(symLink)
	if err != nil {
		return "", fmt.Errorf("failed to resolve sym link: %v", err)
	}
	return path.Join(dir, fname), nil
}

// reads specified JSON file in to `data`
func readJSON(bucket storageBucket, key string, data interface{}) error {
	rawData, err := bucket.readObject(key)
	if err != nil {
		return fmt.Errorf("failed to read %s: %v", key, err)
	}
	err = json.Unmarshal(rawData, &data)
	if err != nil {
		return fmt.Errorf("failed to parse %s: %v", key, err)
	}
	return nil
}

// Lists the GCS "directory paths" immediately under prefix.
func (bucket gcsBucket) listSubDirs(prefix string) ([]string, error) {
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	dirs := []string{}
	it := bucket.Objects(context.Background(), &storage.Query{
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
func (bucket gcsBucket) listAll(prefix string) ([]string, error) {
	keys := []string{}
	it := bucket.Objects(context.Background(), &storage.Query{
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
func (bucket gcsBucket) listBuildIDs(root string) ([]int64, error) {
	ids := []int64{}
	if strings.HasPrefix(root, logsPrefix) {
		dirs, err := bucket.listSubDirs(root)
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
		keys, err := bucket.listAll(root)
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

func parseJobHistURL(url *url.URL) (bucketName, root string, buildID int64, err error) {
	buildID = emptyID
	p := strings.TrimPrefix(url.Path, "/job-history/")
	s := strings.SplitN(p, "/", 2)
	if len(s) < 2 {
		err = fmt.Errorf("invalid path (expected /job-history/<gcs-path>): %v", url.Path)
		return
	}
	bucketName = s[0]
	root = s[1] // `root` is the root GCS "directory" prefix for this job's results
	if bucketName == "" {
		err = fmt.Errorf("missing GCS bucket name: %v", url.Path)
		return
	}
	if root == "" {
		err = fmt.Errorf("invalid GCS path for job: %v", url.Path)
		return
	}

	if idVals := url.Query()[idParam]; len(idVals) >= 1 && idVals[0] != "" {
		buildID, err = strconv.ParseInt(idVals[0], 10, 64)
		if err != nil {
			err = fmt.Errorf("invalid value for %s: %v", idParam, err)
			return
		}
		if buildID < 0 {
			err = fmt.Errorf("invalid value %s = %d", idParam, buildID)
			return
		}
	}

	return
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

func getBuildData(bucket storageBucket, dir string) (buildData, error) {
	b := buildData{
		Result:     "Unknown",
		commitHash: "Unknown",
	}
	started := gcs.Started{}
	err := readJSON(bucket, path.Join(dir, "started.json"), &started)
	if err != nil {
		return b, fmt.Errorf("failed to read started.json: %v", err)
	}
	b.Started = time.Unix(started.Timestamp, 0)
	if started.Revision != "" {
		b.commitHash = started.Revision
	} else if commitHash, err := getPullCommitHash(started.Pull); err == nil {
		b.commitHash = commitHash
	}
	finished := gcs.Finished{}
	err = readJSON(bucket, path.Join(dir, "finished.json"), &finished)
	if err != nil {
		logrus.Infof("failed to read finished.json (job might be unfinished): %v", err)
	}
	if finished.Revision != "" {
		b.commitHash = finished.Revision
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

	bucketName, root, top, err := parseJobHistURL(url)
	if err != nil {
		return tmpl, fmt.Errorf("invalid url %s: %v", url.String(), err)
	}
	tmpl.Name = root
	bucket := gcsBucket{bucketName, gcsClient.Bucket(bucketName)}

	latest, err := readLatestBuild(bucket, root)
	if err != nil {
		return tmpl, fmt.Errorf("failed to locate build data: %v", err)
	}
	if top == emptyID || top > latest {
		top = latest
	}
	if top != latest {
		tmpl.LatestLink = linkID(url, emptyID)
	}

	buildIDs, err := bucket.listBuildIDs(root)
	if err != nil {
		return tmpl, fmt.Errorf("failed to get build ids: %v", err)
	}
	sort.Sort(sort.Reverse(int64slice(buildIDs)))

	// determine which results to display on this page
	shownIDs, firstIndex, lastIndex := cropResults(buildIDs, top)

	// get links to the neighboring pages
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

	// concurrently fetch data for all of the builds to be shown
	bch := make(chan buildData)
	for i, buildID := range shownIDs {
		go func(i int, buildID int64) {
			id := strconv.FormatInt(buildID, 10)
			dir, err := bucket.getPath(root, id, "")
			if err != nil {
				logrus.Errorf("failed to get path: %v", err)
				bch <- buildData{}
				return
			}
			b, err := getBuildData(bucket, dir)
			if err != nil {
				logrus.Warningf("build %d information incomplete: %v", buildID, err)
			}
			b.index = i
			b.ID = id
			b.SpyglassLink, err = bucket.spyglassLink(root, id)
			if err != nil {
				logrus.Errorf("failed to get spyglass link: %v", err)
			}
			bch <- b
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
