package webhook

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/bytedance/sonic"
)

// JobCompletionPayload is the payload sent to webhook URLs
type JobCompletionPayload struct {
	JobID     string `json:"job_id"`
	Status    string `json:"status"`
	S3URL     string `json:"s3_url,omitempty"`
	Error     string `json:"error,omitempty"`
	Timestamp string `json:"timestamp"`
}

// Client handles webhook notifications
type Client struct {
	httpClient *http.Client
}

// NewClient creates a new webhook client
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SendJobComplete sends a job completion notification to a webhook URL
func (c *Client) SendJobComplete(ctx context.Context, webhookURL string, headers map[string]string, payload JobCompletionPayload) error {
	if webhookURL == "" {
		return nil // No webhook URL provided, nothing to do
	}

	// Add timestamp
	payload.Timestamp = time.Now().UTC().Format(time.RFC3339)

	// Marshal payload to JSON
	jsonData, err := sonic.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "GoVid/1.0")

	// Set custom headers if provided
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// SendJobCompleteAsync sends a job completion notification asynchronously
func (c *Client) SendJobCompleteAsync(webhookURL string, headers map[string]string, payload JobCompletionPayload) {
	if webhookURL == "" {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		err := c.SendJobComplete(ctx, webhookURL, headers, payload)
		if err != nil {
			log.Printf("Failed to send webhook to %s: %v", webhookURL, err)
		} else {
			log.Printf("Successfully sent webhook to %s for job %s", webhookURL, payload.JobID)
		}
	}()
}
