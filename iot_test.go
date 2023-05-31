package iot

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleNextPageTokenAsNumber(t *testing.T) {
	t.Setenv("CLEARBLADE_CONFIGURATION", "./test_credentials.json")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v/4/webhook/execute/fakeSystemKey/cloudiot" {
			t.Errorf("Expected to request '/api/v/4/webhook/execute/fakeSystemKey/cloudiot', got: %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"deviceRegistries":[], "nextPageToken": 42}`))
	}))
	defer server.Close()

	ctx := context.Background()
	service, err := NewService(ctx)
	if err != nil {
		t.Errorf("Failed to initialize service: %s", err.Error())
	}
	service.ServiceAccountCredentials = &ServiceAccountCredentials{
		SystemKey: "fakeSystemKey",
		Token:     "fakeToken",
		Url:       server.URL,
		Project:   "testProject",
	}

	parent := fmt.Sprintf("projects/%s/locations/%s", "testProject", "us-central1")
	resp, err := service.Projects.Locations.Registries.List(parent).Do()
	if err != nil {
		t.Errorf("Failed to list registries: %s", err.Error())
	}

	if resp.NextPageToken != "42" {
		t.Errorf("Expected NextPageToken to be '42' but got: %s", resp.NextPageToken)
	}

}

func TestHandleNextPageTokenAsString(t *testing.T) {
	t.Setenv("CLEARBLADE_CONFIGURATION", "./test_credentials.json")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v/4/webhook/execute/fakeSystemKey/cloudiot" {
			t.Errorf("Expected to request '/api/v/4/webhook/execute/fakeSystemKey/cloudiot', got: %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"deviceRegistries":[], "nextPageToken": "42"}`))
	}))
	defer server.Close()

	ctx := context.Background()
	service, err := NewService(ctx)
	if err != nil {
		t.Errorf("Failed to initialize service: %s", err.Error())
	}
	service.ServiceAccountCredentials = &ServiceAccountCredentials{
		SystemKey: "fakeSystemKey",
		Token:     "fakeToken",
		Url:       server.URL,
		Project:   "testProject",
	}

	parent := fmt.Sprintf("projects/%s/locations/%s", "testProject", "us-central1")

	resp, err := service.Projects.Locations.Registries.List(parent).Do()
	if err != nil {
		t.Errorf("Failed to list registries: %s", err.Error())
	}

	if resp.NextPageToken != "42" {
		t.Errorf("Expected NextPageToken to be '42' but got: %s", resp.NextPageToken)
	}

}
