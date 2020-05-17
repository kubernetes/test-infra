/*
Copyright 2020 The Kubernetes Authors.

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

package util

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/googleapi"

	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/gcsupload"
	pkgio "k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/io/providers"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
)

type Author interface {
	NewWriter(ctx context.Context, bucket, path string, overwrite bool) (io.WriteCloser, error)
}

type StorageAuthor struct {
	Client *storage.Client
	Opener pkgio.Opener
}

func (sa StorageAuthor) NewWriter(ctx context.Context, bucket, path string, overwrite bool) (io.WriteCloser, error) {
	pp, err := v1.ParsePath(bucket)
	if err != nil {
		return nil, err
	}

	if pp.StorageProvider() == providers.GS {
		obj := sa.Client.Bucket(bucket).Object(path)
		if !overwrite {
			obj = obj.If(storage.Conditions{DoesNotExist: true})
		}
		return obj.NewWriter(ctx), nil
	}

	absolutePath := fmt.Sprintf("%s/%s", bucket, path)
	writer, err := sa.Opener.Writer(ctx, absolutePath)
	if err != nil {
		return nil, err
	}
	return &noOverwriteWriter{ctx, sa.Opener, writer, absolutePath}, nil
}

type noOverwriteWriter struct {
	ctx    context.Context
	opener pkgio.Opener
	io.WriteCloser
	path string
}

var PreconditionFileAlreadyExistsFailed = fmt.Errorf("error file already exists")

func (w *noOverwriteWriter) Write(p []byte) (int, error) {
	_, err := w.opener.Reader(w.ctx, w.path)
	// we got an error, but not file not exists
	if err != nil && !pkgio.IsNotExist(err) {
		return 0, err
	}
	// we got no error => file already exists
	if err == nil {
		return 0, PreconditionFileAlreadyExistsFailed
	}

	return w.WriteCloser.Write(p)
}

func WriteContent(ctx context.Context, logger *logrus.Entry, author Author, bucket, path string, overwrite bool, content []byte) error {
	logger.WithFields(logrus.Fields{"bucket": bucket, "path": path}).Debugf("Uploading to %s/%s; overwrite: %v", bucket, path, overwrite)
	w, err := author.NewWriter(ctx, bucket, path, overwrite)
	if err != nil {
		return err
	}
	_, err = w.Write(content)
	var reportErr error
	if isErrUnexpected(err) {
		reportErr = err
		logger.WithError(err).WithFields(logrus.Fields{"bucket": bucket, "path": path}).Warn("Uploading info to GCS failed (write)")
	}
	err = w.Close()
	if isErrUnexpected(err) {
		reportErr = err
		logger.WithError(err).WithFields(logrus.Fields{"bucket": bucket, "path": path}).Warn("Uploading info to GCS failed (close)")
	}
	return reportErr
}

func isErrUnexpected(err error) bool {
	if err == nil {
		return false
	}
	// Precondition Failed is expected and we can silently ignore it.
	if e, ok := err.(*googleapi.Error); ok {
		if e.Code == http.StatusPreconditionFailed {
			return false
		}
	}
	// Precondition file already exists is expected
	if err == PreconditionFileAlreadyExistsFailed {
		return false
	}

	return true
}

func GetJobDestination(cfg config.Getter, pj *v1.ProwJob) (bucket, dir string, err error) {
	// We can't divine a destination for jobs that don't have a build ID, so don't try.
	if pj.Status.BuildID == "" {
		return "", "", errors.New("cannot get job destination for job with no BuildID")
	}
	// The decoration config is always provided for decorated jobs, but many
	// jobs are not decorated, so we guess that we should use the default location
	// for those jobs. This assumption is usually (but not always) correct.
	// The TestGrid configurator uses the same assumption.
	repo := "*"
	if pj.Spec.Refs != nil {
		repo = pj.Spec.Refs.Org + "/" + pj.Spec.Refs.Repo
	}

	ddc := cfg().Plank.GetDefaultDecorationConfigs(repo)

	var gcsConfig *v1.GCSConfiguration
	if pj.Spec.DecorationConfig != nil && pj.Spec.DecorationConfig.GCSConfiguration != nil {
		gcsConfig = pj.Spec.DecorationConfig.GCSConfiguration
	} else if ddc != nil && ddc.GCSConfiguration != nil {
		gcsConfig = ddc.GCSConfiguration
	} else {
		return "", "", fmt.Errorf("couldn't figure out a GCS config for %q", pj.Spec.Job)
	}

	ps := downwardapi.NewJobSpec(pj.Spec, pj.Status.BuildID, pj.Name)
	_, d, _ := gcsupload.PathsForJob(gcsConfig, &ps, "")

	return gcsConfig.Bucket, d, nil
}
