package axiosd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/axios-os/axios/pkg/providers"
)

// wsSink is the minimal surface the chat loop needs to talk to a client.
// *websocket.Conn satisfies it; tests supply a fake.
type wsSink interface {
	WriteJSON(v any) error
}

// FallbackSpec is one entry of the provider fallback chain (from config
// fallback_providers).
type FallbackSpec struct {
	Provider string
	Model    string
}

// chatClient is the provider surface the loop depends on. *providers.Client
// satisfies it; tests supply a fake.
type chatClient interface {
	Name() string
	Model() string
	Stream(ctx context.Context, system string, msgs []providers.Message, tools []providers.ToolDef, onDelta func(string)) (*providers.NormalizedResponse, error)
}

const (
	// maxLoopIterations bounds the agentic tool loop.
	maxLoopIterations = 20
	// maxProviderAttempts bounds retries against one provider per completion.
	maxProviderAttempts = 3
	// defaultRetryBackoff is the base delay between retries (doubled per attempt).
	defaultRetryBackoff = 500 * time.Millisecond
)

// chatLoop is the ONE agentic loop for every provider: complete → execute
// tool calls → append canonical role:"tool" results → continue, streaming
// text deltas to the sink as they arrive. Retries follow the error
// classifier's hints; fallback-worthy failures advance the configured
// fallback chain, swapping the client in place.
type chatLoop struct {
	client      chatClient
	system      string
	tools       []providers.ToolDef
	fallbacks   []FallbackSpec
	buildClient func(provider, model string) (chatClient, error)
	execTool    func(toolName, toolID string, rawInput json.RawMessage) string
	logger      *slog.Logger

	// sleep and backoffBase are injectable for tests.
	sleep       func(time.Duration)
	backoffBase time.Duration
}

// run drives the agentic loop for one user turn. The session already contains
// the new user message; assistant/tool messages are appended as they happen.
func (cl *chatLoop) run(ctx context.Context, sink wsSink, session *Session) {
	if cl.sleep == nil {
		cl.sleep = time.Sleep
	}
	if cl.backoffBase <= 0 {
		cl.backoffBase = defaultRetryBackoff
	}
	if cl.logger == nil {
		cl.logger = slog.Default()
	}

	fallbackIdx := 0

	for iter := 0; iter < maxLoopIterations; iter++ {
		msgs := session.GetMessages()
		cl.logger.Info("chat loop completion",
			"provider", cl.client.Name(),
			"model", cl.client.Model(),
			"messages", len(msgs),
			"iteration", iter,
		)

		resp, deltaSent, ok := cl.complete(ctx, sink, msgs, &fallbackIdx)
		if !ok {
			return
		}

		// If the transport buffered (or produced no deltas), deliver the
		// full text once so the UI still sees the assistant reply.
		if !deltaSent && resp.Content != "" {
			cl.send(sink, ChatMessage{
				Type:     "assistant",
				Content:  resp.Content,
				Model:    cl.client.Model(),
				Provider: cl.client.Name(),
			})
		}

		// Record the assistant turn canonically (text + tool calls + any
		// transport replay state).
		session.AddMessage(providers.Message{
			Role:         "assistant",
			Content:      resp.Content,
			ToolCalls:    resp.ToolCalls,
			ProviderData: resp.ProviderData,
		})

		if resp.FinishReason != providers.FinishToolCalls || len(resp.ToolCalls) == 0 {
			cl.logger.Info("chat loop done",
				"finish_reason", resp.FinishReason,
				"tool_calls", len(resp.ToolCalls),
				"content_len", len(resp.Content),
			)
			return
		}

		// Execute every requested tool and append canonical role:"tool" results.
		for _, tc := range resp.ToolCalls {
			args := tc.Arguments
			if strings.TrimSpace(args) == "" {
				args = "{}"
			}

			cl.logger.Info("tool use requested", "tool", tc.Name, "id", tc.ID)
			cl.send(sink, ChatMessage{
				Type:     "tool_use",
				ToolName: tc.Name,
				ToolID:   tc.ID,
				Content:  args,
			})

			result := cl.execTool(tc.Name, tc.ID, json.RawMessage(args))

			cl.send(sink, ChatMessage{
				Type:     "tool_result",
				ToolID:   tc.ID,
				ToolName: tc.Name,
				Content:  result,
			})

			session.AddMessage(providers.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
				Name:       tc.Name,
			})
		}
	}

	cl.logger.Warn("hit max agentic loop iterations", "max", maxLoopIterations)
}

// complete performs one completion with retry (per the classifier's hints)
// and fallback-chain advancement. It returns the response, whether any text
// deltas were already streamed to the sink, and whether the loop may continue.
func (cl *chatLoop) complete(ctx context.Context, sink wsSink, msgs []providers.Message, fallbackIdx *int) (*providers.NormalizedResponse, bool, bool) {
	attempt := 0

	for {
		deltaSent := false
		onDelta := func(text string) {
			if text == "" {
				return
			}
			deltaSent = true
			cl.send(sink, ChatMessage{
				Type:     "assistant",
				Content:  text,
				Model:    cl.client.Model(),
				Provider: cl.client.Name(),
			})
		}

		resp, err := cl.client.Stream(ctx, cl.system, msgs, cl.tools, onDelta)
		if err == nil {
			return resp, deltaSent, true
		}

		var ce *providers.ClassifiedError
		if errors.As(err, &ce) {
			if ce.Retryable && attempt+1 < maxProviderAttempts {
				attempt++
				delay := cl.backoffBase << (attempt - 1)
				cl.logger.Warn("provider request failed; retrying",
					"provider", cl.client.Name(),
					"model", cl.client.Model(),
					"reason", string(ce.Reason),
					"attempt", attempt+1,
					"delay", delay,
				)
				cl.sleep(delay)
				continue
			}
			if ce.ShouldFallback && cl.advanceFallback(sink, fallbackIdx, ce) {
				attempt = 0
				continue
			}
		}

		cl.logger.Error("provider request failed", "provider", cl.client.Name(), "model", cl.client.Model(), "error", err)
		cl.send(sink, ChatMessage{
			Type:    "error",
			Content: fmt.Sprintf("AI request failed: %v", err),
		})
		return nil, deltaSent, false
	}
}

// advanceFallback swaps the client to the next viable entry of the fallback
// chain, announcing the switch on the sink. Returns false when the chain is
// exhausted (or no chain/builder is configured).
func (cl *chatLoop) advanceFallback(sink wsSink, idx *int, cause *providers.ClassifiedError) bool {
	if cl.buildClient == nil {
		return false
	}

	for *idx < len(cl.fallbacks) {
		fb := cl.fallbacks[*idx]
		*idx++

		next, err := cl.buildClient(fb.Provider, fb.Model)
		if err != nil {
			cl.logger.Warn("fallback provider unavailable", "provider", fb.Provider, "model", fb.Model, "error", err)
			continue
		}

		prevName, prevModel := cl.client.Name(), cl.client.Model()
		cl.client = next
		cl.logger.Info("advanced provider fallback chain",
			"from_provider", prevName,
			"from_model", prevModel,
			"to_provider", next.Name(),
			"to_model", next.Model(),
			"reason", string(cause.Reason),
		)
		cl.send(sink, ChatMessage{
			Type: "status",
			Content: fmt.Sprintf("Provider %s (%s) failed (%s) — switching to fallback %s (%s).",
				prevName, prevModel, cause.Reason, next.Name(), next.Model()),
		})
		return true
	}
	return false
}

// send writes one message to the sink, logging (but not failing on) errors.
func (cl *chatLoop) send(sink wsSink, msg ChatMessage) {
	if err := sink.WriteJSON(msg); err != nil {
		cl.logger.Error("chat sink write failed", "error", err)
	}
}

// runChatLoop wires a chatLoop from server state and runs it against the
// given client (resolved by the caller from the provider runtime).
func (s *Server) runChatLoop(ctx context.Context, sink wsSink, session *Session, client *providers.Client) {
	loop := &chatLoop{
		client:    client,
		system:    s.system,
		tools:     s.toolDefs,
		fallbacks: s.fallbacks,
		buildClient: func(provider, model string) (chatClient, error) {
			if s.runtime == nil {
				return nil, fmt.Errorf("provider runtime not initialized")
			}
			return s.runtime.ClientFor(provider, model)
		},
		// The permission middleware needs the connection's sink (approval
		// requests) and context (cancellation of in-flight approval waits).
		execTool: func(toolName, toolID string, rawInput json.RawMessage) string {
			return s.executeTool(ctx, sink, toolName, toolID, rawInput)
		},
		logger: s.logger,
	}
	loop.run(ctx, sink, session)
}
