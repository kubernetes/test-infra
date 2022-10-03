/*
Copyright 2017 The Kubernetes Authors.

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

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"github.com/sirupsen/logrus"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"k8s.io/test-infra/kubetest/boskos/common"
	"k8s.io/test-infra/kubetest/boskos/storage"
	"k8s.io/test-infra/prow/config/secret"
)

var (
	// ErrNotFound is returned by Acquire() when no resources are available.
	ErrNotFound = errors.New("resources not found")
	// ErrAlreadyInUse is returned by Acquire when resources are already being requested.
	ErrAlreadyInUse = errors.New("resources already used by another user")
	// ErrContextRequired is returned by AcquireWait and AcquireByStateWait when
	// they are invoked with a nil context.
	ErrContextRequired = errors.New("context required")
	// ErrTypeNotFound is returned when the requested resource type (rtype) does not exist.
	// For this error to be returned, you must set DistinguishNotFoundVsTypeNotFound to true.
	ErrTypeNotFound = errors.New("resource type not found")
)

// Client defines the public Boskos client object
type Client struct {
	// Dialer is the net.Dialer used to establish connections to the remote
	// boskos endpoint.
	Dialer DialerWithRetry
	// DistinguishNotFoundVsTypeNotFound, if set, will make it possible to distinguish between
	// ErrNotFound and ErrTypeNotFound. For backwards-compatibility, this flag is off by
	// default.
	DistinguishNotFoundVsTypeNotFound bool

	// http is the http.Client used to interact with the boskos REST API
	http http.Client

	owner       string
	url         string
	username    string
	getPassword func() []byte
	lock        sync.Mutex

	storage storage.PersistenceLayer
}

// NewClient creates a Boskos client for the specified URL and resource owner.
//
// Clients created with this function default to retrying failed connection
// attempts three times with a ten second pause between each attempt.
func NewClient(owner string, urlString, username, passwordFile string) (*Client, error) {

	if (username == "") != (passwordFile == "") {
		return nil, fmt.Errorf("username and passwordFile must be specified together")
	}

	var getPassword func() []byte
	if passwordFile != "" {
		u, err := url.Parse(urlString)
		if err != nil {
			return nil, err
		}
		if u.Scheme != "https" {
			// returning error here would make the tests hard
			// we print out a warning message here instead
			fmt.Printf("[WARNING] should NOT use password without enabling TLS: '%s'\n", urlString)
		}

		if err := secret.Add(passwordFile); err != nil {
			logrus.WithError(err).Fatal("Failed to start secrets agent")
		}
		getPassword = secret.GetTokenGenerator(passwordFile)
	}

	return NewClientWithPasswordGetter(owner, urlString, username, getPassword)
}

// NewClientWithPasswordGetter creates a Boskos client for the specified URL and resource owner.
//
// Clients created with this function default to retrying failed connection
// attempts three times with a ten second pause between each attempt.
func NewClientWithPasswordGetter(owner string, urlString, username string, passwordGetter func() []byte) (*Client, error) {
	client := &Client{
		url:         urlString,
		username:    username,
		getPassword: passwordGetter,
		owner:       owner,
		storage:     storage.NewMemoryStorage(),
	}

	// Configure the dialer to attempt three additional times to establish
	// a connection after a failed dial attempt. The dialer should wait 10
	// seconds between each attempt.
	client.Dialer.RetryCount = 3
	client.Dialer.RetrySleep = time.Second * 10

	// Configure the dialer and HTTP client transport to mimic the configuration
	// of the http.DefaultTransport with the exception that the Dialer's Dial
	// and DialContext functions are assigned to the client transport.
	//
	// See https://golang.org/pkg/net/http/#RoundTripper for the values
	// values used for the http.DefaultTransport.
	client.Dialer.Timeout = 30 * time.Second
	client.Dialer.KeepAlive = 30 * time.Second
	client.Dialer.DualStack = true
	client.http.Transport = &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		Dial:                  client.Dialer.Dial,
		DialContext:           client.Dialer.DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return client, nil
}

// public method

// Acquire asks boskos for a resource of certain type in certain state, and set the resource to dest state.
// Returns the resource on success.
func (c *Client) Acquire(rtype, state, dest string) (*common.Resource, error) {
	return c.AcquireWithPriority(rtype, state, dest, "")
}

// AcquireWithPriority asks boskos for a resource of certain type in certain state, and set the resource to dest state.
// Returns the resource on success.
// Boskos Priority are FIFO.
func (c *Client) AcquireWithPriority(rtype, state, dest, requestID string) (*common.Resource, error) {
	r, err := c.acquire(rtype, state, dest, requestID)
	if err != nil {
		return nil, err
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	if r != nil {
		c.storage.Add(*r)
	}

	return r, nil
}

// AcquireWait blocks until Acquire returns the specified resource or the
// provided context is cancelled or its deadline exceeded.
func (c *Client) AcquireWait(ctx context.Context, rtype, state, dest string) (*common.Resource, error) {
	// request with FIFO priority
	requestID := uuid.New().String()
	return c.AcquireWaitWithPriority(ctx, rtype, state, dest, requestID)
}

// AcquireWaitWithPriority blocks until Acquire returns the specified resource or the
// provided context is cancelled or its deadline exceeded. This allows you to pass in a request priority.
// Boskos Priority are FIFO.
func (c *Client) AcquireWaitWithPriority(ctx context.Context, rtype, state, dest, requestID string) (*common.Resource, error) {
	if ctx == nil {
		return nil, ErrContextRequired
	}
	// Try to acquire the resource until available or the context is
	// cancelled or its deadline exceeded.
	for {
		r, err := c.AcquireWithPriority(rtype, state, dest, requestID)
		if err != nil {
			if err == ErrAlreadyInUse || err == ErrNotFound {
				select {
				case <-ctx.Done():
					return nil, err
				case <-time.After(3 * time.Second):
					continue
				}
			}
			return nil, err
		}
		return r, nil
	}
}

// AcquireByState asks boskos for a resources of certain type, and set the resource to dest state.
// Returns a list of resources on success.
func (c *Client) AcquireByState(state, dest string, names []string) ([]common.Resource, error) {
	resources, err := c.acquireByState(state, dest, names)
	if err != nil {
		return nil, err
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	for _, r := range resources {
		c.storage.Add(r)
	}
	return resources, nil
}

// AcquireByStateWait blocks until AcquireByState returns the specified
// resource(s) or the provided context is cancelled or its deadline
// exceeded.
func (c *Client) AcquireByStateWait(ctx context.Context, state, dest string, names []string) ([]common.Resource, error) {
	if ctx == nil {
		return nil, ErrContextRequired
	}
	// Try to acquire the resource(s) until available or the context is
	// cancelled or its deadline exceeded.
	for {
		r, err := c.AcquireByState(state, dest, names)
		if err != nil {
			if err == ErrAlreadyInUse || err == ErrNotFound {
				select {
				case <-ctx.Done():
					return nil, err
				case <-time.After(3 * time.Second):
					continue
				}
			}
			return nil, err
		}
		return r, nil
	}
}

// ReleaseAll returns all resources hold by the client back to boskos and set them to dest state.
func (c *Client) ReleaseAll(dest string) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	resources, err := c.storage.List()
	if err != nil {
		return err
	}
	if len(resources) == 0 {
		return fmt.Errorf("no holding resource")
	}
	var allErrors error
	for _, r := range resources {
		c.storage.Delete(r.Name)
		err := c.Release(r.Name, dest)
		if err != nil {
			allErrors = multierror.Append(allErrors, err)
		}
	}
	return allErrors
}

// ReleaseOne returns one of owned resources back to boskos and set it to dest state.
func (c *Client) ReleaseOne(name, dest string) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if _, err := c.storage.Get(name); err != nil {
		return fmt.Errorf("no resource name %v", name)
	}
	c.storage.Delete(name)
	if err := c.Release(name, dest); err != nil {
		return err
	}
	return nil
}

// UpdateAll signals update for all resources hold by the client.
func (c *Client) UpdateAll(state string) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	resources, err := c.storage.List()
	if err != nil {
		return err
	}
	if len(resources) == 0 {
		return fmt.Errorf("no holding resource")
	}
	var allErrors error
	for _, r := range resources {
		if err := c.Update(r.Name, state, nil); err != nil {
			allErrors = multierror.Append(allErrors, err)
			continue
		}
		if err := c.updateLocalResource(r, state, nil); err != nil {
			allErrors = multierror.Append(allErrors, err)
		}
	}
	return allErrors
}

// SyncAll signals update for all resources hold by the client.
func (c *Client) SyncAll() error {
	c.lock.Lock()
	defer c.lock.Unlock()

	resources, err := c.storage.List()
	if err != nil {
		return err
	}
	if len(resources) == 0 {
		logrus.Info("no resource to sync")
		return nil
	}
	var allErrors error
	for _, r := range resources {
		if err := c.Update(r.Name, r.State, nil); err != nil {
			allErrors = multierror.Append(allErrors, err)
			continue
		}
		if _, err := c.storage.Update(r); err != nil {
			allErrors = multierror.Append(allErrors, err)
		}
	}
	return allErrors
}

// UpdateOne signals update for one of the resources hold by the client.
func (c *Client) UpdateOne(name, state string, userData *common.UserData) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	r, err := c.storage.Get(name)
	if err != nil {
		return fmt.Errorf("no resource name %v", name)
	}
	if err := c.Update(r.Name, state, userData); err != nil {
		return err
	}
	return c.updateLocalResource(r, state, userData)
}

// Reset will scan all boskos resources of type, in state, last updated before expire, and set them to dest state.
// Returns a map of {resourceName:owner} for further actions.
func (c *Client) Reset(rtype, state string, expire time.Duration, dest string) (map[string]string, error) {
	return c.reset(rtype, state, expire, dest)
}

// Metric will query current metric for target resource type.
// Return a common.Metric object on success.
func (c *Client) Metric(rtype string) (common.Metric, error) {
	return c.metric(rtype)
}

// HasResource tells if current client holds any resources
func (c *Client) HasResource() bool {
	resources, _ := c.storage.List()
	return len(resources) > 0
}

// private methods

func (c *Client) updateLocalResource(res common.Resource, state string, data *common.UserData) error {
	res.State = state
	if res.UserData == nil {
		res.UserData = data
	} else {
		res.UserData.Update(data)
	}
	_, err := c.storage.Update(res)
	return err
}

func (c *Client) acquire(rtype, state, dest, requestID string) (*common.Resource, error) {
	values := url.Values{}
	values.Set("type", rtype)
	values.Set("state", state)
	values.Set("owner", c.owner)
	values.Set("dest", dest)
	if requestID != "" {
		values.Set("request_id", requestID)
	}

	res := common.Resource{}

	work := func(retriedErrs *[]error) (bool, error) {
		resp, err := c.httpPost("/acquire", values, "", nil)
		if err != nil {
			// Swallow the error so we can retry
			*retriedErrs = append(*retriedErrs, err)
			return false, nil
		}
		defer resp.Body.Close()

		switch resp.StatusCode {
		case http.StatusOK:
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return false, err
			}

			err = json.Unmarshal(body, &res)
			if err != nil {
				return false, err
			}
			if res.Name == "" {
				return false, fmt.Errorf("unable to parse resource")
			}
			return true, nil
		case http.StatusUnauthorized:
			return false, ErrAlreadyInUse
		case http.StatusNotFound:
			// The only way to distinguish between all reasources being busy and a request for a non-existent
			// resource type is to check the text of the accompanying error message.
			if c.DistinguishNotFoundVsTypeNotFound {
				if bytes, err := io.ReadAll(resp.Body); err == nil {
					errorMsg := string(bytes)
					if strings.Contains(errorMsg, common.ResourceTypeNotFoundMessage(rtype)) {
						return false, ErrTypeNotFound
					}
				}
			}
			return false, ErrNotFound
		default:
			*retriedErrs = append(*retriedErrs, fmt.Errorf("status %s, status code %v", resp.Status, resp.StatusCode))
			// Swallow it so we can retry
			return false, nil
		}
	}

	return &res, retry(work)
}

func (c *Client) acquireByState(state, dest string, names []string) ([]common.Resource, error) {
	values := url.Values{}
	values.Set("state", state)
	values.Set("dest", dest)
	values.Set("names", strings.Join(names, ","))
	values.Set("owner", c.owner)
	var resources []common.Resource

	work := func(retriedErrs *[]error) (bool, error) {
		resp, err := c.httpPost("/acquirebystate", values, "", nil)
		if err != nil {
			*retriedErrs = append(*retriedErrs, err)
			return false, nil
		}
		defer resp.Body.Close()

		switch resp.StatusCode {
		case http.StatusOK:
			if err := json.NewDecoder(resp.Body).Decode(&resources); err != nil {
				return false, err
			}
			return true, nil
		case http.StatusUnauthorized:
			return false, ErrAlreadyInUse
		case http.StatusNotFound:
			return false, ErrNotFound
		default:
			*retriedErrs = append(*retriedErrs, fmt.Errorf("status %s, status code %v", resp.Status, resp.StatusCode))
			return false, nil
		}
	}

	return resources, retry(work)
}

// Release a lease for a resource and set its state to the destination state
func (c *Client) Release(name, dest string) error {
	values := url.Values{}
	values.Set("name", name)
	values.Set("dest", dest)
	values.Set("owner", c.owner)

	work := func(retriedErrs *[]error) (bool, error) {
		resp, err := c.httpPost("/release", values, "", nil)
		if err != nil {
			*retriedErrs = append(*retriedErrs, err)
			return false, nil
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			*retriedErrs = append(*retriedErrs, fmt.Errorf("status %s, statusCode %v releasing %s", resp.Status, resp.StatusCode, name))
			return false, nil
		}
		return true, nil
	}

	return retry(work)
}

// Update a resource on the server, setting the state and user data
func (c *Client) Update(name, state string, userData *common.UserData) error {
	var bodyData *bytes.Buffer
	if userData != nil {
		bodyData = new(bytes.Buffer)
		err := json.NewEncoder(bodyData).Encode(userData)
		if err != nil {
			return err
		}
	}
	values := url.Values{}
	values.Set("name", name)
	values.Set("owner", c.owner)
	values.Set("state", state)

	work := func(retriedErrs *[]error) (bool, error) {
		// As the body is an io.Reader and hence its content
		// can only be read once, we have to copy it for every request we make
		var body io.Reader
		if bodyData != nil {
			body = bytes.NewReader(bodyData.Bytes())
		}
		resp, err := c.httpPost("/update", values, "application/json", body)
		if err != nil {
			*retriedErrs = append(*retriedErrs, err)
			return false, nil
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			*retriedErrs = append(*retriedErrs, fmt.Errorf("status %s, status code %v updating %s", resp.Status, resp.StatusCode, name))
			return false, nil
		}
		return true, nil
	}

	return retry(work)
}

func (c *Client) reset(rtype, state string, expire time.Duration, dest string) (map[string]string, error) {
	rmap := make(map[string]string)
	values := url.Values{}
	values.Set("type", rtype)
	values.Set("state", state)
	values.Set("expire", expire.String())
	values.Set("dest", dest)

	work := func(retriedErrs *[]error) (bool, error) {
		resp, err := c.httpPost("/reset", values, "", nil)
		if err != nil {
			*retriedErrs = append(*retriedErrs, err)
			return false, nil
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return false, err
			}

			err = json.Unmarshal(body, &rmap)
			return true, err
		}
		*retriedErrs = append(*retriedErrs, fmt.Errorf("status %s, status code %v", resp.Status, resp.StatusCode))
		return false, nil

	}

	return rmap, retry(work)
}

func (c *Client) metric(rtype string) (common.Metric, error) {
	var metric common.Metric
	values := url.Values{}
	values.Set("type", rtype)

	work := func(retriedErrs *[]error) (bool, error) {
		resp, err := c.httpGet("/metric", values)
		if err != nil {
			*retriedErrs = append(*retriedErrs, err)
			return false, nil
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			*retriedErrs = append(*retriedErrs, fmt.Errorf("status %s, status code %v", resp.Status, resp.StatusCode))
			return false, nil
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return false, err
		}

		return true, json.Unmarshal(body, &metric)
	}

	return metric, retry(work)
}

func (c *Client) httpGet(action string, values url.Values) (*http.Response, error) {
	u, _ := url.ParseRequestURI(c.url)
	u.Path = action
	u.RawQuery = values.Encode()
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if c.username != "" && c.getPassword != nil {
		req.SetBasicAuth(c.username, string(c.getPassword()))
	}
	return c.http.Do(req)
}

func (c *Client) httpPost(action string, values url.Values, contentType string, body io.Reader) (*http.Response, error) {
	u, _ := url.ParseRequestURI(c.url)
	u.Path = action
	u.RawQuery = values.Encode()
	req, err := http.NewRequest(http.MethodPost, u.String(), body)
	if err != nil {
		return nil, err
	}
	if c.username != "" && c.getPassword != nil {
		req.SetBasicAuth(c.username, string(c.getPassword()))
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return c.http.Do(req)
}

// DialerWithRetry is a composite version of the net.Dialer that retries
// connection attempts.
type DialerWithRetry struct {
	net.Dialer

	// RetryCount is the number of times to retry a connection attempt.
	RetryCount uint

	// RetrySleep is the length of time to pause between retry attempts.
	RetrySleep time.Duration
}

// Dial connects to the address on the named network.
func (d *DialerWithRetry) Dial(network, address string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, address)
}

// DialContext connects to the address on the named network using the provided context.
func (d *DialerWithRetry) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	// Always bump the retry count by 1 in order to equal the actual number of
	// attempts. For example, if a retry count of 2 is specified, the intent
	// is for three attempts -- the initial attempt with two retries in case
	// the initial attempt times out.
	count := d.RetryCount + 1
	sleep := d.RetrySleep
	i := uint(0)
	for {
		conn, err := d.Dialer.DialContext(ctx, network, address)
		if err != nil {
			if isDialErrorRetriable(err) {
				if i < count-1 {
					select {
					case <-time.After(sleep):
						i++
						continue
					case <-ctx.Done():
						return nil, err
					}
				}
			}
			return nil, err
		}
		return conn, nil
	}
}

// isDialErrorRetriable determines whether or not a dialer should retry
// a failed connection attempt by examining the connection error to see
// if it is one of the following error types:
//   - Timeout
//   - Temporary
//   - ECONNREFUSED
//   - ECONNRESET
func isDialErrorRetriable(err error) bool {
	opErr, isOpErr := err.(*net.OpError)
	if !isOpErr {
		return false
	}
	if opErr.Timeout() || opErr.Temporary() {
		return true
	}
	sysErr, isSysErr := opErr.Err.(*os.SyscallError)
	if !isSysErr {
		return false
	}
	switch sysErr.Err {
	case syscall.ECONNREFUSED, syscall.ECONNRESET:
		return true
	}
	return false
}

// workFunc describes retrieable work. It should
// * Return an error for non-recoverable errors
// * Write retriable errors into `retriedErrs` and return with false, nil
// * Return with true, nil on success
type workFunc func(retriedErrs *[]error) (bool, error)

// SleepFunc is called when requests are retried. This may be replaced in tests.
var SleepFunc = time.Sleep

func retry(work workFunc) error {
	var retriedErrs []error

	maxAttempts := 4
	for i := 1; i <= maxAttempts; i++ {
		success, err := work(&retriedErrs)
		if err != nil {
			return err
		}
		if success {
			return nil
		}
		if i == maxAttempts {
			break
		}

		SleepFunc(time.Duration(i*i) * time.Second)
	}

	return utilerrors.NewAggregate(retriedErrs)
}
