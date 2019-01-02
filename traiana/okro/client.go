package okro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	okrov1beta1 "github.com/traiana/okro/okro/api/v1beta1"
)

type Client struct {
	BaseURL string

	httpc http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		httpc:   http.Client{},
	}
}

func (c *Client) ValidateCatalog(tenant string, catalog *okrov1beta1.Catalog, commit string) error {
	url := fmt.Sprintf("%s/tenants/%s/catalog:validate", c.BaseURL, tenant)
	jsonStr, err := json.Marshal(catalog)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-VALIDATE-AGAINST", commit)
	resp, err := c.httpc.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	var errorResponse errorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errorResponse); err != nil {
		return err
	}
	return errorResponse.Error
}

func (c *Client) ValidateTenantDomain(tenant string, domain string, tenantDomain *okrov1beta1.TenantDomain, commit string) error {
	url := fmt.Sprintf("%s/tenants/%s/domains/%s:validate", c.BaseURL, tenant, domain)
	jsonStr, err := json.Marshal(tenantDomain)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-VALIDATE-AGAINST", commit)
	resp, err := c.httpc.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	var errorResponse errorResponse

	if err := json.NewDecoder(resp.Body).Decode(&errorResponse); err != nil {
		return fmt.Errorf("failed to decode validation error response: %v", err)
	}
	return errorResponse.Error
}

func (c *Client) GetTenantByRepo(repoOwner string, repoName string) (*okrov1beta1.Tenant, error) {
	url := fmt.Sprintf("%s/tenants:getByRepo", c.BaseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	q.Add("owner", repoOwner)
	q.Add("name", repoName)
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpc.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusOK {
		var res struct {
			Tenant *okrov1beta1.Tenant `json:"tenant"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			return nil, err
		}
		return res.Tenant, nil
	}

	var errorResponse errorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errorResponse); err != nil {
		return nil, fmt.Errorf("failed to decode validation error response: %v", err)
	}
	return nil, errorResponse.Error
}

type errorResponse struct {
	Error okrov1beta1.Error `json:"error"`
}
