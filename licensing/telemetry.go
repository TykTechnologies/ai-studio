package licensing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func NewClient(url string) *Client {
	telemetryURL := telemetryAPIURL
	if url != "" {
		telemetryURL = url
	}

	return &Client{
		http: &http.Client{Timeout: 10 * time.Second},
		URL:  telemetryURL,
	}
}

func (c *Client) Track(identity, eventName string, properties map[string]interface{}) error {
	event := Event{
		Identity:   identity,
		Event:      eventName,
		Timestamp:  time.Now().Unix(),
		Properties: properties,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("error marshaling event: %w", err)
	}

	resp, err := c.http.Post(c.URL+"/api/track", "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("error sending event to Telemetry: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code from Telemetry: %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}
