package repoowners

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

type OwnersServer struct {
	OwnersClient *Client
}

type ownersRequest struct {
	Org        string   `json:"org_name"`
	Repo       string   `json:"repo_name"`
	BaseCommit string   `json:"base_commit"`
	Paths      []string `json:"file_paths"`
}

type httpJSONError struct {
	Message string `json:"error_message"`
}

type ownersResponse struct {
	ConfigMap map[string]*Config `json:"owners,omitempty"`
}

const (
	contentTypeJSON = "This JSON-API require a JSON content-type"
	emptyBody       = "The body is empty, there is no content to parse"
	emptyPath       = "Path(s) list is empty or non-existing"
	nonPossibleRepo = "The Org or repo or BaseCommit may be not valid(s)"
	corruptedBody   = "The body seems to be corrupted"
	corruptedJSON   = "The JSON seems to be corrupted"
)

func generateMessage(message string) []byte {
	result, err := json.Marshal(httpJSONError{Message: message})
	if err != nil {
		result = []byte(fmt.Sprintf("failed to marshal error: %v", err))
	}
	return result
}

func generateErrorMessage(err error, message string) []byte {
	return generateMessage(fmt.Sprintf("%v : %v", message, err))
}

func (s *OwnersServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Header.Get("Content-Type") != "application/json" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write(generateMessage(contentTypeJSON))
		return
	}

	rawBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(generateErrorMessage(err, corruptedBody))
		return
	} else if len(rawBody) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write(generateMessage(emptyBody))
		return
	}

	reqBody := &ownersRequest{}
	if err = json.Unmarshal(rawBody, reqBody); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write(generateErrorMessage(err, corruptedJSON))
		return
	}
	if len(reqBody.Paths) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write(generateMessage(emptyPath))
		return
	}

	ownerIfc, err := s.OwnersClient.LoadRepoOwners(reqBody.Org, reqBody.Repo, reqBody.BaseCommit)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write(generateErrorMessage(err, nonPossibleRepo))
		return
	}

	requestedOwners := &ownersResponse{ConfigMap: map[string]*Config{}}
	for _, path := range reqBody.Paths {
		var approvers []string
		if len(ownerIfc.Approvers(path)) > 0 {
			approvers = ownerIfc.Approvers(path).List()
		}
		var requiredReviewers []string
		if len(ownerIfc.RequiredReviewers(path)) > 0 {
			requiredReviewers = ownerIfc.RequiredReviewers(path).List()
		}
		var reviewers []string
		if len(ownerIfc.Reviewers(path)) > 0 {
			reviewers = ownerIfc.Reviewers(path).List()
		}
		var labels []string
		if len(ownerIfc.FindLabelsForFile(path)) > 0 {
			labels = ownerIfc.FindLabelsForFile(path).List()
		}

		requestedOwners.ConfigMap[path] = &Config{
			Approvers:         approvers,
			RequiredReviewers: requiredReviewers,
			Reviewers:         reviewers,
			Labels:            labels,
		}
	}

	rawResultBody, err := json.Marshal(requestedOwners.ConfigMap)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(generateMessage(corruptedJSON))
	} else {
		w.WriteHeader(http.StatusOK)
		w.Write(rawResultBody)
	}
}
