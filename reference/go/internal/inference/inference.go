// Package inference implements the text Chat Completions profile without retries.
package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"slices"
	"strings"

	"github.com/tmaxmax/go-sse"

	"github.com/itsmingjie/inference-discovery/reference/go/internal/authentication"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/descriptor"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/transport"
)

const MaxResponse = 1048576

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// assistantPayload is shared by complete messages and streaming deltas.
type assistantPayload struct {
	Content   *string         `json:"content"`
	ToolCalls json.RawMessage `json:"tool_calls"`
}

func (p assistantPayload) validate() error {
	if len(p.ToolCalls) > 0 && string(p.ToolCalls) != "null" {
		return fmt.Errorf("unsupported tool-call completion")
	}
	return nil
}

type Client struct {
	HTTP    *http.Client
	Base    string
	Session authentication.Session
}

func (c Client) request(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	if c.Session == nil {
		return nil, fmt.Errorf("authentication must be resolved before endpoint access")
	}
	raw, err := descriptor.APIURL(c.Base, path)
	if err != nil {
		return nil, err
	}
	return transport.AuthorizedRequest(ctx, c.HTTP, method, raw, body, c.Session.Authorize)
}

func (c Client) Models(ctx context.Context) ([]string, error) {
	res, err := c.request(ctx, "GET", "models", nil)
	if err != nil {
		return nil, err
	}
	b, err := transport.Read(res, MaxResponse, "application/json")
	if err != nil {
		return nil, err
	}
	var response struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err = json.Unmarshal(b, &response); err != nil {
		return nil, err
	}
	if len(response.Data) == 0 || len(response.Data) > 4096 {
		return nil, fmt.Errorf("model list must contain 1..4096 models")
	}
	models := []string{}
	for _, m := range response.Data {
		if !descriptor.Text(m.ID, 256) {
			return nil, fmt.Errorf("invalid model ID")
		}
		models = append(models, m.ID)
	}
	slices.Sort(models)
	return slices.Compact(models), nil
}

func SelectModel(models []string, def, override string) (string, error) {
	chosen := override
	if chosen == "" {
		chosen = def
	}
	if chosen != "" {
		if slices.Contains(models, chosen) {
			return chosen, nil
		}
		return "", fmt.Errorf("selected/default model %q is unavailable", chosen)
	}
	if len(models) == 0 {
		return "", fmt.Errorf("no models available")
	}
	return models[0], nil
}

func (c Client) Chat(ctx context.Context, model string, messages []Message, stream bool, emit func(string) error) (string, error) {
	if !descriptor.Text(model, 256) || len(messages) == 0 || len(messages) > 128 {
		return "", fmt.Errorf("invalid model or conversation length (maximum 128 messages)")
	}
	b, err := json.Marshal(struct {
		Model    string    `json:"model"`
		Messages []Message `json:"messages"`
		Stream   bool      `json:"stream"`
	}{model, messages, stream})
	if err != nil {
		return "", err
	}
	if len(b) > MaxResponse {
		return "", fmt.Errorf("conversation exceeds 1 MiB; start a new conversation")
	}
	res, err := c.request(ctx, "POST", "chat/completions", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	if stream {
		defer res.Body.Close()
		kind, _, _ := mime.ParseMediaType(res.Header.Get("Content-Type"))
		if kind != "text/event-stream" {
			return "", fmt.Errorf("expected text/event-stream")
		}
		return readStream(res.Body, emit)
	}
	b, err = transport.Read(res, MaxResponse, "application/json")
	if err != nil {
		return "", err
	}
	var r struct {
		Choices []struct {
			Message assistantPayload `json:"message"`
		} `json:"choices"`
		Error json.RawMessage `json:"error"`
	}
	if err = json.Unmarshal(b, &r); err != nil {
		return "", err
	}
	if len(r.Error) > 0 || len(r.Choices) == 0 {
		return "", fmt.Errorf("invalid text completion or upstream error")
	}
	if err := r.Choices[0].Message.validate(); err != nil {
		return "", err
	}
	if r.Choices[0].Message.Content == nil {
		return "", fmt.Errorf("invalid text completion: missing content")
	}
	s := *r.Choices[0].Message.Content
	if err = emit(s); err != nil {
		return "", err
	}
	return s, nil
}

// readStream uses SSE framing without reconnection or request retries.
func readStream(r io.Reader, emit func(string) error) (string, error) {
	limited := &io.LimitedReader{R: r, N: 8*MaxResponse + 1}
	var output strings.Builder
	for event, err := range sse.Read(limited, &sse.ReadConfig{MaxEventSize: 65536}) {
		if limited.N <= 0 {
			return output.String(), fmt.Errorf("stream exceeds 8 MiB")
		}
		if err != nil {
			return output.String(), fmt.Errorf("stream interrupted: %w", err)
		}
		if event.Data == "[DONE]" {
			return output.String(), nil
		}
		var chunk struct {
			Choices []struct {
				Index int              `json:"index"`
				Delta assistantPayload `json:"delta"`
			} `json:"choices"`
			Error json.RawMessage `json:"error"`
			Usage json.RawMessage `json:"usage"`
		}
		if err := json.Unmarshal([]byte(event.Data), &chunk); err != nil {
			return output.String(), fmt.Errorf("malformed stream event: %w", err)
		}
		if len(chunk.Error) > 0 {
			return output.String(), fmt.Errorf("upstream streaming error")
		}
		if chunk.Choices == nil && len(chunk.Usage) == 0 {
			return output.String(), fmt.Errorf("stream event missing choices")
		}
		for _, choice := range chunk.Choices {
			if choice.Index != 0 {
				continue
			}
			if err := choice.Delta.validate(); err != nil {
				return output.String(), err
			}
			if choice.Delta.Content == nil {
				continue
			}
			text := *choice.Delta.Content
			if output.Len()+len(text) > MaxResponse {
				return output.String(), fmt.Errorf("completion exceeds 1 MiB")
			}
			output.WriteString(text)
			if err := emit(text); err != nil {
				return output.String(), err
			}
		}
	}
	return output.String(), fmt.Errorf("stream interrupted before [DONE]; request was not replayed")
}
