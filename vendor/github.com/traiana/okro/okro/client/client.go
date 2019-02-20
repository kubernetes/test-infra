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
	commitValidationHeader = okrov1beta2.CommitValidationHeader
	updatedByHeader        = okrov1beta2.UpdatedByHeader
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
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	setHeaders(req, nil)
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

func (c *Client) GetTenantMeta(tenant string) (*okrov1beta2.Meta, error) {
	url := fmt.Sprintf("%s/tenants/%s/meta", c.baseURL, tenant)
	return c.getMeta(url)
}

func (c *Client) GetEnvCd(env string) (*okrov1beta2.EnvCd, error) {
	url := fmt.Sprintf("%s/envs/%s/cd", c.baseURL, env)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	setHeaders(req, nil)
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
		Env *okrov1beta2.EnvCd `json:"env"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.Env, nil
}

func (c *Client) GetEnvMeta(env string) (*okrov1beta2.Meta, error) {
	url := fmt.Sprintf("%s/envs/%s/meta", c.baseURL, env)
	return c.getMeta(url)
}

func (c *Client) GetCatalog(tenant string) (*okrov1beta2.Catalog, error) {
	url := fmt.Sprintf("%s/tenants/%s/catalog", c.baseURL, tenant)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	setHeaders(req, nil)
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

func (c *Client) GetCatalogMeta(tenant string) (*okrov1beta2.Meta, error) {
	url := fmt.Sprintf("%s/tenants/%s/catalog/meta", c.baseURL, tenant)
	return c.getMeta(url)
}

func (c *Client) ValidateCatalog(tenant string, catalog *okrov1beta2.Catalog, commit string) error {
	url := fmt.Sprintf("%s/tenants/%s/catalog:validate", c.baseURL, tenant)
	jsonStr, err := json.Marshal(catalog)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonStr))
	if err != nil {
		return err
	}
	setHeaders(req, nil)
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

func (c *Client) PutCatalog(tenant string, catalog *okrov1beta2.Catalog, updatedBy string) error {
	url := fmt.Sprintf("%s/tenants/%s/catalog", c.baseURL, tenant)
	jsonStr, err := json.Marshal(catalog)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(jsonStr))
	if err != nil {
		return err
	}
	setHeaders(req, map[string]string{
		updatedByHeader: updatedBy,
	})
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
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	setHeaders(req, nil)
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

func (c *Client) GetDomainMeta(tenant string, domain string) (*okrov1beta2.Meta, error) {
	url := fmt.Sprintf("%s/tenants/%s/domains/%s/meta", c.baseURL, tenant, domain)
	return c.getMeta(url)
}

func (c *Client) ValidateDomain(tenant string, domain *okrov1beta2.Domain, commit string) error {
	url := fmt.Sprintf("%s/tenants/%s/domains/%s:validate", c.baseURL, tenant, domain.Name)
	b, err := json.Marshal(domain)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	setHeaders(req, map[string]string{
		commitValidationHeader: commit,
	})
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

func (c *Client) PutDomain(tenant string, domain *okrov1beta2.Domain, updatedBy string) error {
	url := fmt.Sprintf("%s/tenants/%s/domains/%s", c.baseURL, tenant, domain.Name)
	jsonStr, err := json.Marshal(domain)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(jsonStr))
	if err != nil {
		return err
	}
	setHeaders(req, map[string]string{
		updatedByHeader: updatedBy,
	})
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

func (c *Client) GetBuild(tenant string, build string) (*okrov1beta2.Build, error) {
	url := fmt.Sprintf("%s/tenants/%s/builds/%s", c.baseURL, tenant, build)
	jsonStr, err := json.Marshal(build)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, url, bytes.NewBuffer(jsonStr))
	if err != nil {
		return nil, err
	}
	setHeaders(req, nil)
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
		Build *okrov1beta2.Build `json:"build"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.Build, nil
}

func (c *Client) GetBuildMeta(tenant string, build string) (*okrov1beta2.Meta, error) {
	url := fmt.Sprintf("%s/tenants/%s/builds/%s/meta", c.baseURL, tenant, build)
	return c.getMeta(url)
}

func (c *Client) CreateBuild(tenant string, build *okrov1beta2.Build) error {
	url := fmt.Sprintf("%s/tenants/%s/builds", c.baseURL, tenant)
	jsonStr, err := json.Marshal(build)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonStr))
	if err != nil {
		return err
	}
	setHeaders(req, nil)
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

func (c *Client) ValidateBuild(tenant string, build *okrov1beta2.Build) (*okrov1beta2.GenericResponse, error) {
	url := fmt.Sprintf("%s/tenants/%s/builds:validate", c.baseURL, tenant)
	jsonStr, err := json.Marshal(build)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonStr))
	if err != nil {
		return nil, err
	}
	setHeaders(req, nil)
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

	var res *okrov1beta2.GenericResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res, nil
}

func (c *Client) getMeta(url string) (*okrov1beta2.Meta, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	setHeaders(req, nil)
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
		Meta *okrov1beta2.Meta `json:"meta"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.Meta, nil
}

func setHeaders(req *http.Request, extra map[string]string) {
	req.Header.Set("Content-Type", "application/json")
	for k, v := range extra {
		req.Header.Set(k, v)
	}
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
