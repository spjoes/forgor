package sync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type LANClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewLANClient(address string) *LANClient {
	return &LANClient{
		baseURL: address,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *LANClient) do(method, path string, body interface{}, result interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		var apiErr APIError
		if err := json.Unmarshal(respBody, &apiErr); err != nil {
			return &APIError{
				StatusCode: resp.StatusCode,
				Code:       "unknown_error",
				Message:    string(respBody),
			}
		}
		apiErr.StatusCode = resp.StatusCode
		return &apiErr
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}

	return nil
}

func (c *LANClient) GetWhoami() (*WhoamiResponse, error) {
	var result WhoamiResponse
	err := c.do(http.MethodGet, "/whoami", nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *LANClient) PullEvents(vaultID UUID, sinceSeq uint64) ([]Event, error) {
	path := fmt.Sprintf("/v1/vaults/%s/events?since_seq=%d", vaultID.String(), sinceSeq)
	var result []Event
	err := c.do(http.MethodGet, path, nil, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (c *LANClient) PushEvent(vaultID UUID, event Event) (*EventResponse, error) {
	path := fmt.Sprintf("/v1/vaults/%s/events", vaultID.String())
	var result EventResponse
	err := c.do(http.MethodPost, path, event, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *LANClient) GetMemberEvents(vaultID UUID, sinceSeq uint64) ([]MemberEvent, error) {
	path := fmt.Sprintf("/v1/vaults/%s/member_events?since_seq=%d", vaultID.String(), sinceSeq)
	var result []MemberEvent
	err := c.do(http.MethodGet, path, nil, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (c *LANClient) GetVaultMembers(vaultID UUID) (*VaultMembershipResponse, error) {
	path := fmt.Sprintf("/v1/vaults/%s/members", vaultID.String())
	var result VaultMembershipResponse
	err := c.do(http.MethodGet, path, nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *LANClient) BaseURL() string {
	return c.baseURL
}
