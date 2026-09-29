package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joegabby/alibi/cli/internal/cli/types"
)

// Model catalogues are fetched live from each provider rather than kept in a
// list here. A hardcoded list is how the OpenRouter default drifted to a model
// id that no longer exists — the call fails at summary time with a 400 and
// nothing points at the cause.
//
// Two of the four providers publish pricing:
//
//	OpenRouter   pricing.prompt / pricing.completion, USD per token, no key needed
//	HuggingFace  providers[].pricing.input / .output, USD per million tokens,
//	             plus an explicit is_free flag, no key needed
//	OpenAI       ids only
//	DeepSeek     ids only
//
// For the latter two PricingKnown stays false and the dashboard shows no badge.

const modelsCacheTTL = 10 * time.Minute

type cachedModels struct {
	models  []types.ModelInfo
	fetched time.Time
}

var (
	modelsCacheMu sync.Mutex
	modelsCache   = map[string]cachedModels{}
)

// ListModels returns the models provider can serve, newest cache first.
// Results are cached in memory for modelsCacheTTL so opening the dropdown
// repeatedly does not hammer the provider.
func ListModels(ctx context.Context, provider types.AIProvider) ([]types.ModelInfo, error) {
	key := strings.ToLower(provider.Name)

	modelsCacheMu.Lock()
	if hit, ok := modelsCache[key]; ok && time.Since(hit.fetched) < modelsCacheTTL {
		modelsCacheMu.Unlock()
		return hit.models, nil
	}
	modelsCacheMu.Unlock()

	var (
		models []types.ModelInfo
		err    error
	)

	switch provider.Name {
	case "OpenAI":
		models, err = listOpenAICompatible(ctx,
			"https://api.openai.com/v1/models", provider.Key)
	case "DeepSeek":
		models, err = listOpenAICompatible(ctx,
			"https://api.deepseek.com/models", provider.Key)
	case "OpenRouter":
		models, err = listOpenRouter(ctx)
	case "HuggingFace":
		models, err = listHuggingFace(ctx)
	default:
		return nil, fmt.Errorf("unsupported AI provider: %s", provider.Name)
	}

	if err != nil {
		return nil, err
	}

	sortModels(models)

	modelsCacheMu.Lock()
	modelsCache[key] = cachedModels{models: models, fetched: time.Now()}
	modelsCacheMu.Unlock()

	return models, nil
}

// ListModelsForProject resolves the provider's API key from the project's
// info.txt and returns that provider's catalogue. The key stays on this side of
// the wire — the dashboard only ever receives model ids and prices.
func ListModelsForProject(ctx context.Context, providerName, projectDir string) ([]types.ModelInfo, error) {
	provider, err := getAIProvider(providerName, projectDir)
	if err != nil {
		return nil, err
	}
	return ListModels(ctx, provider)
}

// sortModels puts free models first, then orders by id so the dropdown is
// stable between refreshes.
func sortModels(models []types.ModelInfo) {
	sort.SliceStable(models, func(i, j int) bool {
		if models[i].Free != models[j].Free {
			return models[i].Free
		}
		return models[i].ID < models[j].ID
	})
}

func getJSON(ctx context.Context, url, apiKey string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("model list request failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("failed to decode model list: %w", err)
	}
	return nil
}

// listOpenAICompatible handles the plain /v1/models shape used by OpenAI and
// DeepSeek: an array of ids with no pricing information of any kind.
func listOpenAICompatible(ctx context.Context, url, apiKey string) ([]types.ModelInfo, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("an API key is required to list models for this provider")
	}

	var res struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := getJSON(ctx, url, apiKey, &res); err != nil {
		return nil, err
	}

	models := make([]types.ModelInfo, 0, len(res.Data))
	for _, m := range res.Data {
		if m.ID == "" {
			continue
		}
		models = append(models, types.ModelInfo{
			ID:           m.ID,
			Name:         m.ID,
			PricingKnown: false,
		})
	}
	return models, nil
}

// listOpenRouter reads the public catalogue. Pricing is reported as a decimal
// string of US dollars per single token, so a model at "0.0000005" costs
// $0.50 per million.
func listOpenRouter(ctx context.Context) ([]types.ModelInfo, error) {
	var res struct {
		Data []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			ContextLength int    `json:"context_length"`
			Pricing       struct {
				Prompt     string `json:"prompt"`
				Completion string `json:"completion"`
			} `json:"pricing"`
		} `json:"data"`
	}
	if err := getJSON(ctx, "https://openrouter.ai/api/v1/models", "", &res); err != nil {
		return nil, err
	}

	models := make([]types.ModelInfo, 0, len(res.Data))
	for _, m := range res.Data {
		if m.ID == "" {
			continue
		}

		in, inOK := parsePrice(m.Pricing.Prompt)
		out, outOK := parsePrice(m.Pricing.Completion)
		known := inOK && outOK

		name := m.Name
		if name == "" {
			name = m.ID
		}

		models = append(models, types.ModelInfo{
			ID:            m.ID,
			Name:          name,
			Free:          known && in == 0 && out == 0,
			PricingKnown:  known,
			InputPer1M:    in * 1_000_000,
			OutputPer1M:   out * 1_000_000,
			ContextLength: m.ContextLength,
		})
	}
	return models, nil
}

// listHuggingFace reads the public router catalogue. Each model is served by
// one or more inference providers, each with its own is_free flag and — only
// sometimes — a pricing object. A model counts as free when any live provider
// serves it free, and its quoted price is the cheapest on offer.
//
// Pricing is a pointer because plenty of entries omit it altogether. Decoding a
// missing object into a value type yields zeros, which would advertise a paid
// model as costing nothing.
func listHuggingFace(ctx context.Context) ([]types.ModelInfo, error) {
	var res struct {
		Data []struct {
			ID        string `json:"id"`
			Providers []struct {
				Provider      string `json:"provider"`
				Status        string `json:"status"`
				ContextLength int    `json:"context_length"`
				IsFree        bool   `json:"is_free"`
				Pricing       *struct {
					Input  float64 `json:"input"`
					Output float64 `json:"output"`
				} `json:"pricing"`
			} `json:"providers"`
		} `json:"data"`
	}
	if err := getJSON(ctx, "https://router.huggingface.co/v1/models", "", &res); err != nil {
		return nil, err
	}

	models := make([]types.ModelInfo, 0, len(res.Data))
	for _, m := range res.Data {
		if m.ID == "" {
			continue
		}

		info := types.ModelInfo{ID: m.ID, Name: m.ID}
		cheapest := -1.0

		for _, p := range m.Providers {
			if p.Status != "" && p.Status != "live" {
				continue
			}

			// is_free is reported even when pricing is absent, so a free route
			// is knowable on its own.
			if p.IsFree {
				info.Free = true
				info.PricingKnown = true
			}
			if p.ContextLength > info.ContextLength {
				info.ContextLength = p.ContextLength
			}
			if p.Pricing != nil {
				info.PricingKnown = true
				if cheapest < 0 || p.Pricing.Input < cheapest {
					cheapest = p.Pricing.Input
					info.InputPer1M = p.Pricing.Input
					info.OutputPer1M = p.Pricing.Output
				}
			}
		}

		// A free route exists, so quote it as free rather than showing the
		// cheapest paid provider's numbers next to a Free badge.
		if info.Free {
			info.InputPer1M, info.OutputPer1M = 0, 0
		}

		models = append(models, info)
	}
	return models, nil
}

// parsePrice reads one of OpenRouter's decimal price strings. An empty or
// unparseable value means the price is unknown rather than zero — treating it
// as zero would advertise a paid model as free.
func parsePrice(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 {
		return 0, false
	}
	return v, true
}
