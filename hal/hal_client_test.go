package hal

import (
	"log"
	"testing"
)

const (
	// TODO: Replace with FakeTestHTTPServer
	openProjectBase = "http://192.168.2.50:8080"
	// This API key is from a test user on a private test OpenProject instance.
	testAPIKey = "6d18cd23b6bd5e4c1531482b2a5eda171ce75cbc10c12d9c3370e898ef01ac69"
)

func TestHalClient_Get(t *testing.T) {
	c := NewHalClient(openProjectBase)

	res, err := c.Get("/api/v3/configuration")
	if err != nil {
		t.Errorf("HalClient failed to Get Hal resource: %v.", err)
	}
	log.Printf("res = %+v", res)
}

func TestHalClient_ApiKey(t *testing.T) {
	c := NewHalClient(openProjectBase)
	c.SetAPIKey(testAPIKey)

	res, err := c.Get("/api/v3/my_preferences")
	if err != nil {
		t.Errorf("HalClient failed to Get Hal resource: %v.", err)
	}
	resErr := res.IsError()
	if resErr != nil {
		t.Errorf("HalClient login failed: %v.", resErr.Message)
	}
	log.Printf("res = %+v", res)
}

func TestHalClient_Unauthorized(t *testing.T) {
	c := NewHalClient(openProjectBase)

	res, err := c.Get("/api/v3/my_preferences")
	if err != nil {
		t.Errorf("HalClient failed to Get Hal resource: %v.", err)
	}
	resErr := res.IsError()
	if resErr == nil {
		t.Errorf("Expected unauthorized response: %v.", res)
	}
}
