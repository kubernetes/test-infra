package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	okrov1beta1 "github.com/traiana/okro/okro/api/v1beta1"
)

const (
	validateHeader = "X-VALIDATE-AGAINST"
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

func (c *Client) GetTenant(tenant string) (*okrov1beta1.Tenant, error) {
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
		Tenant *okrov1beta1.Tenant `json:"tenant"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.Tenant, nil
}

func (c *Client) GetCatalog(tenant string) (*okrov1beta1.Catalog, error) {
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
		Catalog *okrov1beta1.Catalog `json:"catalog"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.Catalog, nil
}

func (c *Client) ValidateCatalog(tenant string, catalog *okrov1beta1.Catalog, commit string) error {
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
	req.Header.Set(validateHeader, commit)
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

func (c *Client) PutCatalog(tenant string, catalog *okrov1beta1.Catalog) error {
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

func (c *Client) GetTenantDomain(tenant string, domain string) (*okrov1beta1.TenantDomain, error) {
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
		TenantDomain *okrov1beta1.TenantDomain `json:"tenant-domain"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.TenantDomain, nil
}

func (c *Client) ValidateTenantDomain(tenant string, domain string, tenantDomain *okrov1beta1.TenantDomain, commit string) error {
	url := fmt.Sprintf("%s/tenants/%s/domains/%s:validate", c.baseURL, tenant, domain)
	b, err := json.Marshal(tenantDomain)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	setDefaultHeaders(req)
	req.Header.Set(validateHeader, commit)
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

func (c *Client) PutTenantDomain(tenant string, domain string, tenantDomain *okrov1beta1.TenantDomain) error {
	url := fmt.Sprintf("%s/tenants/%s/domains/%s", c.baseURL, tenant, domain)
	jsonStr, err := json.Marshal(tenantDomain)
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

func (c *Client) GetTenantByRepo(repoOwner string, repoName string) (*okrov1beta1.Tenant, error) {
	url := fmt.Sprintf("%s/tenants:getByRepo", c.baseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	q.Add("owner", repoOwner)
	q.Add("name", repoName)
	req.URL.RawQuery = q.Encode()
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

	var res struct {
		Tenant *okrov1beta1.Tenant `json:"tenant"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.Tenant, nil
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
		Error okrov1beta1.Error `json:"error"`
	}
	if err := json.Unmarshal(b, &res); err == nil {
		return res.Error
	}
	if len(b) == 0 {
		return fmt.Errorf(resp.Status)
	}
	return fmt.Errorf(strings.TrimSpace(string(b)))
}
