package hal

import (
	"net/http"
)

type HalClient struct {
	http.Client

	base string

	// API Key Auth
	apiKey *string
}

func NewHalClient(base string) *HalClient {
	return &HalClient{
		base: base,
	}
}

func (c *HalClient) SetAPIKey(key string) {
	c.apiKey = &key
}

func (c *HalClient) newGet(path string) (*http.Request, error) {
	req, err := http.NewRequest("GET", c.base+path, nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != nil {
		req.SetBasicAuth("apikey", *c.apiKey)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func (c *HalClient) doRequest(req *http.Request) (Resource, error) {
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return Decode(resp.Body)
}

func (c *HalClient) Get(path string) (Resource, error) {
	req, err := c.newGet(path)
	if err != nil {
		return nil, err
	}

	return c.doRequest(req)
}
