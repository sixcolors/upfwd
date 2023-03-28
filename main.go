package main

import (
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"k8s.io/utils/env"
)

/* This is a simple HTTP server that performs a health check on a target URL and
 * redirects to it if the health check passes. If the health check fails, it
 * returns a 503 Service Unavailable response with a custom HTML page.
 *
 * The health check is performed periodically, and the global health status is
 * updated based on the result. The global health status is used to determine
 * whether to redirect to the target URL (StatusTemporaryRedirect)
 * or return a 503 Service Unavailable response.
 *
 * Environment variables:
 * SERVER_PORT - The port to listen on - defaults to 3000
 * TARGET_URL - The URL to redirect to if the health check passes - defaults to https://example.com
 * HEALTH_CHECK_URL - The URL to perform the health check against - defaults to https://example.com/healthz
 * HEALTH_CHECK_INTERVAL - The interval in seconds between health checks, in seconds - defaults to 60
 * HEALTH_CHECK_TIMEOUT - The timeout in seconds for the health check, in seconds - defaults to 10
 * HEALTH_CHECK_SUCCESS_CODE - The HTTP status code that indicates a successful health check - defaults to 200
 * HEALTH_CHECK_BODY - The body of the response that indicates a successful health check - If not specified, the body is ignored
 *
 * The server logs debug information to stdout. The log format is:
 * [DEBUG] <remote address> <method> <path> <status code>
 *
 * The server logs all requests to stdout. The log format is:
 *
 * [ACCESS] <remote address> <method> <path> <status code>
 *
 * The server logs all errors to stdout. The log format is:
 * [ERROR] <error message>
 *
 * The server logs all health check results to stdout. The log format is:
 * [INFO] <health check result>
 *
 */

type NullBool struct {
	Bool  bool
	Valid bool // Valid is true if Bool is not NULL
}

var globalHealthStatus = NullBool{Valid: false}

func main() {
	log.Println("[\033[1;32mINFO\033[0m] Starting server...")

	// Get configuration from ENV or default values
	serverPort, err := env.GetInt("SERVER_PORT", 3000)
	if err != nil {
		log.Fatalf("[\033[1;31mERROR\033[0m] Error parsing server port: %s\n", err)
	}
	log.Printf("[\033[1;34mDEBUG\033[0m] Server port: %d\n", serverPort)
	targetUrlString := env.GetString("TARGET_URL", "https://example.com")
	targetUrl, err := url.Parse(targetUrlString)
	if err != nil {
		log.Fatalf("[\033[1;31mERROR\033[0m] Error parsing target URL: %s\n", err)
	}
	log.Println("[\033[1;34mDEBUG\033[0m] Target URL:", targetUrl.String())

	healthCheckURLString := env.GetString("HEALTH_CHECK_URL", "https://example.com/healthz")
	healthCheckURL, err := url.Parse(healthCheckURLString)
	if err != nil {
		// ERROR in color red, everything else is normal
		log.Printf("[\033[1;31mERROR\033[0m] Error parsing health check URL: %s\n", err)

	}
	log.Println("[\033[1;34mDEBUG\033[0m] Health check URL:", healthCheckURL.String())
	healthCheckBody := env.GetString("HEALTH_CHECK_BODY", "")
	if healthCheckBody != "" {
		log.Println("[\033[1;34mDEBUG\033[0m] Health check body:", healthCheckBody)
	} else {
		log.Println("[\033[1;34mDEBUG\033[0m] Health check body: not specified, ignoring body")
	}

	// Check that the target URL and health check URL are the same FQDN
	if targetUrl.Hostname() != healthCheckURL.Hostname() {
		// WARN in colour if the target URL and health check URL are not the same FQDN
		log.Printf("[\033[1;33mWARN\033[0m] Target URL and health check URL are not the same FQDN (%s != %s)\n", targetUrl.Hostname(), healthCheckURL.Hostname())
	}

	healthCheckIntervalSeconds, _ := env.GetInt("HEALTH_CHECK_INTERVAL", 60)
	healthCheckInterval := time.Duration(healthCheckIntervalSeconds) * time.Second
	log.Printf("[\033[1;34mDEBUG\033[0m] Health check interval: %s\n", healthCheckInterval.String())
	healthCheckTimeoutSeconds, _ := env.GetInt("HEALTH_CHECK_TIMEOUT", 10)
	healthCheckTimeout := time.Duration(healthCheckTimeoutSeconds) * time.Second
	log.Printf("[\033[1;34mDEBUG\033[0m] Health check timeout: %s\n", healthCheckTimeout.String())
	healthCheckValidStatuses := []int{http.StatusOK}
	log.Printf("[\033[1;34mDEBUG\033[0m] Health check valid statuses: %v\n", healthCheckValidStatuses)

	// Set up a ticker to run the health check periodically
	ticker := time.NewTicker(healthCheckInterval)
	checkHealth := func() {
		// Perform the health check, with a timeout
		client := http.Client{
			Timeout: healthCheckTimeout,
		}
		resp, err := client.Get(healthCheckURL.String())
		if err != nil {
			if !globalHealthStatus.Valid || globalHealthStatus.Bool {
				log.Printf("[\033[1;31mERROR\033[0m] Health check of %s failed with error: %s\n", healthCheckURL.String(), err)
				globalHealthStatus.Valid = true
				globalHealthStatus.Bool = false
			}
			return
		}

		// Check if the response status code is valid and the body matches if specified
		isValid := false
		for _, validStatus := range healthCheckValidStatuses {
			if resp.StatusCode == validStatus {
				isValid = true
				break
			}
		}
		if healthCheckBody != "" {
			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Printf("[\033[1;31mERROR\033[0m] Error reading response body: %s\n", err)
				return
			}
			// Check if the response body matches the expected body ignoring whitespace
			if strings.TrimSpace(string(bodyBytes)) != strings.TrimSpace(healthCheckBody) {
				isValid = false
			}
		}

		// Set the global health status based on the health check result
		// if it has changed
		if isValid && (!globalHealthStatus.Valid || !globalHealthStatus.Bool) {
			globalHealthStatus.Valid = true
			globalHealthStatus.Bool = true
			log.Printf("[\033[1;32mINFO\033[0m] Health check of %s passed", healthCheckURL.String())
		} else if !isValid && (!globalHealthStatus.Valid || globalHealthStatus.Bool) {
			globalHealthStatus.Valid = true
			globalHealthStatus.Bool = false
			log.Printf("[\033[1;31mERROR\033[0m] Health check of %s failed with status code: %d\n", healthCheckURL.String(), resp.StatusCode)
		}

		// Close the response body
		if err := resp.Body.Close(); err != nil {
			log.Printf("[\033[1;31mERROR\033[0m] Error closing response body: %s\n", err)
		}
	}
	go func() {
		// Run the first health check immediately
		checkHealth()

		for range ticker.C {
			// Perform the health check
			checkHealth()
		}
	}()

	// Set up the HTTP server
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if globalHealthStatus.Valid && globalHealthStatus.Bool {
			// If the health check passes, redirect to the same path on the target URL
			http.Redirect(w, r, targetUrl.String()+r.URL.Path, http.StatusTemporaryRedirect)
			log.Printf("[\033[1;35mACCESS\033[0m] %s %s %s %d", r.RemoteAddr, r.Method, r.URL.Path, http.StatusTemporaryRedirect)
		} else {
			// If the health check fails, return a 503 Service Unavailable response
			if strings.Contains(r.Header.Get("Accept"), "application/json") || (strings.HasPrefix(r.URL.Path, "/api/") && !strings.Contains(r.Header.Get("Accept"), "text/html")) {
				// If the request is for the API, or the request is for a path that starts with /api/ and does not accept HTML, return a JSON response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, err := w.Write([]byte(`{"status": "unavailable", "message": "service is currently undergoing a migration. Please try again later.", "detail": "service is currently undergoing a migration. Please try again later.", "code": 503}`))
				if err != nil {
					log.Printf("[\033[1;31mERROR\033[0m] Error writing response body: %s\n", err)
				}
			} else {
				// Otherwise, return an HTML response
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusServiceUnavailable)

				tmpl, err := template.New("migration").Parse(`<!doctype html>
<title>Server Migration</title>
<style>
  body { text-align: center; padding: 150px; }
  h1 { font-size: 50px; }
  body { font: 20px Helvetica, sans-serif; color: #333; }
  article { display: block; text-align: left; width: 650px; margin: 0 auto; }
  a { color: #dc8100; text-decoration: none; }
  a:hover { color: #333; text-decoration: none; }
</style>
<article>
    <h1>We&rsquo;ll be back soon!</h1>
    <div>
        <p>Sorry for the inconvenience but we&rsquo;re performing a migration at the moment. We&rsquo;ll be back online shortly!</p>
        <p>&mdash; Server Team</p>
    </div>
</article>`)
				if err != nil {
					log.Printf("[\033[1;31mERROR\033[0m] Error parsing template: %s", err)
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				err = tmpl.Execute(w, nil)
				if err != nil {
					log.Printf("[\033[1;31mERROR\033[0m] Error executing template: %s", err)
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}
			log.Printf("[\033[1;35mACCESS\033[0m] %s %s %s %d", r.RemoteAddr, r.Method, r.URL.Path, http.StatusTemporaryRedirect)
		}
	})

	// Start the HTTP server
	log.Printf("[\033[1;32mINFO\033[0m] Starting server on port %d\n", serverPort)
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", serverPort),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	err = server.ListenAndServe()
	if err != nil {
		log.Fatalf("[\033[1;31mERROR\033[0m] Error starting server: %s\n", err)
	}
}
