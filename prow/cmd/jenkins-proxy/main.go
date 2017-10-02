package main

import (
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"
)

// TODO: Prometheus metrics
func handle(p Proxy, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Jenkins-Proxy", "JenkinsProxy")

	// Authenticate the request.
	if err := authenticate(r, p.Auth()); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		log.Printf("Unauthorized: %v, (Request: %s %s)", err, r.Method, r.URL.String())
		return
	}

	// There are three different kinds of requests that the jenkins-operator
	// is doing:
	//
	// * build requests (needs a job)
	// * status requests (needs a job)
	// * queue requests (does not need a job)
	//
	// Requests that need a job will need to be resolved using the proxy
	// cache. Queue requests need to be broadcasted to all masters.
	requestedJob := getRequestedJob(r.URL.Path)
	if len(requestedJob) == 0 {
		// Demux requests to all master build queues.
		// TODO: Handle queue cancellation requests. These are tricky because
		// we need to determine to what master to send the request and the
		// request itself does not contain anything related to the build that
		// needs to be cancelled apart from its id in the queue. We will likely
		// need to send the job name inside an http header from prow, and figure
		// out the master here using the job cache.
		if isQueueRequest(r) {
			resp, err := p.ListQueues(r)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				log.Printf("Cannot list from queues: %v, (Request: %s %s)", err, r.Method, r.URL.String())
				return
			}
			forwardResponse(w, resp)
			return
		}

		http.Error(w, "Forbidden.", http.StatusForbidden)
		log.Printf("Forbidden. (Request: %s %s)", r.Method, r.URL.String())
		return
	}

	// Get the destination URL from one of our masters.
	destURL, err := p.GetDestinationURL(r, requestedJob)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		log.Printf("Cannot get destination URL: %v, (Request: %s %s)", err, r.Method, r.URL.String())
		return
	}
	if len(destURL) == 0 {
		http.NotFound(w, r)
		log.Printf("Job %q not found. (Request: %s %s)", requestedJob, r.Method, r.URL.String())
		return
	}

	// Proxy the request to the destination URL.
	resp, err := p.ProxyRequest(r, destURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		log.Printf("Cannot proxy to %s: %v, (Request: %s %s)", destURL, err, r.Method, r.URL.String())
		return
	}
	forwardResponse(w, resp)
}

var configPath = flag.String("config-path", "/etc/jenkins-proxy/config", "Configuration path.")

func main() {
	flag.Parse()

	p, err := NewProxy(*configPath)
	if err != nil {
		log.Fatalf("%v", err)
	}

	log.Printf("Serving on :8080")
	http.HandleFunc("/", p.handler())
	log.Fatal("Jenkins proxy ListenAndServe returned:", http.ListenAndServe(":8080", nil))
}
