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

func TestMainFlow(t *testing.T) {
	globalHealthStatus.Set(false, false)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	tsClosed := false
	defer func() {
		if !tsClosed {
			ts.Close()
		}
	}()

	t.Setenv("SERVER_PORT", "8080")
	t.Setenv("TARGET_URL", ts.URL)
	t.Setenv("HEALTH_CHECK_URL", ts.URL)
	t.Setenv("HEALTH_CHECK_INTERVAL", "1")
	t.Setenv("HEALTH_CHECK_TIMEOUT", "1")

	c := make(chan struct{})
	go func() {
		main()
		close(c)
	}()

	t.Cleanup(func() {
		p, err := os.FindProcess(os.Getpid())
		if err == nil {
			_ = p.Signal(os.Interrupt)
		}
		select {
		case <-c:
		case <-time.After(5 * time.Second):
			t.Log("server did not shut down within cleanup timeout")
		}
	})

	time.Sleep(1 * time.Second)

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Get("http://localhost:8080")
	if err != nil {
		t.Fatalf("Error sending request to server: %s", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("Error closing response body: %s", err)
		}
	}()

	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Errorf("Expected status code %d, but got %d", http.StatusTemporaryRedirect, resp.StatusCode)
	}
	if location := resp.Header.Get("Location"); location != ts.URL+"/" {
		t.Errorf("Expected Location header to be %s/, but got %s", ts.URL, location)
	}

	ts.Close()
	tsClosed = true

	time.Sleep(2 * time.Second)

	unhealthyResp, err := client.Get("http://localhost:8080")
	if err != nil {
		t.Fatalf("Error sending request to server: %s", err)
	}
	defer func() {
		if err := unhealthyResp.Body.Close(); err != nil {
			t.Logf("Error closing response body: %s", err)
		}
	}()

	if unhealthyResp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status code %d, but got %d", http.StatusServiceUnavailable, unhealthyResp.StatusCode)
	}

	jsonData := []byte(`{
		"name": "Morpheus",
		"position": "Captained"
	}`)
	jsonReq, err := http.NewRequest("POST", "http://localhost:8080/api/neberkenezer/crew/", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Fatalf("Error creating request: %s", err)
	}
	jsonReq.Header.Set("Content-Type", "application/json; charset=UTF-8")
	jsonReq.Header.Set("Accept", "application/json")
	jsonResp, err := client.Do(jsonReq)
	if err != nil {
		t.Fatalf("Error sending request to server: %s", err)
	}
	defer func() {
		if err := jsonResp.Body.Close(); err != nil {
			t.Logf("Error closing response body: %s", err)
		}
	}()

	if jsonResp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status code %d, but got %d", http.StatusServiceUnavailable, jsonResp.StatusCode)
	}

	if jsonResp.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type header to be application/json, but got %s", jsonResp.Header.Get("Content-Type"))
	}

	jsonBodyBytes, err := io.ReadAll(jsonResp.Body)
	if err != nil {
		t.Fatalf("Error reading json response body: %s", err)
	}
	expectedJSONBody := `{"status": "unavailable", "message": "service is currently undergoing a migration. Please try again later.", "detail": "service is currently undergoing a migration. Please try again later.", "code": 503}`
	if strings.TrimSpace(string(jsonBodyBytes)) != expectedJSONBody {
		t.Errorf("Expected json body %s, but got %s", expectedJSONBody, string(jsonBodyBytes))
	}

	htmlReq, err := http.NewRequest("GET", "http://localhost:8080/", nil)
	if err != nil {
		t.Fatalf("Error creating HTML request: %s", err)
	}
	htmlReq.Header.Set("Accept", "text/html")
	respHTML, err := client.Do(htmlReq)
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
}