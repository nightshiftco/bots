// Package nightshift is a thin JSON-over-HTTP client for the nightshift
// API's grpc-gateway. Request and response bodies are protojson-encoded
// using the generated nsv1 types, which keeps enum names ("INVOKER_TYPE_USER")
// in sync with the wire format.
package nightshift

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	nsv1 "github.com/nightshiftco/nightshift/gen/go/nightshift/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type Client struct {
	baseURL    string
	authBearer string
	http       *http.Client
}

func New(baseURL, bearer string) *Client {
	return &Client{
		baseURL:    baseURL,
		authBearer: bearer,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// StatusError wraps a non-2xx response. Code is the HTTP status; GRPCCode
// is the canonical gRPC code emitted by the gateway when present.
type StatusError struct {
	HTTPCode int
	GRPCCode int // 0 if unknown
	Status   string
	Message  string
}

func (e *StatusError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("nightshift api: http %d %s", e.HTTPCode, e.Status)
	}
	return fmt.Sprintf("nightshift api: http %d (grpc %d): %s", e.HTTPCode, e.GRPCCode, e.Message)
}

// IsAlreadyExists reports whether err is a gateway response with gRPC
// code 6 (ALREADY_EXISTS). Seed steps treat this as success.
func IsAlreadyExists(err error) bool {
	var s *StatusError
	if errors.As(err, &s) {
		return s.GRPCCode == 6 || s.HTTPCode == http.StatusConflict
	}
	return false
}

// gatewayError is the shape returned by grpc-gateway on non-2xx.
type gatewayError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

func (c *Client) do(ctx context.Context, method, path string, in, out proto.Message) error {
	var body io.Reader
	if in != nil {
		b, err := protojson.Marshal(in)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.authBearer)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode/100 != 2 {
		ge := gatewayError{}
		_ = json.Unmarshal(respBody, &ge)
		return &StatusError{
			HTTPCode: resp.StatusCode,
			GRPCCode: ge.Code,
			Status:   ge.Status,
			Message:  ge.Message,
		}
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}
	return nil
}

// ---- Workers ------------------------------------------------------------

func (c *Client) CreateRun(ctx context.Context, req *nsv1.CreateRunRequest) (*nsv1.Run, error) {
	out := &nsv1.CreateRunResponse{}
	if err := c.do(ctx, http.MethodPost, "/v1/runs", req, out); err != nil {
		return nil, err
	}
	return out.GetRun(), nil
}

func (c *Client) GetRun(ctx context.Context, runID string) (*nsv1.Run, error) {
	out := &nsv1.GetRunResponse{}
	if err := c.do(ctx, http.MethodGet, "/v1/runs/"+url.PathEscape(runID), nil, out); err != nil {
		return nil, err
	}
	return out.GetRun(), nil
}

// FinalEvent represents the last StreamEvent of a terminal run, flattened
// for ergonomic access in the Slack handler. Raw is decoded from the
// proto Struct so callers can read fields like "result".
type FinalEvent struct {
	Index int64
	Type  string
	Raw   map[string]any
}

// LastEvent returns the highest-index StreamEvent of a run. The API
// returns events in ascending order regardless of `order_by`, so we
// fetch a large page and take the tail. eventCount lets callers bound
// the page; pass 0 to use a default.
func (c *Client) LastEvent(ctx context.Context, runID string, eventCount int64) (*FinalEvent, error) {
	pageSize := eventCount
	if pageSize <= 0 || pageSize > 500 {
		pageSize = 500
	}
	p := fmt.Sprintf("/v1/runs/%s/events?page_size=%d",
		url.PathEscape(runID), pageSize)
	out := &nsv1.ListRunEventsResponse{}
	if err := c.do(ctx, http.MethodGet, p, nil, out); err != nil {
		return nil, err
	}
	events := out.GetEvents()
	if len(events) == 0 {
		return nil, errors.New("no events found")
	}
	ev := events[len(events)-1]
	var raw map[string]any
	if r := ev.GetRaw(); r != nil {
		raw = r.AsMap()
	}
	return &FinalEvent{Index: ev.GetIndex(), Type: ev.GetType(), Raw: raw}, nil
}

// ---- Config: connectors + skills (seed) --------------------------------

func (c *Client) CreateConnector(ctx context.Context, req *nsv1.CreateConnectorRequest) (*nsv1.Connector, error) {
	out := &nsv1.CreateConnectorResponse{}
	if err := c.do(ctx, http.MethodPost, "/v1/connectors", req, out); err != nil {
		return nil, err
	}
	return out.GetConnector(), nil
}

func (c *Client) SetConnectorStaticToken(ctx context.Context, req *nsv1.SetConnectorStaticTokenRequest) error {
	p := fmt.Sprintf("/v1/connectors/%s:setStaticToken", url.PathEscape(req.GetConnectorName()))
	out := &nsv1.SetConnectorStaticTokenResponse{}
	return c.do(ctx, http.MethodPost, p, req, out)
}

func (c *Client) CreateSkill(ctx context.Context, req *nsv1.CreateSkillRequest) (*nsv1.Skill, error) {
	out := &nsv1.CreateSkillResponse{}
	if err := c.do(ctx, http.MethodPost, "/v1/skills", req, out); err != nil {
		return nil, err
	}
	return out.GetSkill(), nil
}
