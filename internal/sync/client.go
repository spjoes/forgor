package sync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type APIError struct {
	StatusCode int    `json:"-"`
	Code       string `json:"code"`
	Message    string `json:"message"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (c *Client) do(method, path string, body interface{}, result interface{}) error {
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

func (c *Client) RegisterDevice(bundle DeviceBundle) error {
	return c.do(http.MethodPost, "/v1/devices/register", bundle, nil)
}

func (c *Client) GetDevice(deviceID string) (*DeviceBundle, error) {
	var result DeviceBundle
	err := c.do(http.MethodGet, "/v1/devices/"+url.PathEscape(deviceID), nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) CreateInvite(vaultID UUID, invite Invite) error {
	path := fmt.Sprintf("/v1/vaults/%s/invites", vaultID.String())
	return c.do(http.MethodPost, path, invite, nil)
}

func (c *Client) GetInvites(deviceID string) ([]Invite, error) {
	path := "/v1/invites?device_id=" + url.QueryEscape(deviceID)
	var result []Invite
	err := c.do(http.MethodGet, path, nil, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) ClaimInvite(inviteID UUID, claim InviteClaim) error {
	path := fmt.Sprintf("/v1/invites/%s/claim", inviteID.String())
	return c.do(http.MethodPost, path, claim, nil)
}

func (c *Client) GetInviteClaims(createdByDeviceID string) ([]InviteClaim, error) {
	path := "/v1/invite_claims?created_by_device_id=" + url.QueryEscape(createdByDeviceID)
	var result []InviteClaim
	err := c.do(http.MethodGet, path, nil, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) CreateMemberEvent(vaultID UUID, event MemberEvent) error {
	path := fmt.Sprintf("/v1/vaults/%s/member_events", vaultID.String())
	return c.do(http.MethodPost, path, event, nil)
}

func (c *Client) GetMemberEvents(vaultID UUID, sinceSeq uint64) ([]MemberEvent, error) {
	path := fmt.Sprintf("/v1/vaults/%s/member_events?since_seq=%d", vaultID.String(), sinceSeq)
	var result []MemberEvent
	err := c.do(http.MethodGet, path, nil, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) GetVaultMembers(vaultID UUID) (*VaultMembershipResponse, error) {
	path := fmt.Sprintf("/v1/vaults/%s/members", vaultID.String())
	var result VaultMembershipResponse
	err := c.do(http.MethodGet, path, nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) PushEvent(vaultID UUID, event Event) (*EventResponse, error) {
	path := fmt.Sprintf("/v1/vaults/%s/events", vaultID.String())
	var result EventResponse
	err := c.do(http.MethodPost, path, event, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) PullEvents(vaultID UUID, sinceSeq uint64) ([]Event, error) {
	path := fmt.Sprintf("/v1/vaults/%s/events?since_seq=%d", vaultID.String(), sinceSeq)
	var result []Event
	err := c.do(http.MethodGet, path, nil, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) CreateKeyUpdate(vaultID UUID, ku KeyUpdate) error {
	path := fmt.Sprintf("/v1/vaults/%s/key_updates", vaultID.String())
	return c.do(http.MethodPost, path, ku, nil)
}

func (c *Client) GetKeyUpdates(deviceID string) ([]KeyUpdate, error) {
	path := "/v1/key_updates?target_device_id=" + url.QueryEscape(deviceID)
	var result []KeyUpdate
	err := c.do(http.MethodGet, path, nil, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) AckKeyUpdate(vaultID UUID, ack KeyUpdateAck) error {
	path := fmt.Sprintf("/v1/vaults/%s/key_update_acks", vaultID.String())
	return c.do(http.MethodPost, path, ack, nil)
}

func (c *Client) CreateSnapshot(vaultID UUID, snapshot Snapshot) error {
	path := fmt.Sprintf("/v1/vaults/%s/snapshots", vaultID.String())
	return c.do(http.MethodPost, path, snapshot, nil)
}

func (c *Client) GetLatestSnapshot(vaultID UUID) (*Snapshot, error) {
	path := fmt.Sprintf("/v1/vaults/%s/snapshots/latest", vaultID.String())
	var result Snapshot
	err := c.do(http.MethodGet, path, nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}
