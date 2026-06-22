package main

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"k8s.io/utils/env"
)

type NullBool struct {
	Bool  bool
	Valid bool
}

type HealthStatus struct {
	mu     sync.RWMutex
	status NullBool
}

func (h *HealthStatus) Get() NullBool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.status
}

func (h *HealthStatus) Set(valid, value bool) {
	h.mu.Lock()
	h.status.Valid = valid
	h.status.Bool = value
	h.mu.Unlock()
}

var globalHealthStatus = HealthStatus{status: NullBool{Valid: false}}

func parseURL(fieldName, rawURL string) (*url.URL, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("error parsing %s: %w", fieldName, err)
	}

	return parsedURL, nil
}

func validateHealthCheckSuccessCode(statusCode int) error {
	if statusCode < 100 || statusCode > 599 {
		return fmt.Errorf("health check success code must be between 100 and 599, got %d", statusCode)
	}

	return nil
}

func buildRedirectURL(base *url.URL, r *http.Request) string {
	u := *base
	if u.Path == "" {
		u.Path = r.URL.Path
	} else {
		u.Path = strings.TrimRight(u.Path, "/") + r.URL.Path
	}
	u.RawQuery = r.URL.RawQuery

	return u.String()
}

func main() {
	log.Println("[\033[1;32mINFO\033[0m] Starting server...")

	serverPort, err := env.GetInt("SERVER_PORT", 3000)
	if err != nil {
		log.Fatalf("[\033[1;31mERROR\033[0m] Error parsing server port: %s\n", err)
	}
	log.Printf("[\033[1;34mDEBUG\033[0m] Server port: %d\n", serverPort)
	targetUrlString := env.GetString("TARGET_URL", "https://example.com")
	targetUrl, err := parseURL("target URL", targetUrlString)
	if err != nil {
		log.Fatalf("[\033[1;31mERROR\033[0m] %s\n", err)
	}
	log.Println("[\033[1;34mDEBUG\033[0m] Target URL:", targetUrl.String())

	healthCheckURLString := env.GetString("HEALTH_CHECK_URL", "https://example.com/healthz")
	healthCheckURL, err := parseURL("health check URL", healthCheckURLString)
	if err != nil {
		log.Fatalf("[\033[1;31mERROR\033[0m] %s\n", err)
	}
	log.Println("[\033[1;34mDEBUG\033[0m] Health check URL:", healthCheckURL.String())
	healthCheckSuccessCode, err := env.GetInt("HEALTH_CHECK_SUCCESS_CODE", http.StatusOK)
	if err != nil {
		log.Fatalf("[\033[1;31mERROR\033[0m] Error parsing health check success code: %s\n", err)
	}
	if err := validateHealthCheckSuccessCode(healthCheckSuccessCode); err != nil {
		log.Fatalf("[\033[1;31mERROR\033[0m] %s\n", err)
	}
	log.Printf("[\033[1;34mDEBUG\033[0m] Health check success code: %d\n", healthCheckSuccessCode)
	healthCheckBody := env.GetString("HEALTH_CHECK_BODY", "")
	if healthCheckBody != "" {
		log.Println("[\033[1;34mDEBUG\033[0m] Health check body:", healthCheckBody)
	} else {
		log.Println("[\033[1;34mDEBUG\033[0m] Health check body: not specified, ignoring body")
	}

	if targetUrl.Hostname() != healthCheckURL.Hostname() {
		log.Printf("[\033[1;33mWARN\033[0m] Target URL and health check URL are not the same FQDN (%s != %s)\n", targetUrl.Hostname(), healthCheckURL.Hostname())
	}

	healthCheckIntervalSeconds, _ := env.GetInt("HEALTH_CHECK_INTERVAL", 60)
	healthCheckInterval := time.Duration(healthCheckIntervalSeconds) * time.Second
	log.Printf("[\033[1;34mDEBUG\033[0m] Health check interval: %s\n", healthCheckInterval.String())
	healthCheckTimeoutSeconds, _ := env.GetInt("HEALTH_CHECK_TIMEOUT", 10)
	healthCheckTimeout := time.Duration(healthCheckTimeoutSeconds) * time.Second
	log.Printf("[\033[1;34mDEBUG\033[0m] Health check timeout: %s\n", healthCheckTimeout.String())
	healthCheckValidStatuses := []int{healthCheckSuccessCode}
	log.Printf("[\033[1;34mDEBUG\033[0m] Health check valid statuses: %v\n", healthCheckValidStatuses)

	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()
	checkHealth := func() {
		client := http.Client{Timeout: healthCheckTimeout}
		resp, err := client.Get(healthCheckURL.String())
		if err != nil {
			current := globalHealthStatus.Get()
			if !current.Valid || current.Bool {
				log.Printf("[\033[1;31mERROR\033[0m] Health check of %s failed with error: %s\n", healthCheckURL.String(), err)
				globalHealthStatus.Set(true, false)
			}
			return
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				log.Printf("[\033[1;31mERROR\033[0m] Error closing response body: %s\n", err)
			}
		}()

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
			if strings.TrimSpace(string(bodyBytes)) != strings.TrimSpace(healthCheckBody) {
				isValid = false
			}
		}

		current := globalHealthStatus.Get()
		if isValid && (!current.Valid || !current.Bool) {
			globalHealthStatus.Set(true, true)
			log.Printf("[\033[1;32mINFO\033[0m] Health check of %s passed", healthCheckURL.String())
		} else if !isValid && (!current.Valid || current.Bool) {
			globalHealthStatus.Set(true, false)
			log.Printf("[\033[1;31mERROR\033[0m] Health check of %s failed with status code: %d\n", healthCheckURL.String(), resp.StatusCode)
		}
	}
	go func() {
		checkHealth()
		for range ticker.C {
			checkHealth()
		}
	}()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		current := globalHealthStatus.Get()
		if current.Valid && current.Bool {
			redirectURL := buildRedirectURL(targetUrl, r)
			w.Header().Set("Location", redirectURL)
			w.WriteHeader(http.StatusTemporaryRedirect)
			log.Printf("[\033[1;35mACCESS\033[0m] status=%d", http.StatusTemporaryRedirect)
		} else {
			status := http.StatusServiceUnavailable
			if strings.Contains(r.Header.Get("Accept"), "application/json") || (strings.HasPrefix(r.URL.Path, "/api/") && !strings.Contains(r.Header.Get("Accept"), "text/html")) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, err := w.Write([]byte(`{"status": "unavailable", "message": "service is currently undergoing a migration. Please try again later.", "detail": "service is currently undergoing a migration. Please try again later.", "code": 503}`))
				if err != nil {
					log.Printf("[\033[1;31mERROR\033[0m] Error writing response body: %s\n", err)
				}
			} else {
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(status)

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
			log.Printf("[\033[1;35mACCESS\033[0m] status=%d", status)
		}
	})

	log.Printf("[\033[1;32mINFO\033[0m] Starting server on port %d\n", serverPort)
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", serverPort),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		err = server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("[\033[1;31mERROR\033[0m] Error starting server: %s\n", err)
		}
	}()

	sig := <-signalCh
	log.Printf("[\033[1;32mINFO\033[0m] Received signal %s, shutting down...\n", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err = server.Shutdown(ctx)
	if err != nil {
		log.Fatalf("[\033[1;31mERROR\033[0m] Error shutting down server: %s\n", err)
	}
	log.Printf("[\033[1;32mINFO\033[0m] Server shut down successfully\n")
}
