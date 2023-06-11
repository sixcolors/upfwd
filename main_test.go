package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestMain(t *testing.T) {
	// Create a test HTTP server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	tsClosed := false
	defer func() {
		if !tsClosed {
			ts.Close()
		}
	}()

	// Set environment variables for testing
	envVars := map[string]string{
		"SERVER_PORT":           "8080",
		"TARGET_URL":            ts.URL,
		"HEALTH_CHECK_URL":      ts.URL,
		"HEALTH_CHECK_INTERVAL": "1",
		"HEALTH_CHECK_TIMEOUT":  "1",
	}
	for k, v := range envVars {
		if err := os.Setenv(k, v); err != nil {
			t.Fatalf("Error setting environment variable %s: %s", k, err)
		}
	}
	defer func() {
		for k := range envVars {
			if err := os.Unsetenv(k); err != nil {
				t.Fatalf("Error unsetting environment variable %s: %s", k, err)
			}
		}
	}()

	// Start the server in a goroutine
	c := make(chan struct{})
	go func() {
		main()
		c <- struct{}{}
	}()

	// Wait for the server to start
	time.Sleep(1 * time.Second)

	// Create a http client
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Send a request to the server
	resp, err := client.Get("http://localhost:8080")
	if err != nil {
		t.Fatalf("Error sending request to server: %s", err)
	}
	defer resp.Body.Close()

	// Check the response status code
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Errorf("Expected status code %d, but got %d", http.StatusTemporaryRedirect, resp.StatusCode)
	}

	// check with health check url down

	// stop ts server
	ts.Close()
	tsClosed = true

	// wait for health check to fail
	time.Sleep(2 * time.Second)

	// Send a request to the server
	resp, err = client.Get("http://localhost:8080")
	if err != nil {
		t.Fatalf("Error sending request to server: %s", err)
	}
	defer resp.Body.Close()

	// Check the response status code
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status code %d, but got %d", http.StatusServiceUnavailable, resp.StatusCode)
	}

	// test request with application/json content type
	var jsonData = []byte(`{
		"name": "Morpheus",
		"position": "Captained"
	}`)
	req, error := http.NewRequest("POST", "http://localhost:8080/api/neberkenezer/crew/", bytes.NewBuffer(jsonData))
	if error != nil {
		t.Fatalf("Error creating request: %s", error)
	}
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("Accept", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Error sending request to server: %s", err)
	}
	defer resp.Body.Close()

	// Check the response status code
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status code %d, but got %d", http.StatusServiceUnavailable, resp.StatusCode)
	}

	// check the header Content-Type is application/json
	if resp.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type header to be application/json, but got %s", resp.Header.Get("Content-Type"))
	}

	// check the body is a json object
	if resp.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type header to be application/json, but got %s", resp.Header.Get("Content-Type"))
	}

	// send interrupt signal to the server
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("Error finding process: %s", err)
	}
	if err := p.Signal(os.Interrupt); err != nil {
		t.Fatalf("Error sending interrupt signal to server: %s", err)
	}

	// Wait for the server to stop
	<-c
}
