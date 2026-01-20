package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// ============================================================
// API CLIENT - Reusable HTTP client for face recognition API
// ============================================================

type APIClient struct {
	Timeout time.Duration
	client  *http.Client
}

// isSuccessStatusCode checks if the HTTP status code indicates success
func (c *APIClient) IsSuccessStatusCode(statusCode int) bool {
	return statusCode >= 200 && statusCode < 300
}

// NewAPIClient creates a new API client instance
func NewAPIClient(timeout time.Duration) *APIClient {
	return &APIClient{
		Timeout: timeout,
		client:  &http.Client{Timeout: timeout},
	}
}

// SendRequest sends a POST request to the API with proper headers
func (c *APIClient) SendRequest(payload interface{}, endpoint string) ([]byte, int, error) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Log request payload
	log.Printf("Request Payload: %s", string(jsonData))

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	c.setHeaders(req)

	// Log request headers
	log.Println("Request Headers:")
	for key, values := range req.Header {
		for _, value := range values {
			log.Printf("  %s: %s", key, value)
		}
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Log response status
	log.Printf("Response Status: %d %s", resp.StatusCode, resp.Status)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response: %w", err)
	}

	// Log response body
	log.Printf("Response Body: %s", string(body))

	return body, resp.StatusCode, nil
}

// setHeaders sets required headers for the API request
func (c *APIClient) setHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "vi,en-US;q=0.9,en;q=0.8")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Secret-Key", os.Getenv("SECRET_KEY"))
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
}

// ParseResponse unmarshals JSON response into provided struct
func (c *APIClient) ParseResponse(body []byte, result interface{}) error {
	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}
	return nil
}

// LogResponse logs the raw response if it's small enough
func (c *APIClient) LogResponse(body []byte, statusCode int) {
	if statusCode >= 200 && statusCode < 300 {
		log.Printf("✅ API response: %d - Success!", statusCode)
	} else {
		log.Printf("⚠️  API response: %d - Failed", statusCode)
	}

	if len(body) > 0 && len(body) < 1000 {
		log.Printf("📥 Raw response: %s", string(body))
	}
}
