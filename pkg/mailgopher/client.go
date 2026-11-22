package mailgopher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Client dispatches plain-text emails through the internal MailGopher service.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient constructs a MailGopher HTTP client.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type sendPayload struct {
	Destination []string `json:"destination"`
	Subject     string   `json:"subject"`
	Content     string   `json:"content"`
}

// Send queues a plain-text email for one or more recipients.
func (c *Client) Send(ctx context.Context, recipients []string, subject, content string) error {
	if c.baseURL == "" {
		return fmt.Errorf("mailgopher URL not configured")
	}
	body, err := json.Marshal(sendPayload{
		Destination: recipients,
		Subject:     subject,
		Content:     content,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/send", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("mailgopher request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("mailgopher returned HTTP %d", resp.StatusCode)
	}
	return nil
}
