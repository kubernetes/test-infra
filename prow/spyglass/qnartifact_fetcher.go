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

package spyglass

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/qiniu/api.v7/auth"
	qc "github.com/qiniu/api.v7/client"
	"github.com/qiniu/api.v7/storage"
	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/qiniu"

	"k8s.io/test-infra/prow/spyglass/lenses"
)

// QNArtifactFetcher contains information used for fetching artifacts from GCS
type QNArtifactFetcher struct {
	bm     *storage.BucketManager
	config *qiniu.Config
}

// NewQNArtifactFetcher creates a new ArtifactFetcher
func NewQNArtifactFetcher(config *qiniu.Config, client *qc.Client) *QNArtifactFetcher {
	return &QNArtifactFetcher{
		config: config,
		bm:     storage.NewBucketManagerEx(auth.New(config.AccessKey, config.SecretKey), nil, client),
	}
}

func (af *QNArtifactFetcher) splitKey(src string) (string, error) {
	split := strings.SplitN(src, "/", 2)
	if len(split) < 2 {
		return "", fmt.Errorf("invalid src %s: expected <bucket>/<key>", src)
	}
	return split[1], nil
}

// Artifacts lists all artifacts available for the given job source
func (af *QNArtifactFetcher) artifacts(key string) ([]string, error) {
	listStart := time.Now()
	ctx := context.Background()
	var artifacts []string
	var marker string
	wait := []time.Duration{16, 32, 64, 128, 256, 256, 512, 512}
	for i := 0; ; {
		entries, err := af.bm.ListBucketContext(ctx, af.config.Bucket, key, "", marker)
		if err != nil {
			logrus.WithField("key", key).WithError(err).Error("Error accessing QINIU artifact.")
			if i >= len(wait) {
				return artifacts, fmt.Errorf("timed out: error accessing GCS artifact: %v", err)
			}
			time.Sleep((wait[i] + time.Duration(rand.Intn(10))) * time.Millisecond)
			i++
			continue
		}

		for entry := range entries {
			artifacts = append(artifacts, entry.Item.Key)
			marker = entry.Marker
		}

		if marker != "" {
			i = 0
		}
		break
	}
	logrus.WithField("duration", time.Since(listStart).String()).Infof("Listed %d artifacts.", len(artifacts))
	return artifacts, nil
}

func (af *QNArtifactFetcher) signURL(bucket, obj string) (string, error) {
	return "signed_fake_url", nil
}

// Artifact constructs a GCS artifact from the given GCS bucket and key. Uses the golang GCS library
// to get read handles. If the artifactName is not a valid key in the bucket a handle will still be
// constructed and returned, but all read operations will fail (dictated by behavior of golang GCS lib).
func (af *QNArtifactFetcher) artifact(artifactName string, sizeLimit int64) (lenses.Artifact, error) {
	obj := qiniu.NewQiniuObject(af.config, artifactName, af.bm)

	deadline := time.Now().Add(time.Second * 60 * 10).Unix()
	accessUrl := storage.MakePrivateURL(auth.New(af.config.AccessKey, af.config.SecretKey), af.config.Domain, artifactName, deadline)

	return NewQNArtifact(context.Background(), obj, accessUrl, artifactName, sizeLimit), nil
}
