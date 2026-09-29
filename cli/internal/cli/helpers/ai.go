package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/joegabby/alibi/cli/internal/cli/types"
)

// defaultModels is only a fallback for when no model is chosen. These ids are
// verified against each provider's live catalogue at the time of writing, but
// they do rot — an earlier hardcoded OpenRouter default drifted to an id that
// no longer existed and every call failed with a 400. Pick a model in the
// dashboard rather than relying on these.
var defaultModels = map[string]string{
	"OpenAI":      "gpt-4o-mini",
	"DeepSeek":    "deepseek-chat",
	"OpenRouter":  "poolside/laguna-xs-2.1:free",
	"HuggingFace": "meta-llama/Meta-Llama-3-8B-Instruct",
}

// chatClient deliberately carries a very generous timeout. The real deadline is
// the caller's context (see llmTimeout in summary.go); this only exists so a
// connection that stalls with no context deadline cannot hang forever.
//
// It used to be 30s, which was shorter than the context deadline and shorter
// than a large diff takes to summarise. Two competing timeouts produced two
// different failures: firing before response headers surfaced as
// "Client.Timeout exceeded while awaiting headers", and firing during the body
// read truncated the response into "unexpected end of JSON input".
var chatClient = &http.Client{Timeout: 5 * time.Minute}

// resolveModel returns the caller's chosen model, falling back to the
// provider's default when none was supplied.
func resolveModel(provider types.AIProvider) string {
	if m := strings.TrimSpace(provider.Model); m != "" {
		return m
	}
	return defaultModels[provider.Name]
}

func GetAISummary(ctx context.Context, prompt string, provider types.AIProvider) (string, error) {
	model := resolveModel(provider)

	switch provider.Name {
	case "OpenAI":
		return callOpenAI(ctx, prompt, provider.Key, model)
	case "DeepSeek":
		return callDeepSeek(ctx, prompt, provider.Key, model)
	case "OpenRouter":
		return callOpenRouter(ctx, prompt, provider.Key, model)
	case "HuggingFace":
		return callHuggingFace(ctx, prompt, provider.Key, model)
	default:
		return "", fmt.Errorf("unsupported AI provider: %s", provider.Name)
	}
}

// chatBody builds the OpenAI-shaped request body every provider here accepts.
func chatBody(model, prompt string) map[string]interface{} {
	return map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
}

// postChat performs one chat-completion call. All four providers speak the same
// dialect, so they share this path — which also means the response handling is
// correct in one place rather than copied four times with the same bug.
func postChat(ctx context.Context, provider, url, apiKey string, body map[string]interface{}) (string, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := chatClient.Do(req)
	if err != nil {
		// Separate "we ran out of time" from "the network broke", because the
		// two call for completely different responses from the user.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", fmt.Errorf(
				"%s did not respond in time (model %v) — try a smaller commit or a faster model: %w",
				provider, body["model"], ctxErr)
		}
		return "", fmt.Errorf("network error calling %s: %w", provider, err)
	}
	defer resp.Body.Close()

	return readChatResponse(ctx, resp, provider)
}

// readChatResponse reads and decodes an OpenAI-shaped chat completion.
//
// Every failure mode below previously collapsed into the same misleading
// "failed to decode JSON: unexpected end of JSON input" message, because the
// error from io.ReadAll was discarded and an aborted read simply produced an
// empty slice for the decoder to choke on.
func readChatResponse(ctx context.Context, resp *http.Response, provider string) (string, error) {
	bodyBytes, readErr := io.ReadAll(resp.Body)

	// A cancelled or expired context aborts the read part-way. Report that
	// rather than the empty-body symptom it creates.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", fmt.Errorf(
			"%s response interrupted after %d bytes: %w",
			provider, len(bodyBytes), ctxErr)
	}
	if readErr != nil {
		return "", fmt.Errorf(
			"failed to read the %s response after %d bytes: %w",
			provider, len(bodyBytes), readErr)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s API error (%d): %s",
			provider, resp.StatusCode, truncate(strings.TrimSpace(string(bodyBytes)), 500))
	}

	if len(bytes.TrimSpace(bodyBytes)) == 0 {
		return "", fmt.Errorf("%s returned status 200 with an empty body", provider)
	}

	var res struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		// Gateways such as OpenRouter sometimes answer 200 with an error object
		// instead of choices; without this the user just saw "no response".
		Error *struct {
			Message string `json:"message"`
			Code    any    `json:"code"`
		} `json:"error"`
	}

	if err := json.Unmarshal(bodyBytes, &res); err != nil {
		return "", fmt.Errorf("failed to decode the %s response: %w\nbody: %s",
			provider, err, truncate(string(bodyBytes), 500))
	}

	if res.Error != nil && strings.TrimSpace(res.Error.Message) != "" {
		return "", fmt.Errorf("%s error: %s", provider, res.Error.Message)
	}

	if len(res.Choices) == 0 {
		return "", fmt.Errorf("no response from %s, body: %s",
			provider, truncate(string(bodyBytes), 500))
	}

	return res.Choices[0].Message.Content, nil
}

// truncate keeps error messages readable when a provider returns a large body.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("… (%d bytes total)", len(s))
}

func callOpenAI(ctx context.Context, prompt, apiKey, model string) (string, error) {
	if apiKey == "" {
		return "", errors.New("no OpenAI API key configured — add one to alibi.yml")
	}
	return postChat(ctx, "OpenAI",
		"https://api.openai.com/v1/chat/completions", apiKey, chatBody(model, prompt))
}

func callDeepSeek(ctx context.Context, prompt, apiKey, model string) (string, error) {
	if apiKey == "" {
		return "", errors.New("no DeepSeek API key configured — add one to alibi.yml")
	}
	return postChat(ctx, "DeepSeek",
		"https://api.deepseek.com/v1/chat/completions", apiKey, chatBody(model, prompt))
}

func callOpenRouter(ctx context.Context, prompt, apiKey, model string) (string, error) {
	if apiKey == "" {
		return "", errors.New("no OpenRouter API key configured — add one to alibi.yml")
	}
	return postChat(ctx, "OpenRouter",
		"https://openrouter.ai/api/v1/chat/completions", apiKey, chatBody(model, prompt))
}

func callHuggingFace(ctx context.Context, prompt, apiKey, model string) (string, error) {
	if apiKey == "" {
		return "", errors.New("no HuggingFace API key configured — add one to alibi.yml")
	}

	body := chatBody(model, prompt)
	body["temperature"] = 0.7
	body["max_tokens"] = 500

	return postChat(ctx, "HuggingFace",
		"https://router.huggingface.co/v1/chat/completions", apiKey, body)
}
