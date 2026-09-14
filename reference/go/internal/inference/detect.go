package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"strings"

	"github.com/itsmingjie/inference-discovery/reference/go/internal/descriptor"
	"github.com/tmaxmax/go-sse"
)

// Detect uses separate synthetic requests, never a user's conversation. A failed
// probe is inconclusive, not proof of an unsupported API. Results are cached by
// the advertiser, not by the inference client.
func (c Client) Detect(ctx context.Context, model string) (descriptor.ModelAPI, error) {
	api := descriptor.ModelAPI{Profiles: []string{}, Capabilities: []string{"streaming"}}
	var problems []string
	for _, profile := range []string{descriptor.Profile, descriptor.ResponsesProfile} {
		if err := c.probe(ctx, model, profile, false); err != nil {
			problems = append(problems, profile+": "+err.Error())
			continue
		}
		toolErr := c.probe(ctx, model, profile, true)
		// The Responses contract includes tools; Chat Completions also permits text-only models.
		if profile == descriptor.ResponsesProfile && toolErr != nil {
			problems = append(problems, profile+" tools: "+toolErr.Error())
			continue
		}
		api.Profiles = append(api.Profiles, profile)
		if profile == descriptor.Profile && toolErr == nil {
			api.Capabilities = append(api.Capabilities, "function-tools")
		}
	}
	if len(api.Profiles) == 0 {
		return api, fmt.Errorf("no API confirmed (%s); supply model API metadata to skip probes", strings.Join(problems, "; "))
	}
	return api, nil
}

func (c Client) probe(ctx context.Context, model, profile string, tools bool) error {
	prompt := "Reply with the single word OK."
	if tools {
		prompt = "Call the inference_probe function with ok set to true. Do not answer with text."
	}
	params := map[string]any{"model": model, "stream": true}
	function := map[string]any{"name": "inference_probe", "description": "A discovery check; no action is executed.", "parameters": map[string]any{"type": "object", "properties": map[string]any{"ok": map[string]string{"type": "boolean"}}, "required": []string{"ok"}, "additionalProperties": false}}
	path := "chat/completions"
	if profile == descriptor.Profile {
		params["messages"] = []Message{{Role: "user", Content: prompt}}
		params["max_tokens"] = 128
		if tools {
			params["tools"] = []any{map[string]any{"type": "function", "function": function}}
		}
	} else {
		path = "responses"
		params["input"] = prompt
		params["max_output_tokens"] = 128
		if tools {
			function["type"] = "function"
			params["tools"] = []any{function}
		}
	}
	if tools {
		params["tool_choice"] = "auto"
	}
	b, _ := json.Marshal(params)
	res, err := c.request(ctx, "POST", path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	kind, _, _ := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if kind != "text/event-stream" {
		return fmt.Errorf("expected an SSE stream")
	}
	return readProbe(res.Body, profile, tools)
}

// readProbe checks semantic evidence, not route existence or acceptance of an
// unknown parameter. SSE framing remains the library's responsibility.
func readProbe(body io.Reader, profile string, tools bool) error {
	limited := &io.LimitedReader{R: body, N: 65537}
	var text, name, arguments, callID string
	finished := false
	done := false
	for event, err := range sse.Read(limited, &sse.ReadConfig{MaxEventSize: 16384}) {
		if limited.N <= 0 {
			return fmt.Errorf("probe exceeds 64 KiB")
		}
		if err != nil {
			return err
		}
		if profile == descriptor.Profile && event.Data == "[DONE]" {
			done = true
			if !tools {
				finished = true
			}
			break
		}
		var e struct {
			Type    string          `json:"type"`
			Delta   string          `json:"delta"`
			Error   json.RawMessage `json:"error"`
			Choices []struct {
				Index  int    `json:"index"`
				Finish string `json:"finish_reason"`
				Delta  struct {
					Content string `json:"content"`
					Tools   []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
			Item struct {
				Type      string `json:"type"`
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
				CallID    string `json:"call_id"`
			} `json:"item"`
			Response struct {
				Status string `json:"status"`
			} `json:"response"`
		}
		if err := json.Unmarshal([]byte(event.Data), &e); err != nil {
			return fmt.Errorf("malformed probe event")
		}
		if len(e.Error) > 0 && string(e.Error) != "null" {
			return fmt.Errorf("upstream probe error")
		}
		if profile == descriptor.Profile {
			for _, choice := range e.Choices {
				if choice.Index != 0 {
					continue
				}
				text += choice.Delta.Content
				for _, tool := range choice.Delta.Tools {
					if tool.Index != 0 {
						continue
					}
					callID += tool.ID
					name += tool.Function.Name
					arguments += tool.Function.Arguments
				}
				if choice.Finish == "tool_calls" {
					finished = true
				}
			}
		} else {
			switch e.Type {
			case "response.output_text.delta":
				text += e.Delta
			case "response.output_item.done":
				if e.Item.Type == "function_call" {
					name, arguments, callID = e.Item.Name, e.Item.Arguments, e.Item.CallID
				}
			case "response.failed", "response.incomplete", "error":
				return fmt.Errorf("probe did not complete")
			case "response.completed":
				finished = e.Response.Status == "completed"
			}
			if finished {
				break
			}
		}
	}
	if !finished || (profile == descriptor.Profile && !done) {
		return fmt.Errorf("probe stream interrupted")
	}
	if tools {
		var args map[string]bool
		if name != "inference_probe" || callID == "" || descriptor.CheckJSON([]byte(arguments)) != nil || json.Unmarshal([]byte(arguments), &args) != nil || len(args) != 1 || !args["ok"] {
			return fmt.Errorf("no valid function call observed")
		}
	} else if strings.TrimSpace(text) == "" {
		return fmt.Errorf("no text observed")
	}
	return nil
}
