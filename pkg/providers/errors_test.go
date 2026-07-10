package providers

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
)

// fakeNetTimeout implements net.Error with Timeout() == true.
type fakeNetTimeout struct{}

func (fakeNetTimeout) Error() string   { return "i/o timeout" }
func (fakeNetTimeout) Timeout() bool   { return true }
func (fakeNetTimeout) Temporary() bool { return true }

func TestClassify(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		status        int
		body          string
		wantReason    Reason
		wantRetryable bool
		wantFallback  bool
	}{
		{
			name: "context deadline exceeded is timeout",
			err:  context.DeadlineExceeded, wantReason: ReasonTimeout, wantRetryable: true, wantFallback: true,
		},
		{
			name: "net timeout is timeout",
			err:  fakeNetTimeout{}, wantReason: ReasonTimeout, wantRetryable: true, wantFallback: true,
		},
		{
			name:       "connection refused is network",
			err:        &net.OpError{Op: "dial", Err: errors.New("connection refused")},
			wantReason: ReasonNetwork, wantRetryable: true, wantFallback: true,
		},
		{
			name:   "401 is auth",
			status: 401, body: `{"error":{"message":"invalid x-api-key","type":"authentication_error"}}`,
			wantReason: ReasonAuth, wantRetryable: false, wantFallback: true,
		},
		{
			name:   "403 is auth",
			status: 403, body: `{"error":{"message":"permission denied"}}`,
			wantReason: ReasonAuth, wantRetryable: false, wantFallback: true,
		},
		{
			name:   "402 is billing",
			status: 402, body: `{"error":{"message":"payment required"}}`,
			wantReason: ReasonBilling, wantRetryable: false, wantFallback: true,
		},
		{
			name:   "429 is rate_limit",
			status: 429, body: `{"error":{"message":"Rate limit reached for gpt-4o","type":"rate_limit_error"}}`,
			wantReason: ReasonRateLimit, wantRetryable: true, wantFallback: true,
		},
		{
			name:   "429 with insufficient_quota is billing",
			status: 429, body: `{"error":{"message":"You exceeded your current quota, please check your plan and billing details.","type":"insufficient_quota","code":"insufficient_quota"}}`,
			wantReason: ReasonBilling, wantRetryable: false, wantFallback: true,
		},
		{
			name:   "408 is timeout",
			status: 408, body: `{"error":{"message":"request timed out"}}`,
			wantReason: ReasonTimeout, wantRetryable: true, wantFallback: true,
		},
		{
			name:   "413 is context_overflow",
			status: 413, body: `{"error":{"message":"payload too large"}}`,
			wantReason: ReasonContextOverflow, wantRetryable: false, wantFallback: false,
		},
		{
			name:   "404 with model pattern is model_not_found",
			status: 404, body: `{"error":{"message":"The model 'gpt-99' does not exist","code":"model_not_found"}}`,
			wantReason: ReasonModelNotFound, wantRetryable: false, wantFallback: true,
		},
		{
			name:   "404 without model pattern is unknown",
			status: 404, body: `{"error":{"message":"no route"}}`,
			wantReason: ReasonUnknown, wantRetryable: false, wantFallback: false,
		},
		{
			name:   "500 is server_error",
			status: 500, body: `{"error":{"message":"internal server error"}}`,
			wantReason: ReasonServerError, wantRetryable: true, wantFallback: true,
		},
		{
			name:   "529 is overloaded",
			status: 529, body: `{"error":{"message":"Overloaded","type":"overloaded_error"}}`,
			wantReason: ReasonOverloaded, wantRetryable: true, wantFallback: true,
		},
		{
			name:   "503 with overloaded body is overloaded",
			status: 503, body: `{"error":{"message":"The server is overloaded, try again"}}`,
			wantReason: ReasonOverloaded, wantRetryable: true, wantFallback: true,
		},
		{
			name:   "400 with context_length_exceeded code is context_overflow",
			status: 400, body: `{"error":{"message":"This model's maximum context length is 128000 tokens.","code":"context_length_exceeded"}}`,
			wantReason: ReasonContextOverflow, wantRetryable: false, wantFallback: false,
		},
		{
			name:   "400 with prompt-too-long message is context_overflow",
			status: 400, body: `{"error":{"message":"prompt is too long: 250000 tokens > 200000 maximum","type":"invalid_request_error"}}`,
			wantReason: ReasonContextOverflow, wantRetryable: false, wantFallback: false,
		},
		{
			name:   "400 with credit balance message is billing",
			status: 400, body: `{"error":{"message":"Your credit balance is too low to access the Anthropic API.","type":"invalid_request_error"}}`,
			wantReason: ReasonBilling, wantRetryable: false, wantFallback: true,
		},
		{
			name:   "400 with content policy is content_policy",
			status: 400, body: `{"error":{"message":"Your request was flagged as potentially violating our usage policy.","code":"content_policy_violation"}}`,
			wantReason: ReasonContentPolicy, wantRetryable: false, wantFallback: false,
		},
		{
			name:   "400 with unrecognized body is unknown",
			status: 400, body: `{"error":{"message":"something odd happened"}}`,
			wantReason: ReasonUnknown, wantRetryable: false, wantFallback: false,
		},
		{
			name:   "non-JSON body still classifies by status",
			status: 500, body: `upstream connect error`,
			wantReason: ReasonServerError, wantRetryable: true, wantFallback: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ce := Classify(tt.err, tt.status, []byte(tt.body), "testprov", "test-model")
			if ce.Reason != tt.wantReason {
				t.Errorf("Reason = %s, want %s", ce.Reason, tt.wantReason)
			}
			if ce.Retryable != tt.wantRetryable {
				t.Errorf("Retryable = %v, want %v", ce.Retryable, tt.wantRetryable)
			}
			if ce.ShouldFallback != tt.wantFallback {
				t.Errorf("ShouldFallback = %v, want %v", ce.ShouldFallback, tt.wantFallback)
			}
			if ce.Provider != "testprov" || ce.Model != "test-model" {
				t.Errorf("provider/model = %s/%s", ce.Provider, ce.Model)
			}
			if ce.StatusCode != tt.status {
				t.Errorf("StatusCode = %d, want %d", ce.StatusCode, tt.status)
			}
		})
	}
}

func TestClassifiedErrorMessage(t *testing.T) {
	ce := Classify(nil, 401, []byte(`{"error":{"message":"invalid x-api-key"}}`), "anthropic", "claude-sonnet-4-6")
	if ce.Message != "invalid x-api-key" {
		t.Errorf("Message = %q", ce.Message)
	}
	errStr := ce.Error()
	for _, want := range []string{"anthropic", "claude-sonnet-4-6", "invalid x-api-key", "auth", "401"} {
		if !strings.Contains(errStr, want) {
			t.Errorf("Error() = %q missing %q", errStr, want)
		}
	}
}
