package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
)

// Reason is a coarse, provider-independent failure category.
type Reason string

const (
	ReasonAuth            Reason = "auth"
	ReasonBilling         Reason = "billing"
	ReasonRateLimit       Reason = "rate_limit"
	ReasonOverloaded      Reason = "overloaded"
	ReasonServerError     Reason = "server_error"
	ReasonTimeout         Reason = "timeout"
	ReasonContextOverflow Reason = "context_overflow"
	ReasonModelNotFound   Reason = "model_not_found"
	ReasonContentPolicy   Reason = "content_policy"
	ReasonNetwork         Reason = "network"
	ReasonUnknown         Reason = "unknown"
)

// ClassifiedError is a provider failure normalized into a Reason plus
// retry/fallback advice. The retry policy itself lives in the daemon, not
// the transport.
type ClassifiedError struct {
	Reason         Reason
	StatusCode     int
	Provider       string
	Model          string
	Message        string
	Retryable      bool
	ShouldFallback bool
}

func (e *ClassifiedError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = "provider request failed"
	}
	if e.StatusCode > 0 {
		return fmt.Sprintf("%s/%s: %s (%s, HTTP %d)", e.Provider, e.Model, msg, e.Reason, e.StatusCode)
	}
	return fmt.Sprintf("%s/%s: %s (%s)", e.Provider, e.Model, msg, e.Reason)
}

// reasonAdvice returns (retryable, shouldFallback) defaults per reason.
func reasonAdvice(r Reason) (bool, bool) {
	switch r {
	case ReasonAuth:
		return false, true
	case ReasonBilling:
		return false, true
	case ReasonRateLimit:
		return true, true
	case ReasonOverloaded:
		return true, true
	case ReasonServerError:
		return true, true
	case ReasonTimeout:
		return true, true
	case ReasonContextOverflow:
		return false, false
	case ReasonModelNotFound:
		return false, true
	case ReasonContentPolicy:
		return false, false
	case ReasonNetwork:
		return true, true
	default:
		return false, false
	}
}

// errorEnvelope matches the common {"error": {...}} shape used by OpenAI,
// Anthropic, and most compatible providers, plus the flat variants.
type errorEnvelope struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"` // string or number depending on provider
	} `json:"error"`
	Message string `json:"message"` // flat form (some providers, ollama)
	Type    string `json:"type"`
}

// extractError pulls a human message and machine code/type out of an error body.
func extractError(body []byte) (message, code string) {
	if len(body) == 0 {
		return "", ""
	}
	var env errorEnvelope
	if err := json.Unmarshal(body, &env); err == nil {
		if env.Error.Message != "" {
			message = env.Error.Message
		} else if env.Message != "" {
			message = env.Message
		}
		if env.Error.Type != "" {
			code = env.Error.Type
		}
		if c, ok := env.Error.Code.(string); ok && c != "" {
			code = c
		}
		if code == "" && env.Type != "" {
			code = env.Type
		}
	}
	if message == "" {
		message = strings.TrimSpace(string(body))
		if len(message) > 500 {
			message = message[:500]
		}
	}
	return message, code
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// Body error codes / message patterns, checked in this order after status codes.
func classifyBody(code, message string) Reason {
	c := strings.ToLower(code)
	m := strings.ToLower(message)

	// Machine-readable error codes first.
	switch {
	case containsAny(c, "context_length_exceeded", "context_length", "max_tokens_exceeded"):
		return ReasonContextOverflow
	case containsAny(c, "insufficient_quota", "billing", "payment_required"):
		return ReasonBilling
	case containsAny(c, "authentication", "invalid_api_key", "permission", "invalid_request_key"):
		return ReasonAuth
	case containsAny(c, "model_not_found"):
		return ReasonModelNotFound
	case containsAny(c, "rate_limit"):
		return ReasonRateLimit
	case containsAny(c, "overloaded"):
		return ReasonOverloaded
	case containsAny(c, "content_policy", "content_filter", "moderation"):
		return ReasonContentPolicy
	case containsAny(c, "timeout"):
		return ReasonTimeout
	}

	// Then free-text message patterns.
	switch {
	case containsAny(m, "context length", "context window", "maximum context", "too many tokens", "prompt is too long"):
		return ReasonContextOverflow
	case containsAny(m, "billing", "credit balance", "insufficient quota", "insufficient credit", "payment required", "purchase credits"):
		return ReasonBilling
	case containsAny(m, "invalid api key", "invalid x-api-key", "incorrect api key", "unauthorized", "authentication"):
		return ReasonAuth
	case containsAny(m, "rate limit", "too many requests"):
		return ReasonRateLimit
	case containsAny(m, "overloaded"):
		return ReasonOverloaded
	case containsAny(m, "content policy", "content filtering", "flagged as potentially violating", "safety system"):
		return ReasonContentPolicy
	case containsAny(m, "timed out", "timeout"):
		return ReasonTimeout
	case isModelNotFoundMessage(m):
		return ReasonModelNotFound
	}
	return ReasonUnknown
}

// isModelNotFoundMessage reports whether a message looks like a missing-model
// error ("model X does not exist", "model not found", ...).
func isModelNotFoundMessage(m string) bool {
	if !strings.Contains(m, "model") {
		return false
	}
	return containsAny(m, "not found", "does not exist", "not supported", "unknown model", "no such model")
}

// Classify normalizes a provider failure. Precedence: transport-level errors
// (timeouts, network), then HTTP status code, then body error codes, then
// message patterns.
func Classify(err error, statusCode int, body []byte, provider, model string) *ClassifiedError {
	ce := &ClassifiedError{
		StatusCode: statusCode,
		Provider:   provider,
		Model:      model,
		Reason:     ReasonUnknown,
	}

	// 1. Transport-level failures (no HTTP status).
	if err != nil {
		ce.Message = err.Error()
		switch {
		case errors.Is(err, context.DeadlineExceeded), errors.Is(err, os.ErrDeadlineExceeded), isNetTimeout(err):
			ce.Reason = ReasonTimeout
		default:
			ce.Reason = ReasonNetwork
		}
		ce.Retryable, ce.ShouldFallback = reasonAdvice(ce.Reason)
		return ce
	}

	message, code := extractError(body)
	ce.Message = message
	bodyReason := classifyBody(code, message)

	// 2. Status code first, refined by body signals where the status is ambiguous.
	switch {
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden: // 401, 403
		ce.Reason = ReasonAuth
	case statusCode == http.StatusPaymentRequired: // 402
		ce.Reason = ReasonBilling
	case statusCode == http.StatusTooManyRequests: // 429
		// OpenAI reports exhausted credit ("insufficient_quota") as 429.
		if bodyReason == ReasonBilling {
			ce.Reason = ReasonBilling
		} else {
			ce.Reason = ReasonRateLimit
		}
	case statusCode == http.StatusRequestTimeout: // 408
		ce.Reason = ReasonTimeout
	case statusCode == http.StatusRequestEntityTooLarge: // 413
		ce.Reason = ReasonContextOverflow
	case statusCode == http.StatusNotFound: // 404
		if bodyReason == ReasonModelNotFound || isModelNotFoundMessage(strings.ToLower(message)) {
			ce.Reason = ReasonModelNotFound
		} else {
			ce.Reason = ReasonUnknown
		}
	case statusCode >= 500: // 5xx
		// 529 is Anthropic's dedicated overloaded status.
		if statusCode == 529 || bodyReason == ReasonOverloaded {
			ce.Reason = ReasonOverloaded
		} else {
			ce.Reason = ReasonServerError
		}
	default:
		// 3./4. Body error codes, then message patterns (e.g. 400s).
		ce.Reason = bodyReason
	}

	ce.Retryable, ce.ShouldFallback = reasonAdvice(ce.Reason)
	return ce
}

// isNetTimeout reports whether err is a net.Error timeout.
func isNetTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
