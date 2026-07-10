package providers

import (
	"context"
	"io"
	"net/http"
	"sync"
)

// Client is the one concrete ChatProvider: a Profile bound to credentials,
// a base URL, a model, and an injected http.Client. It has no hardcoded URLs
// and is fully testable against httptest servers.
type Client struct {
	profile    *Profile
	transport  Transport
	httpClient *http.Client
	apiKey     string
	baseURL    string

	mu    sync.RWMutex
	model string
}

// NewClient binds a profile to runtime state. Empty baseURL/model fall back
// to the profile's BaseURL/DefaultModel; a nil http.Client gets a default.
func NewClient(profile *Profile, apiKey, baseURL, model string, hc *http.Client) *Client {
	if profile == nil {
		profile = &Profile{Name: "custom", APIMode: APIModeChatCompletions}
	}
	if baseURL == "" {
		baseURL = profile.BaseURL
	}
	if model == "" {
		model = profile.DefaultModel
	}
	if hc == nil {
		hc = &http.Client{}
	}
	return &Client{
		profile:    profile,
		transport:  GetTransport(profile.APIMode),
		httpClient: hc,
		apiKey:     apiKey,
		baseURL:    baseURL,
		model:      model,
	}
}

// Name returns the profile name of the provider this client talks to.
func (c *Client) Name() string { return c.profile.Name }

// Model returns the model currently in use.
func (c *Client) Model() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.model
}

// SetModel swaps the model in place (used when advancing a fallback chain).
func (c *Client) SetModel(m string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.model = m
}

// Complete performs a non-streaming completion.
func (c *Client) Complete(ctx context.Context, system string, msgs []Message, tools []ToolDef) (*NormalizedResponse, error) {
	body, err := c.do(ctx, system, msgs, tools, false)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return c.transport.ParseResponse(body)
}

// Stream performs a streaming completion, invoking onDelta for each text
// fragment and returning the same NormalizedResponse shape as Complete.
func (c *Client) Stream(ctx context.Context, system string, msgs []Message, tools []ToolDef, onDelta func(string)) (*NormalizedResponse, error) {
	body, err := c.do(ctx, system, msgs, tools, true)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return c.transport.ParseStream(body, onDelta)
}

// do builds and executes one request, classifying transport and HTTP errors.
// On success the caller owns the returned body.
func (c *Client) do(ctx context.Context, system string, msgs []Message, tools []ToolDef, stream bool) (io.ReadCloser, error) {
	model := c.Model()
	model = NormalizeModelForProvider(model, c.profile.Name)

	req, err := c.transport.BuildRequest(ctx, c.profile, c.apiKey, c.baseURL, model, system, msgs, tools, stream)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, Classify(err, 0, nil, c.profile.Name, model)
	}
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		resp.Body.Close()
		return nil, Classify(nil, resp.StatusCode, errBody, c.profile.Name, model)
	}
	return resp.Body, nil
}
