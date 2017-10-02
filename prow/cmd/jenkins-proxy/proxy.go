package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type AuthConfig struct {
	Basic       *BasicAuthConfig       `json:"basic_auth,omitempty"`
	BearerToken *BearerTokenAuthConfig `json:"bearer_token,omitempty"`
}

type BasicAuthConfig struct {
	// User is the Jenkins user used for auth.
	User string `json:"user"`
	// TokenFile is the location of the token file.
	TokenFile string `json:"token_file"`
	// token is the token loaded in memory from the location above.
	Token string
}

type BearerTokenAuthConfig struct {
	// TokenFile is the location of the token file.
	TokenFile string `json:"token_file"`
	// token is the token loaded in memory from the location above.
	Token string
}

type JenkinsMaster struct {
	// URLString loads into url in runtime.
	URLString string `json:"url"`
	// url of the Jenkins master to serve traffic to.
	url *url.URL
	// AuthConfig contains the authentication to be used for this master.
	Auth *AuthConfig `json:"auth,omitempty"`
}

type Proxy interface {
	Auth() *BasicAuthConfig
	GetDestinationURL(r *http.Request, requestedJob string) (string, error)
	ProxyRequest(r *http.Request, destURL string) (*http.Response, error)
	ListQueues(r *http.Request) (*http.Response, error)
}

var _ Proxy = &proxy{}

// proxy is able to proxy requests to different Jenkins masters.
type proxy struct {
	sync.RWMutex
	client *http.Client
	// ProxyAuth is used for authenticating with the proxy.
	ProxyAuth *BasicAuthConfig `json:"proxy_auth,omitempty"`
	// Masters includes all the information for contacting different
	// Jenkins masters.
	Masters []JenkinsMaster `json:"masters"`
	// job cache
	cache map[string][]string
}

func NewProxy(path string) (*proxy, error) {
	p := &proxy{
		cache: make(map[string][]string),
	}
	err := p.Load(path)

	go func() {
		for range time.Tick(1 * time.Minute) {
			if err := p.Load(path); err != nil {
				log.Printf("Error loading jenkins proxy config: %v", err)
			} else {
				log.Printf("Jenkins proxy config reloaded.")
			}
		}
	}()

	return p, err
}

func (p *proxy) Load(path string) error {
	log.Printf("Reading config from %s", path)
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("error reading %s: %v", path, err)
	}
	np := &proxy{}
	if err := json.Unmarshal(b, np); err != nil {
		return fmt.Errorf("error unmarshaling %s: %v", path, err)
	}
	if np.ProxyAuth != nil {
		token, err := loadToken(np.ProxyAuth.TokenFile)
		if err != nil {
			return fmt.Errorf("cannot read token file: %v", err)
		}
		np.ProxyAuth.Token = token
	} else {
		log.Print("Authentication to proxy is disabled!")
	}
	if len(np.Masters) == 0 {
		return fmt.Errorf("at least one Jenkins master needs to be setup in %s", path)
	}
	for i, m := range np.Masters {
		u, err := url.Parse(m.URLString)
		if err != nil {
			return fmt.Errorf("cannot parse %s: %v", m.URLString, err)
		}
		np.Masters[i].url = u
		// Setup auth
		if m.Auth != nil {
			if m.Auth.Basic != nil {
				token, err := loadToken(m.Auth.Basic.TokenFile)
				if err != nil {
					return fmt.Errorf("cannot read token file: %v", err)
				}
				np.Masters[i].Auth.Basic.Token = token
			} else if m.Auth.BearerToken != nil {
				token, err := loadToken(m.Auth.BearerToken.TokenFile)
				if err != nil {
					return fmt.Errorf("cannot read token file: %v", err)
				}
				np.Masters[i].Auth.BearerToken.Token = token
			}
		}
	}
	np.client = &http.Client{
		Timeout: 15 * time.Second,
	}

	p.Lock()
	defer p.Unlock()
	np.cache = p.cache
	np.syncCache(true)
	p.ProxyAuth = np.ProxyAuth
	p.Masters = np.Masters
	p.client = np.client
	p.cache = np.cache
	return nil
}

func (p *proxy) syncCache(avoidKnown bool) {
	for _, m := range p.Masters {
		url := m.url.String()
		// If the master is already known, do not relist from it.
		_, isKnown := p.cache[url]
		if avoidKnown && isKnown {
			continue
		}
		log.Printf("Listing jobs from %s", url)
		jobs, err := p.listJenkinsJobs(m.url)
		if err != nil {
			// Do not crash the proxy if a master is unavailable on proxy startup.
			log.Printf("cannot list jobs from %s: %v", url, err)
			continue
		}
		p.cache[url] = jobs
	}
}

func (p *proxy) listJenkinsJobs(url *url.URL) ([]string, error) {
	resp, err := p.request(http.MethodGet, fmt.Sprintf("%s/api/json?tree=jobs[name]", url.String()), nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("response not 2XX: %s", resp.Status)
	}
	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	jobs := struct {
		Jobs []struct {
			Name string `json:"name"`
		} `json:"jobs"`
	}{}
	if err := json.Unmarshal(buf, &jobs); err != nil {
		return nil, err
	}
	var jenkinsJobs []string
	for _, job := range jobs.Jobs {
		jenkinsJobs = append(jenkinsJobs, job.Name)
	}
	return jenkinsJobs, nil
}

const maxRetries = 5

// Retry on transport failures and 500s.
func (p *proxy) request(method, path string, body io.Reader, h *http.Header) (*http.Response, error) {
	var resp *http.Response
	var err error
	backoff := 100 * time.Millisecond
	for retries := 0; retries < maxRetries; retries++ {
		resp, err = p.doRequest(method, path, body, h)
		if err == nil && resp.StatusCode < 500 {
			break
		} else if err == nil && retries+1 < maxRetries {
			resp.Body.Close()
		}

		time.Sleep(backoff)
		backoff *= 2
	}
	return resp, err
}

func (p *proxy) doRequest(method, path string, body io.Reader, h *http.Header) (*http.Response, error) {
	req, err := http.NewRequest(method, path, body)
	if err != nil {
		return nil, err
	}
	if h != nil {
		copyHeader(h, &req.Header)
	}
	// Configure auth
	for _, m := range p.Masters {
		if strings.HasPrefix(path, m.url.String()) {
			if m.Auth.Basic != nil {
				req.SetBasicAuth(m.Auth.Basic.User, m.Auth.Basic.Token)
			} else if m.Auth.BearerToken != nil {
				req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", m.Auth.BearerToken.Token))
			}
			break
		}
	}
	return p.client.Do(req)
}

func (p *proxy) handler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		handle(p, w, r)
	}
}

func (p *proxy) Auth() *BasicAuthConfig {
	p.RLock()
	defer p.RUnlock()
	return p.ProxyAuth
}

func (p *proxy) ProxyRequest(r *http.Request, destURL string) (*http.Response, error) {
	p.RLock()
	defer p.RUnlock()
	return p.request(r.Method, destURL, r.Body, &r.Header)
}

func (p *proxy) GetDestinationURL(r *http.Request, requestedJob string) (string, error) {
	p.Lock()
	defer p.Unlock()
	// Get the master URL by looking at the job cache.
	masterURL := p.getMasterURL(requestedJob)
	if len(masterURL) == 0 {
		// Update the cache by relisting from all masters.
		log.Printf("Cannot find job %q in the cache; relisting from all masters", requestedJob)
		p.syncCache(false)

		masterURL = p.getMasterURL(requestedJob)
		// Return 404 back to the client.
		if len(masterURL) == 0 {
			return "", nil
		}
	}
	// The requested job exists in one of our masters, swap
	// the request hostname for our master hostname and retain
	// the path and any url parameters.
	return replaceHostname(r.URL, masterURL), nil
}

func (p *proxy) getMasterURL(requestedJob string) string {
	for masterURL, jobs := range p.cache {
		for _, job := range jobs {
			// This is our master.
			if job == requestedJob {
				return masterURL
			}
		}
	}
	return ""
}

type JenkinsBuild struct {
	Actions []struct {
		Parameters []struct {
			Name  string      `json:"name"`
			Value interface{} `json:"value"`
		} `json:"parameters"`
	} `json:"actions"`
	Task struct {
		// Used for tracking unscheduled builds for jobs.
		Name string `json:"name"`
	} `json:"task"`
	Number int     `json:"number"`
	Result *string `json:"result"`
}

func (p *proxy) ListQueues(r *http.Request) (*http.Response, error) {
	p.RLock()
	defer p.RUnlock()

	total := struct {
		QueuedBuilds []JenkinsBuild `json:"items"`
	}{}

	for masterURL := range p.cache {
		destURL := replaceHostname(r.URL, masterURL)
		resp, err := p.request(http.MethodGet, destURL, nil, nil)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("response not 2XX??: %s", resp.Status)
		}
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		page := struct {
			QueuedBuilds []JenkinsBuild `json:"items"`
		}{}
		if err := json.Unmarshal(data, &page); err != nil {
			return nil, err
		}
		total.QueuedBuilds = append(total.QueuedBuilds, page.QueuedBuilds...)
	}

	body, err := json.Marshal(total)
	if err != nil {
		return nil, err
	}

	return &http.Response{
		Status:        "200 OK",
		StatusCode:    200,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Body:          ioutil.NopCloser(bytes.NewBuffer(body)),
		ContentLength: int64(len(body)),
		Request:       r,
		Header:        make(http.Header, 0),
	}, nil
}
