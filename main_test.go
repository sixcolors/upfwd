package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("Error closing response body: %s", err)
		}
	}()

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
	unhealthyResp, err := client.Get("http://localhost:8080")
	if err != nil {
		t.Fatalf("Error sending request to server: %s", err)
	}
	defer func() {
		if err := unhealthyResp.Body.Close(); err != nil {
			t.Logf("Error closing response body: %s", err)
		}
	}()

	// Check the response status code
	if unhealthyResp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status code %d, but got %d", http.StatusServiceUnavailable, unhealthyResp.StatusCode)
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
	jsonResp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Error sending request to server: %s", err)
	}
	defer func() {
		if err := jsonResp.Body.Close(); err != nil {
			t.Logf("Error closing response body: %s", err)
		}
	}()

	// Check the response status code
	if jsonResp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status code %d, but got %d", http.StatusServiceUnavailable, jsonResp.StatusCode)
	}

	// check the header Content-Type is application/json
	if jsonResp.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type header to be application/json, but got %s", jsonResp.Header.Get("Content-Type"))
	}

	// check the body is a json object
	jsonBodyBytes, err := io.ReadAll(jsonResp.Body)
	if err != nil {
		t.Fatalf("Error reading json response body: %s", err)
	}
	expectedJsonBody := `{"status": "unavailable", "message": "service is currently undergoing a migration. Please try again later.", "detail": "service is currently undergoing a migration. Please try again later.", "code": 503}`
	if strings.TrimSpace(string(jsonBodyBytes)) != expectedJsonBody {
		t.Errorf("Expected json body %s, but got %s", expectedJsonBody, string(jsonBodyBytes))
	}

	// Test request with Accept: text/html when health check fails
	reqHTML, err := http.NewRequest("GET", "http://localhost:8080/", nil)
	if err != nil {
		t.Fatalf("Error creating HTML request: %s", err)
	}
	reqHTML.Header.Set("Accept", "text/html")
	respHTML, err := client.Do(reqHTML)
	if err != nil {
		t.Fatalf("Error sending HTML request to server: %s", err)
	}
	defer func() {
		if err := respHTML.Body.Close(); err != nil {
			t.Logf("Error closing HTML response body: %s", err)
		}
	}()

	if respHTML.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status code %d for HTML request, but got %d", http.StatusServiceUnavailable, respHTML.StatusCode)
	}
	if contentType := respHTML.Header.Get("Content-Type"); contentType != "text/html" {
		t.Errorf("Expected Content-Type header to be text/html, but got %s", contentType)
	}
	htmlBodyBytes, err := io.ReadAll(respHTML.Body)
	if err != nil {
		t.Fatalf("Error reading HTML response body: %s", err)
	}
	if !strings.Contains(string(htmlBodyBytes), "<title>Server Migration</title>") {
		t.Errorf("Expected HTML body to contain '<title>Server Migration</title>', but got: %s", string(htmlBodyBytes))
	}
	if !strings.Contains(string(htmlBodyBytes), "<h1>We&rsquo;ll be back soon!</h1>") {
		t.Errorf("Expected HTML body to contain '<h1>We&rsquo;ll be back soon!</h1>', but got: %s", string(htmlBodyBytes))
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
