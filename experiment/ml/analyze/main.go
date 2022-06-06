/*
Copyright 2022 The Kubernetes Authors.

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
	"errors"
	"flag"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"time"

	"cloud.google.com/go/storage"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
)

var (
	location       = flag.String("region", "", "Use a model in the specified region")
	model          = flag.String("model", "", "Use the specified model ID")
	projectID      = flag.String("project", "", "Use a model in the specified project")
	quotaProjectID = flag.String("quota-project", "", "Specify a quota project to charge")

	buildURL = flag.String("build", "", "Paste in a full prow https build URL instead")

	sentenceLen = flag.Int("max-sentence-length", 600, "Truncate sentences to at most this many bytes")
	documentLen = flag.Int("max-document-length", 200, "Process at most this many lines from the top/bottom of the build")

	qps    = flag.Int("qps", 6, "Limit prediction requests")
	burst  = flag.Int("burst-seconds", 4, "Allow bursts of activity for this many qps seconds")
	warmup = flag.Bool("warmup", false, "Start with the burst pool filled")

	additional = flag.Bool("additional", false, "Print all hot lines, not just the hottest")
	annotate   = flag.Bool("annotate", false, "Ask whether to annotate after predicting")
	shout      = flag.Bool("shout", false, "Make the server noisy")

	port    = flag.Int("port", 0, "Listen for annotation requests on this port")
	timeout = flag.Duration("timeout", time.Minute, "Maximum time to answer a request")
)

func main() {
	flag.Parse()
	var build *gcs.Path
	if *port == 0 {
		if *buildURL == "" {
			log.Fatal("--build and --port unset")
		}
		b, err := pathFromView(*buildURL)
		if err != nil {
			log.Fatalf("Could not parse --build=%q: %v", *buildURL, err)
		}
		build = b
	}
	if *projectID == "" {
		log.Fatal("--project unset")
	}
	if *location == "" {
		log.Fatal("--region unset")
	}
	if *model == "" {
		log.Fatal("--model unset")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	storageClient, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatalf("Could not create GCS client: %v", err)
	}

	predictor, err := defaultPredictionClient(ctx)
	if err != nil {
		log.Fatalf("Failed to create predictor: %v", err)
	}
	defer predictor.client.Close()

	if *port > 0 {
		if err := serveOnPort(ctx, storageClient, predictor, *port, *timeout); err != nil {
			log.Fatalf("Serve failed: %v", err)
		}
		return
	}

	gcsClient := gcs.NewClient(storageClient)

	lines, _, err := annotateBuild(ctx, gcsClient, predictor, *build)
	if err != nil {
		log.Fatalf("Failed to annotate build: %v", err)
	}

	if *annotate && lines != nil {
		if err := askSaveLines(ctx, storageClient, *build, lines); err != nil {
			log.Fatalf("Failed to save lines: %v", err)
		}
	}
}

var (
	buildRE = regexp.MustCompile(`https?://[^/]+/view/gc?s/(.+?)/?$`)
)

func pathFromView(view string) (*gcs.Path, error) {
	mat := buildRE.FindStringSubmatch(view)
	if mat == nil {
		return nil, errors.New("--build must match https://HOST/view/gs/PREFIX/JOB/BUILD")
	}
	return gcs.NewPath("gs://" + mat[1] + "/build-log.txt")
}

func askSaveLines(ctx context.Context, storageClient *storage.Client, build gcs.Path, lines []int) error {
	min, max := minMax(lines)
	fmt.Printf("Annotate lines %d-%d of %s? [y/N] ", min, max, build)
	var answer string
	_, _ = fmt.Scanln(&answer) // intentionally ignore error on just enter
	if answer == "" || (answer[0] != 'y' && answer[0] != 'Y') {
		return nil
	}

	return saveLines(ctx, storageClient, build, min, max)
}

const (
	focusStart = "focus-start"
	focusEnd   = "focus-end"
)

func saveLines(ctx context.Context, storageClient *storage.Client, path gcs.Path, min, max int) error {
	if min > max {
		max, min = min, max
	}

	meta := map[string]string{
		focusStart: "",
		focusEnd:   "",
	}

	if min > 0 {
		meta[focusStart] = strconv.Itoa(min)
	}
	if max > 0 {
		meta[focusEnd] = strconv.Itoa(max)
	}

	attrs := storage.ObjectAttrsToUpdate{Metadata: meta}
	if _, err := storageClient.Bucket(path.Bucket()).Object(path.Object()).Update(ctx, attrs); err != nil {
		return err
	}
	log.Println("Annotated", path, "to focus on lines", min, "-", max)
	return nil
}
