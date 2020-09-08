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

package common

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/io/providers"
	"k8s.io/test-infra/prow/spyglass/api"
)

var lensTemplate = template.Must(template.New("sg").Parse(string(MustAsset("static/spyglass-lens.html"))))
var buildLogRegex = regexp.MustCompile(`^(?:[^/]*-)?build-log\.txt$`)

type LensWithConfiguration struct {
	Config LensOpt
	Lens   api.Lens
}

func NewLensServer(
	listenAddress string,
	pjFetcher ProwJobFetcher,
	storageArtifactFetcher ArtifactFetcher,
	podLogArtifactFetcher ArtifactFetcher,
	cfg config.Getter,
	lenses []LensWithConfiguration,
) (*http.Server, error) {

	mux := http.NewServeMux()

	seenLens := sets.String{}
	for _, lens := range lenses {
		if seenLens.Has(lens.Config.LensName) {
			return nil, fmt.Errorf("duplicate lens named %q", lens.Config.LensName)
		}
		seenLens.Insert(lens.Config.LensName)

		logrus.WithField("Lens", lens.Config.LensName).Info("Adding handler for lens")
		opt := lensHandlerOpts{
			PJFetcher:              pjFetcher,
			StorageArtifactFetcher: storageArtifactFetcher,
			PodLogArtifactFetcher:  podLogArtifactFetcher,
			ConfigGetter:           cfg,
			LensOpt:                lens.Config,
		}
		mux.Handle(DyanmicPathForLens(lens.Config.LensName), newLensHandler(lens.Lens, opt))
	}
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logrus.WithField("path", r.URL.Path).Error("LensServer got request on unhandled path")
		http.NotFound(w, r)
	}))

	return &http.Server{Addr: listenAddress, Handler: mux}, nil
}

type LensOpt struct {
	LensResourcesDir string
	LensName         string
	LensTitle        string
}

type lensHandlerOpts struct {
	PJFetcher              ProwJobFetcher
	StorageArtifactFetcher ArtifactFetcher
	PodLogArtifactFetcher  ArtifactFetcher
	ConfigGetter           config.Getter
	LensOpt
}

func newLensHandler(lens api.Lens, opts lensHandlerOpts) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			writeHTTPError(w, fmt.Errorf("failed to read request body: %w", err), http.StatusInternalServerError)
			return
		}

		request := &api.LensRequest{}
		if err := json.Unmarshal(body, request); err != nil {
			writeHTTPError(w, fmt.Errorf("failed to unmarshal request: %w", err), http.StatusBadRequest)
			return
		}

		artifacts, err := FetchArtifacts(r.Context(), opts.PJFetcher, opts.ConfigGetter, opts.StorageArtifactFetcher, opts.PodLogArtifactFetcher, request.ArtifactSource, "", opts.ConfigGetter().Deck.Spyglass.SizeLimit, request.Artifacts)
		if err != nil || len(artifacts) == 0 {
			statusCode := http.StatusInternalServerError
			if len(artifacts) == 0 {
				statusCode = http.StatusNotFound
				err = errors.New("no artifacts found")
			}

			writeHTTPError(w, fmt.Errorf("failed to retrieve expected artifacts: %w", err), statusCode)
			return
		}

		switch request.Action {
		case api.RequestActionInitial:
			w.Header().Set("Content-Type", "text/html; encoding=utf-8")
			lensTemplate.Execute(w, struct {
				Title   string
				BaseURL string
				Head    template.HTML
				Body    template.HTML
			}{
				opts.LensTitle,
				request.ResourceRoot,
				template.HTML(lens.Header(artifacts, opts.LensResourcesDir, opts.ConfigGetter().Deck.Spyglass.Lenses[request.LensIndex].Lens.Config)),
				template.HTML(lens.Body(artifacts, opts.LensResourcesDir, "", opts.ConfigGetter().Deck.Spyglass.Lenses[request.LensIndex].Lens.Config)),
			})

		case api.RequestActionRerender:
			w.Header().Set("Content-Type", "text/html; encoding=utf-8")
			w.Write([]byte(lens.Body(artifacts, opts.LensResourcesDir, request.Data, opts.ConfigGetter().Deck.Spyglass.Lenses[request.LensIndex].Lens.Config)))

		case api.RequestActionCallBack:
			w.Write([]byte(lens.Callback(artifacts, opts.LensResourcesDir, request.Data, opts.ConfigGetter().Deck.Spyglass.Lenses[request.LensIndex].Lens.Config)))

		default:
			w.WriteHeader(http.StatusBadRequest)
			// This is a bit weird as we proxy this and the request we are complaining about was issued by Deck, not by the original client that sees this error
			w.Write([]byte(fmt.Sprintf("Invalid action %q", request.Action)))
		}
	}
}

func writeHTTPError(w http.ResponseWriter, err error, statusCode int) {
	if statusCode == 0 {
		statusCode = http.StatusInternalServerError
	}
	logrus.WithError(err).WithField("statusCode", statusCode).Debug("Failed to process request")
	w.WriteHeader(statusCode)
	if _, err := w.Write([]byte(err.Error())); err != nil {
		logrus.WithError(err).Error("Failed to write response")
	}
}

// ArtifactFetcher knows how to fetch artifacts
type ArtifactFetcher interface {
	Artifact(ctx context.Context, key string, artifactName string, sizeLimit int64) (api.Artifact, error)
}

// FetchArtifacts fetches artifacts.
// TODO: Unexport once we only have remote lenses
func FetchArtifacts(
	ctx context.Context,
	pjFetcher ProwJobFetcher,
	cfg config.Getter,
	storageArtifactFetcher ArtifactFetcher,
	podLogArtifactFetcher ArtifactFetcher,
	src string,
	podName string,
	sizeLimit int64,
	artifactNames []string,
) ([]api.Artifact, error) {
	artStart := time.Now()
	arts := []api.Artifact{}
	keyType, key, err := splitSrc(src)
	if err != nil {
		return arts, fmt.Errorf("error parsing src: %v", err)
	}
	gcsKey := ""
	switch keyType {
	case api.ProwKeyType:
		storageProvider, key, err := ProwToGCS(pjFetcher, cfg, key)
		if err != nil {
			logrus.Warningln(err)
		}
		gcsKey = fmt.Sprintf("%s://%s", storageProvider, strings.TrimSuffix(key, "/"))
	default:
		if keyType == api.GCSKeyType {
			keyType = providers.GS
		}
		gcsKey = fmt.Sprintf("%s://%s", keyType, strings.TrimSuffix(key, "/"))
	}

	logsNeeded := []string{}

	for _, name := range artifactNames {
		art, err := storageArtifactFetcher.Artifact(ctx, gcsKey, name, sizeLimit)
		if err == nil {
			// Actually try making a request, because calling StorageArtifactFetcher.artifact does no I/O.
			// (these files are being explicitly requested and so will presumably soon be accessed, so
			// the extra network I/O should not be too problematic).
			_, err = art.Size()
		}
		if err != nil {
			if buildLogRegex.MatchString(name) {
				logsNeeded = append(logsNeeded, name)
			}
			continue
		}
		arts = append(arts, art)
	}

	for _, logName := range logsNeeded {
		art, err := podLogArtifactFetcher.Artifact(ctx, src, logName, sizeLimit)
		if err != nil {
			logrus.Errorf("Failed to fetch pod log: %v", err)
		} else {
			arts = append(arts, art)
		}
	}

	logrus.WithField("duration", time.Since(artStart).String()).Infof("Retrieved artifacts for %v", src)
	return arts, nil
}

// ProwJobFetcher knows how to get a ProwJob
type ProwJobFetcher interface {
	GetProwJob(job string, id string) (prowv1.ProwJob, error)
}

// prowToGCS returns the GCS key corresponding to the given prow key
// TODO: Unexport once we only have remote lenses
func ProwToGCS(fetcher ProwJobFetcher, config config.Getter, prowKey string) (string, string, error) {
	jobName, buildID, err := KeyToJob(prowKey)
	if err != nil {
		return "", "", fmt.Errorf("could not get GCS src: %v", err)
	}

	job, err := fetcher.GetProwJob(jobName, buildID)
	if err != nil {
		return "", "", fmt.Errorf("failed to get prow job from src %q: %v", prowKey, err)
	}

	url := job.Status.URL
	prefix := config().Plank.GetJobURLPrefix(job.Spec.Refs)
	if !strings.HasPrefix(url, prefix) {
		return "", "", fmt.Errorf("unexpected job URL %q when finding GCS path: expected something starting with %q", url, prefix)
	}

	// example:
	// * url: https://prow.k8s.io/view/gs/kubernetes-jenkins/logs/ci-benchmark-microbenchmarks/1258197944759226371
	// * prefix: https://prow.k8s.io/view/
	// * storagePath: gs/kubernetes-jenkins/logs/ci-benchmark-microbenchmarks/1258197944759226371
	storagePath := strings.TrimPrefix(url, prefix)
	if strings.HasPrefix(storagePath, api.GCSKeyType) {
		storagePath = strings.Replace(storagePath, api.GCSKeyType, providers.GS, 1)
	}
	storagePathWithoutProvider := storagePath
	storagePathSegments := strings.SplitN(storagePath, "/", 2)
	if providers.HasStorageProviderPrefix(storagePath) {
		storagePathWithoutProvider = storagePathSegments[1]
	}

	// try to parse storageProvider from DecorationConfig.GCSConfiguration.Bucket
	// if it doesn't work fallback to URL parsing
	if job.Spec.DecorationConfig != nil && job.Spec.DecorationConfig.GCSConfiguration != nil {
		prowPath, err := prowv1.ParsePath(job.Spec.DecorationConfig.GCSConfiguration.Bucket)
		if err == nil {
			return prowPath.StorageProvider(), storagePathWithoutProvider, nil
		}
		logrus.Warnf("Could not parse storageProvider from DecorationConfig.GCSConfiguration.Bucket = %s: %v", job.Spec.DecorationConfig.GCSConfiguration.Bucket, err)
	}

	return storagePathSegments[0], storagePathWithoutProvider, nil
}

func splitSrc(src string) (keyType, key string, err error) {
	split := strings.SplitN(src, "/", 2)
	if len(split) < 2 {
		err = fmt.Errorf("invalid src %s: expected <key-type>/<key>", src)
		return
	}
	keyType = split[0]
	key = split[1]
	return
}

// keyToJob takes a spyglass URL and returns the jobName and buildID.
func KeyToJob(src string) (jobName string, buildID string, err error) {
	src = strings.Trim(src, "/")
	parsed := strings.Split(src, "/")
	if len(parsed) < 2 {
		return "", "", fmt.Errorf("expected at least two path components in %q", src)
	}
	jobName = parsed[len(parsed)-2]
	buildID = parsed[len(parsed)-1]
	return jobName, buildID, nil
}

const prefixSpyglassDynamicHandlers = "dynamic"

func DyanmicPathForLens(lensName string) string {
	return fmt.Sprintf("/%s/%s", prefixSpyglassDynamicHandlers, lensName)
}
