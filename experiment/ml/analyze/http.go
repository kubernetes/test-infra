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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
)

func serveOnPort(ctx context.Context, storageClient *storage.Client, predictor *predictionClient, port int, timeout time.Duration) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		status, body, err := processRequest(ctx, storageClient, predictor, r)
		if err != nil {
			if status == http.StatusInternalServerError {
				log.Println("ERROR: Failed to annotate:", err)
			} else {
				log.Println("Could not annotate:", err)
			}
		}
		w.WriteHeader(status)
		if _, err := w.Write([]byte(body)); err != nil {
			log.Println("Failed to write body:", err)
		}
	})

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	log.Println("Listening for connections on port", port)
	return http.ListenAndServe(":"+strconv.Itoa(port), mux)
}

func processRequest(ctx context.Context, storageClient *storage.Client, predictor *predictionClient, r *http.Request) (int, string, error) {
	gcsClient := gcs.NewClient(storageClient)
	const wantMethod = http.MethodPost
	if r.Method != wantMethod {
		return http.StatusMethodNotAllowed, "Wrong method", fmt.Errorf("method must be %s", wantMethod)
	}
	const maxRequestLen = 2000
	if r.ContentLength > maxRequestLen {
		return http.StatusBadRequest, "Request too long", fmt.Errorf("%d byte request", r.ContentLength)
	}
	bytes, err := io.ReadAll(r.Body)
	if err != nil {
		return http.StatusBadRequest, "Could not read request", fmt.Errorf("read request: %w", err)
	}

	if len(bytes) > maxRequestLen { // maybe a streaming request
		return http.StatusBadRequest, "Request too long", fmt.Errorf("%d byte request", len(bytes))
	}

	req, err := parseRequest(bytes)
	if err != nil {
		return http.StatusBadRequest, "Could not parse request", fmt.Errorf("parse %d byte request: %v", len(bytes), err)
	}

	link, b, err := req.Path()
	if err != nil {
		return http.StatusBadRequest, "Could not parse gcs path", fmt.Errorf("parse %s build: %v", string(bytes), err)
	}

	attrs, err := gcsClient.Stat(ctx, *b)
	if err != nil {
		log.Printf("Failed to stat %s: %v", b, err)
		return http.StatusNotFound, "Could not read " + b.String(), fmt.Errorf("stat %s: %w", b, err)
	}
	resp := jsonResult{Link: link}

	if s, e := attrs.Metadata[focusStart], attrs.Metadata[focusEnd]; !req.Overwrite && (s != "" || e != "") {
		log.Println("Already annotated:", req.URL)
		resp.Min, err = strconv.Atoi(s)
		if err != nil {
			return http.StatusConflict, "Bad existing start line", fmt.Errorf("parse %s start (%q): %v", b, s, err)
		}
		resp.Max, err = strconv.Atoi(e)
		if err != nil {
			return http.StatusConflict, "Bad existing end line", fmt.Errorf("parse %s end (%q): %v", b, e, err)
		}
		resp.Pinned = true
	} else {
		log.Println("Annotating:", req.URL)
		lines, _, err := annotateBuild(ctx, gcsClient, predictor, *b)
		if err != nil {
			return http.StatusInternalServerError, "Could not process log", fmt.Errorf("annotate %s: %v", b, err)
		}

		resp.Min, resp.Max = minMax(lines)
		if req.Pin {
			if err := saveLines(ctx, storageClient, *b, resp.Min, resp.Max); err != nil {
				log.Println("Could not save lines:", b, err)
			} else {
				resp.Pinned = true
			}
		}
	}
	if !resp.Pinned {
		u, err := url.Parse(resp.Link)
		if err != nil {
			return http.StatusInternalServerError, "Failed to parse link", fmt.Errorf("parse link %s: %v", resp.Link, err)
		}
		u.Fragment = ""
		resp.Link = fmt.Sprintf("%s#%s%d-%d", u.String(), "1:build-log.txt%3A", resp.Min, resp.Max)
	}

	txt, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return http.StatusInternalServerError, "Failed to format response", fmt.Errorf("marshal %s response: %v", b, err)
	}

	log.Println("Annotated", resp)
	return http.StatusOK, string(txt), nil
}

func parseRequest(buf []byte) (*jsonRequest, error) {
	var r jsonRequest
	if err := json.Unmarshal(buf, &r); err != nil {
		r.URL = string(buf)
		if r.URL == "" {
			return nil, errors.New("empty request")
		}
		log.Println("Raw URL string", r.URL)
	}
	return &r, nil

}

type jsonRequest struct {
	URL       string `json:"url"`
	Pin       bool   `json:"pin"`
	Overwrite bool   `json:"overwrite"`
}

func (r jsonRequest) Path() (string, *gcs.Path, error) {
	var link string
	p, err := pathFromView(r.URL)
	if err != nil {
		p, err = pathFromURL(r.URL)
		if p != nil {
			link = fmt.Sprintf("https://prow.k8s.io/view/gs/%s/%s", p.Bucket(), path.Dir(p.Object()))
		}
	} else {
		link = r.URL
	}
	return link, p, err
}

type jsonResult struct {
	Min    int    `json:"min"`
	Max    int    `json:"max"`
	Link   string `json:"link"`
	Pinned bool   `json:"pinned"`
}

func (jr jsonResult) String() string {
	return fmt.Sprintf("%s (lines %d-%d)", jr.Link, jr.Min, jr.Max)
}

func pathFromURL(raw string) (*gcs.Path, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse url: %v", err)
	}
	switch u.Scheme {
	case "gs":
		return gcs.NewPath(raw)
	case "http", "https":
		u.Host = strings.ReplaceAll(u.Host, "storage.googleapis.com", "")
		u.Host = strings.ReplaceAll(u.Host, "storage.cloud.google.com", "")
		u.Fragment = ""
		u.RawQuery = ""
		u.User = nil
		u.Scheme = "gs"
		if u.Host == "" && len(u.Path) > 0 && u.Path[0] == '/' {
			u.Path = u.Path[1:]
		}
		return gcs.NewPath(u.String())
	}
	return nil, errors.New("scheme must be gs, http or https")
}
