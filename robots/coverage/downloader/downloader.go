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

// Package downloader finds and downloads the coverage profile file from the latest healthy build
// stored in given gcs directory
package downloader

import (
	"context"
	"fmt"
	"io/ioutil"
	"path"
	"sort"
	"strconv"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
)

const (
	//covProfileCompletionMarker is the file name of the completion marker for coverage profiling
	covProfileCompletionMarker = "profile-completed"
)

//listGcsObjects get the slice of gcs objects under a given path
func listGcsObjects(ctx context.Context, client *storage.Client, bucketName, prefix, delim string) (
	[]string, error) {

	var objects []string
	it := client.Bucket(bucketName).Objects(ctx, &storage.Query{
		Prefix:    prefix,
		Delimiter: delim,
	})

	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return objects, fmt.Errorf("error iterating: %v", err)
		}

		if attrs.Prefix != "" {
			objects = append(objects, path.Base(attrs.Prefix))
		}
	}
	logrus.Info("end of listGcsObjects(...)")
	return objects, nil
}

// doesObjectExist checks whether an object exists in GCS bucket
func doesObjectExist(ctx context.Context, client *storage.Client, bucket, object string) bool {
	_, err := client.Bucket(bucket).Object(object).Attrs(ctx)
	if err != nil {
		logrus.Infof("Error getting attrs from object '%s': %v", object, err)
		return false
	}
	return true
}

func getProfile(ctx context.Context, client *storage.Client, bucket, object string) ([]byte, error) {
	logrus.Infof("Running ProfileReader on bucket '%s', object='%s'\n",
		bucket, object)
	o := client.Bucket(bucket).Object(object)
	reader, err := o.NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot read object '%s': %v", object, err)
	}
	return ioutil.ReadAll(reader)
}

// FindBaseProfile finds the coverage profile file from the latest healthy build
// stored in given gcs directory
func FindBaseProfile(ctx context.Context, client *storage.Client, bucket, prowJobName, artifactsDirName,
	covProfileName string) ([]byte, error) {

	dirOfJob := path.Join("logs", prowJobName)

	strBuilds, err := listGcsObjects(ctx, client, bucket, dirOfJob+"/", "/")
	if err != nil {
		return nil, fmt.Errorf("error listing gcs objects: %v", err)
	}

	builds := sortBuilds(strBuilds)
	profilePath := ""
	for _, build := range builds {
		artifactsDirPath := path.Join(dirOfJob, strconv.Itoa(build), artifactsDirName)
		dirOfCompletionMarker := path.Join(artifactsDirPath, covProfileCompletionMarker)
		if doesObjectExist(ctx, client, bucket, dirOfCompletionMarker) {
			profilePath = path.Join(artifactsDirPath, covProfileName)
			break
		}
	}
	if profilePath == "" {
		return nil, fmt.Errorf("no healthy build found for job '%s' in bucket '%s'; total # builds = %v", dirOfJob, bucket, len(builds))
	}
	return getProfile(ctx, client, bucket, profilePath)
}

// sortBuilds converts all build from str to int and sorts all builds in descending order and
// returns the sorted slice
func sortBuilds(strBuilds []string) []int {
	var res []int
	for _, buildStr := range strBuilds {
		num, err := strconv.Atoi(buildStr)
		if err != nil {
			logrus.Infof("Non-int build number found: '%s'", buildStr)
		} else {
			res = append(res, num)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(res)))
	logrus.Infof("Sorted Builds: %v\n", res)
	return res
}
