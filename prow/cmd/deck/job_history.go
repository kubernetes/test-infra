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
	"io"
	"io/ioutil"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	pkgio "k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/io/providers"
	"k8s.io/test-infra/prow/pod-utils/gcs"
)

const (
	resultsPerPage  = 20
	idParam         = "buildId"
	latestBuildFile = "latest-build.txt"

	// ** Job history assumes the GCS layout specified here:
	// https://github.com/kubernetes/test-infra/tree/master/gubernator#gcs-bucket-layout
	logsPrefix     = gcs.NonPRLogs
	spyglassPrefix = "/view"
	emptyID        = int64(-1) // indicates no build id was specified
)

var (
	linkRe = regexp.MustCompile("/([0-9]+)\\.txt$")
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
	getStorageProvider() string
	listSubDirs(ctx context.Context, prefix string) ([]string, error)
	listAll(ctx context.Context, prefix string) ([]string, error)
	readObject(ctx context.Context, key string) ([]byte, error)
}

// blobStorageBucket is our real implementation of storageBucket.
// Use `newBlobStorageBucket` to instantiate (includes bucket-level validation).
type blobStorageBucket struct {
	name            string
	storageProvider string
	pkgio.Opener
}

// newBlobStorageBucket validates the bucketName and returns a new instance of blobStorageBucket.
func newBlobStorageBucket(bucketName, storageProvider string, config *config.Config, opener pkgio.Opener) (blobStorageBucket, error) {
	if err := config.ValidateStorageBucket(bucketName); err != nil {
		return blobStorageBucket{}, fmt.Errorf("could not instantiate storage bucket: %v", err)
	}
	return blobStorageBucket{bucketName, storageProvider, opener}, nil
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

func (bucket blobStorageBucket) readObject(ctx context.Context, key string) ([]byte, error) {
	rc, err := bucket.Opener.Reader(ctx, fmt.Sprintf("%s://%s/%s", bucket.storageProvider, bucket.name, key))
	if err != nil {
		return nil, fmt.Errorf("creating reader for object %s: %w", key, err)
	}
	defer rc.Close()
	return ioutil.ReadAll(rc)
}

func (bucket blobStorageBucket) getName() string {
	return bucket.name
}

func (bucket blobStorageBucket) getStorageProvider() string {
	return bucket.storageProvider
}

func readLatestBuild(ctx context.Context, bucket storageBucket, root string) (int64, error) {
	key := path.Join(root, latestBuildFile)
	data, err := bucket.readObject(ctx, key)
	if err != nil {
		return -1, fmt.Errorf("failed to read %s: %w", key, err)
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return -1, fmt.Errorf("failed to parse %s: %v", key, err)
	}
	return n, nil
}

// resolve symlinks into the actual log directory for a particular test run, e.g.:
// * input:  gs://prow-artifacts/pr-logs/pull/cluster-api-provider-openstack/1687/bazel-build/1248207834168954881
// * output: pr-logs/pull/cluster-api-provider-openstack/1687/bazel-build/1248207834168954881
func (bucket blobStorageBucket) resolveSymLink(ctx context.Context, symLink string) (string, error) {
	data, err := bucket.readObject(ctx, symLink)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", symLink, err)
	}
	// strip gs://<bucket-name> from global address `u`
	u := strings.TrimSpace(string(data))
	parsedURL, err := url.Parse(u)
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(parsedURL.Path, "/"), nil
}

func (bucket blobStorageBucket) spyglassLink(ctx context.Context, root, id string) (string, error) {
	p, err := bucket.getPath(ctx, root, id, "")
	if err != nil {
		return "", fmt.Errorf("failed to get path: %v", err)
	}
	return path.Join(spyglassPrefix, bucket.storageProvider, bucket.name, p), nil
}

func (bucket blobStorageBucket) getPath(ctx context.Context, root, id, fname string) (string, error) {
	if strings.HasPrefix(root, logsPrefix) {
		return path.Join(root, id, fname), nil
	}
	symLink := path.Join(root, id+".txt")
	dir, err := bucket.resolveSymLink(ctx, symLink)
	if err != nil {
		return "", fmt.Errorf("failed to resolve sym link: %w", err)
	}
	return path.Join(dir, fname), nil
}

// reads specified JSON file in to `data`
func readJSON(ctx context.Context, bucket storageBucket, key string, data interface{}) error {
	rawData, err := bucket.readObject(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to read %s: %v", key, err)
	}
	err = json.Unmarshal(rawData, &data)
	if err != nil {
		return fmt.Errorf("failed to parse %s: %v", key, err)
	}
	return nil
}

// Lists the "directory paths" immediately under prefix.
func (bucket blobStorageBucket) listSubDirs(ctx context.Context, prefix string) ([]string, error) {
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	it, err := bucket.Opener.Iterator(ctx, fmt.Sprintf("%s://%s/%s", bucket.storageProvider, bucket.name, prefix), "/")
	if err != nil {
		return nil, err
	}

	dirs := []string{}
	for {
		attrs, err := it.Next(ctx)
		if err != nil {
			if err == io.EOF {
				break
			}
			return dirs, err
		}
		if attrs.IsDir {
			dirs = append(dirs, attrs.Name)
		}
	}
	return dirs, nil
}

// Lists all keys with given prefix.
func (bucket blobStorageBucket) listAll(ctx context.Context, prefix string) ([]string, error) {
	it, err := bucket.Opener.Iterator(ctx, fmt.Sprintf("%s://%s/%s", bucket.storageProvider, bucket.name, prefix), "")
	if err != nil {
		return nil, err
	}

	keys := []string{}
	for {
		attrs, err := it.Next(ctx)
		if err != nil {
			if err == io.EOF {
				break
			}
			return keys, err
		}
		keys = append(keys, attrs.Name)
	}
	return keys, nil
}

// Gets all build ids for a job.
func (bucket blobStorageBucket) listBuildIDs(ctx context.Context, root string) ([]int64, error) {
	ids := []int64{}
	if strings.HasPrefix(root, logsPrefix) {
		dirs, err := bucket.listSubDirs(ctx, root)
		if err != nil {
			return ids, fmt.Errorf("failed to list directories: %v", err)
		}
		for _, dir := range dirs {
			leaf := path.Base(dir)
			i, err := strconv.ParseInt(leaf, 10, 64)
			if err == nil {
				ids = append(ids, i)
			} else {
				logrus.WithField("gcs-path", dir).Warningf("unrecognized directory name (expected int64): %s", leaf)
			}
		}
	} else {
		keys, err := bucket.listAll(ctx, root)
		if err != nil {
			return ids, fmt.Errorf("failed to list keys: %v", err)
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

// parseJobHistURL parses the job History URL
// example urls:
// * new format: https://prow.k8s.io/job-history/gs/kubernetes-jenkins/pr-logs/directory/pull-capi?buildId=1245584383100850177
// * old format: https://prow.k8s.io/job-history/kubernetes-jenkins/pr-logs/directory/pull-capi?buildId=1245584383100850177
// Newly generated URLs will include the storageProvider. We still support old URLs so they don't break.
// For old URLs we assume that the storageProvider is `gs`.
// examples return values:
// * storageProvider: gs, s3
// * bucketName: kubernetes-jenkins
// * root: pr-logs/directory/pull-capi
// * buildID: 1245584383100850177
func parseJobHistURL(url *url.URL) (storageProvider, bucketName, root string, buildID int64, err error) {
	buildID = emptyID
	p := strings.TrimPrefix(url.Path, "/job-history/")
	// examples for p:
	// * new format: gs/kubernetes-jenkins/pr-logs/directory/pull-cluster-api-provider-openstack-test
	// * old format: kubernetes-jenkins/pr-logs/directory/pull-cluster-api-provider-openstack-test

	// inject gs/ if old format is used
	if !providers.HasStorageProviderPrefix(p) {
		p = fmt.Sprintf("%s/%s", providers.GS, p)
	}

	// handle new format
	s := strings.SplitN(p, "/", 3)
	if len(s) < 3 {
		err = fmt.Errorf("invalid path (expected either /job-history/<gcs-path> or /job-history/<storage-type>/<storage-path>): %v", url.Path)
		return
	}
	storageProvider = s[0]
	bucketName = s[1]
	root = s[2] // `root` is the root "directory" prefix for this job's results

	if bucketName == "" {
		err = fmt.Errorf("missing bucket name: %v", url.Path)
		return
	}
	if root == "" {
		err = fmt.Errorf("invalid path for job: %v", url.Path)
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

func getBuildData(ctx context.Context, bucket storageBucket, dir string) (buildData, error) {
	b := buildData{
		Result:     "Unknown",
		commitHash: "Unknown",
	}
	started := gcs.Started{}
	err := readJSON(ctx, bucket, path.Join(dir, prowv1.StartedStatusFile), &started)
	if err != nil {
		return b, fmt.Errorf("failed to read started.json: %v", err)
	}
	b.Started = time.Unix(started.Timestamp, 0)
	if commitHash, err := getPullCommitHash(started.Pull); err == nil {
		b.commitHash = commitHash
	}
	finished := gcs.Finished{}
	err = readJSON(ctx, bucket, path.Join(dir, prowv1.FinishedStatusFile), &finished)
	if err != nil {
		b.Result = "Pending"
		logrus.Debugf("failed to read finished.json (job might be unfinished): %v", err)
	}
	// Testgrid metadata.Finished is deprecating the Revision field, however
	// the actual finished.json is still using revision and maps to DeprecatedRevision.
	// TODO(ttyang): update both to match when fejta completely removes DeprecatedRevision.
	if finished.DeprecatedRevision != "" {
		b.commitHash = finished.DeprecatedRevision
	}

	if finished.Timestamp != nil {
		b.Duration = time.Unix(*finished.Timestamp, 0).Sub(b.Started)
	} else {
		b.Duration = time.Since(b.Started).Round(time.Second)
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

// Gets job history from the bucket specified in config.
func getJobHistory(ctx context.Context, url *url.URL, cfg config.Getter, opener pkgio.Opener) (jobHistoryTemplate, error) {
	start := time.Now()
	tmpl := jobHistoryTemplate{}

	storageProvider, bucketName, root, top, err := parseJobHistURL(url)
	if err != nil {
		return tmpl, fmt.Errorf("invalid url %s: %v", url.String(), err)
	}
	bucket, err := newBlobStorageBucket(bucketName, storageProvider, cfg(), opener)
	if err != nil {
		return tmpl, err
	}
	tmpl.Name = root
	latest, err := readLatestBuild(ctx, bucket, root)
	if err != nil {
		return tmpl, fmt.Errorf("failed to locate build data: %v", err)
	}
	if top == emptyID || top > latest {
		top = latest
	}
	if top != latest {
		tmpl.LatestLink = linkID(url, emptyID)
	}

	buildIDs, err := bucket.listBuildIDs(ctx, root)
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
			dir, err := bucket.getPath(ctx, root, id, "")
			if err != nil {
				if !pkgio.IsNotExist(err) {
					logrus.WithError(err).Error("Failed to get path")
				}
				bch <- buildData{}
				return
			}
			b, err := getBuildData(ctx, bucket, dir)
			if err != nil {
				logrus.Warningf("build %d information incomplete: %v", buildID, err)
			}
			b.index = i
			b.ID = id
			b.SpyglassLink, err = bucket.spyglassLink(ctx, root, id)
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

	elapsed := time.Since(start)
	logrus.Infof("loaded %s in %v", url.Path, elapsed)
	return tmpl, nil
}
