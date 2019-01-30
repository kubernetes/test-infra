package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	okrov1beta2 "github.com/traiana/okro/okro/api/v1beta2"
)

const (
	commitValidationHeader = "X-Validate-Against"
)

type Client struct {
	baseURL string
	httpc   http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		httpc:   http.Client{},
	}
}

func (c *Client) GetTenant(tenant string) (*okrov1beta2.Tenant, error) {
	url := fmt.Sprintf("%s/tenants/%s", c.baseURL, tenant)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	setDefaultHeaders(req)
	resp, err := c.httpc.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, parseErrorResponse(resp)
	}

	var res *struct {
		Tenant *okrov1beta2.Tenant `json:"tenant"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.Tenant, nil
}

func (c *Client) GetCatalog(tenant string) (*okrov1beta2.Catalog, error) {
	url := fmt.Sprintf("%s/tenants/%s/catalog", c.baseURL, tenant)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	setDefaultHeaders(req)
	resp, err := c.httpc.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, parseErrorResponse(resp)
	}

	var res *struct {
		Catalog *okrov1beta2.Catalog `json:"catalog"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.Catalog, nil
}

func (c *Client) ValidateCatalog(tenant string, catalog *okrov1beta2.Catalog, commit string) error {
	url := fmt.Sprintf("%s/tenants/%s/catalog:validate", c.baseURL, tenant)
	jsonStr, err := json.Marshal(catalog)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	if err != nil {
		return err
	}
	setDefaultHeaders(req)
	req.Header.Set(commitValidationHeader, commit)
	resp, err := c.httpc.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusNoContent {
		return parseErrorResponse(resp)
	}
	return nil
}

func (c *Client) PutCatalog(tenant string, catalog *okrov1beta2.Catalog) error {
	url := fmt.Sprintf("%s/tenants/%s/catalog", c.baseURL, tenant)
	jsonStr, err := json.Marshal(catalog)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(jsonStr))
	if err != nil {
		return err
	}
	setDefaultHeaders(req)
	resp, err := c.httpc.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusNoContent {
		return parseErrorResponse(resp)
	}
	return nil
}

func (c *Client) GetDomain(tenant string, domain string) (*okrov1beta2.Domain, error) {
	url := fmt.Sprintf("%s/tenants/%s/domains/%s", c.baseURL, tenant, domain)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	setDefaultHeaders(req)
	resp, err := c.httpc.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, parseErrorResponse(resp)
	}

	var res *struct {
		Domain *okrov1beta2.Domain `json:"domain"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.Domain, nil
}

func (c *Client) ValidateDomain(tenant string, domain *okrov1beta2.Domain, commit string) error {
	url := fmt.Sprintf("%s/tenants/%s/domains/%s:validate", c.baseURL, tenant, domain.Name)
	b, err := json.Marshal(domain)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	setDefaultHeaders(req)
	req.Header.Set(commitValidationHeader, commit)
	resp, err := c.httpc.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusNoContent {
		return parseErrorResponse(resp)
	}
	return nil
}

func (c *Client) PutDomain(tenant string, domain *okrov1beta2.Domain) error {
	url := fmt.Sprintf("%s/tenants/%s/domains/%s", c.baseURL, tenant, domain.Name)
	jsonStr, err := json.Marshal(domain)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(jsonStr))
	if err != nil {
		return err
	}
	setDefaultHeaders(req)
	resp, err := c.httpc.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusNoContent {
		return parseErrorResponse(resp)
	}
	return nil
}

func setDefaultHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
}

func parseErrorResponse(resp *http.Response) error {
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %v", err)
	}
	var res struct {
		Error okrov1beta2.Error `json:"error"`
	}
	if err := json.Unmarshal(b, &res); err == nil {
		return res.Error
	}
	if len(b) == 0 {
		return fmt.Errorf(resp.Status)
	}
	return fmt.Errorf(strings.TrimSpace(string(b)))
}
