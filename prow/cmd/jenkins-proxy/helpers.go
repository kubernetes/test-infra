package main

import (
	"bytes"
	"crypto/subtle"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
)

func loadToken(file string) (string, error) {
	raw, err := ioutil.ReadFile(file)
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(raw)), nil
}

func forwardResponse(w http.ResponseWriter, resp *http.Response) {
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	destHeaders := w.Header()
	copyHeader(&resp.Header, &destHeaders)

	// Forward the status code we got from Jenkins. w.Write by default
	// returns a 200 if w.WriteHeader is not invoked and the prow Jenkins
	// operator expects a 201 in case a build was created.
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}

func copyHeader(source, dest *http.Header) {
	if source == nil {
		return
	}
	for n, v := range *source {
		for _, vv := range v {
			dest.Add(n, vv)
		}
	}
}

// getRequestedJob is attempting to determine if this is a job-specific
// request in a pretty hacky way.
func getRequestedJob(path string) string {
	parts := strings.Split(path, "/")
	jobIndex := -1
	for i, part := range parts {
		if part == "job" {
			// This is a job-specific request. Record the index.
			jobIndex = i + 1
			break
		}
	}
	// If this is not a job-specific request, fail for now. Eventually we
	// are going to proxy queue requests.
	if jobIndex == -1 {
		return ""
	}
	// Sanity check
	if jobIndex+1 > len(parts) {
		return ""
	}
	return parts[jobIndex]
}

func isQueueRequest(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/queue/api/json") && r.Method == http.MethodGet
}

// authenticate the request and return whether auth is missing or failed.
func authenticate(r *http.Request, auth *BasicAuthConfig) error {
	if auth == nil {
		// No auth required to the proxy.
		return nil
	}
	user, pass, ok := r.BasicAuth()
	if !ok {
		// Missing auth headers.
		return errors.New("basic authentication required")
	}
	userCmp := subtle.ConstantTimeCompare([]byte(auth.User), []byte(user))
	passCmp := subtle.ConstantTimeCompare([]byte(auth.Token), []byte(pass))
	if userCmp != 1 || passCmp != 1 {
		// Failed.
		return errors.New("basic authentication failed")
	}
	// Authenticated.
	return nil
}

func replaceHostname(u *url.URL, masterURL string) string {
	destURL := fmt.Sprintf("%s%s", masterURL, u.Path)
	if len(u.RawQuery) > 0 {
		destURL = fmt.Sprintf("%s?%s", destURL, u.RawQuery)
	}
	return destURL
}
