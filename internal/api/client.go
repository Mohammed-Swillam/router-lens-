package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultBaseURL = "https://openrouter.ai/api/v1"
	DefaultTimeout = 15 * time.Second
)

// Client interacts with the OpenRouter API
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
}

// NewClient initializes an API client with optional API key
func NewClient(apiKey string, baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &Client{
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		baseURL: baseURL,
		apiKey:  apiKey,
	}
}

// FetchModelEndpoints retrieves all endpoint data for a specific model ID (e.g. "deepseek/deepseek-chat")
func (c *Client) FetchModelEndpoints(ctx context.Context, modelID string) (*EndpointsResponse, error) {
	cleanModelID := strings.TrimSpace(modelID)
	if cleanModelID == "" {
		return nil, fmt.Errorf("model ID cannot be empty")
	}

	// Model endpoints URL: https://openrouter.ai/api/v1/models/{author}/{slug}/endpoints
	// If the model ID contains slashes, we properly build the path
	endpointURL := fmt.Sprintf("%s/models/%s/endpoints", c.baseURL, cleanModelID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpointURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "RouterLens/1.0")
	req.Header.Set("HTTP-Referer", "https://github.com/routerlens")
	req.Header.Set("X-Title", "RouterLens")
	req.Header.Set("Accept", "application/json")

	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("model '%s' not found on OpenRouter (404)", cleanModelID)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenRouter API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var endpointsResp EndpointsResponse
	if err := json.Unmarshal(bodyBytes, &endpointsResp); err != nil {
		return nil, fmt.Errorf("failed to parse endpoints JSON: %w", err)
	}

	return &endpointsResp, nil
}

// FetchAllModels retrieves the list of available models for validation/search
func (c *Client) FetchAllModels(ctx context.Context) (*ModelsListResponse, error) {
	modelsURL := fmt.Sprintf("%s/models", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "RouterLens/1.0")
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenRouter API error %d: %s", resp.StatusCode, string(body))
	}

	var modelsResp ModelsListResponse
	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		return nil, fmt.Errorf("failed to parse models JSON: %w", err)
	}

	return &modelsResp, nil
}

// EscapeModelID encodes the model path if needed
func EscapeModelID(modelID string) string {
	parts := strings.Split(modelID, "/")
	escapedParts := make([]string, len(parts))
	for i, p := range parts {
		escapedParts[i] = url.PathEscape(p)
	}
	return strings.Join(escapedParts, "/")
}
