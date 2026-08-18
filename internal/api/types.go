package api

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// FlexibleFloat parses strings or numbers into a float64
type FlexibleFloat float64

func (f *FlexibleFloat) UnmarshalJSON(data []byte) error {
	if string(data) == "null" || len(data) == 0 {
		*f = 0
		return nil
	}
	var num float64
	if err := json.Unmarshal(data, &num); err == nil {
		*f = FlexibleFloat(num)
		return nil
	}
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		val, err := strconv.ParseFloat(str, 64)
		if err != nil {
			*f = 0
			return nil
		}
		*f = FlexibleFloat(val)
		return nil
	}
	return fmt.Errorf("cannot unmarshal %s into FlexibleFloat", string(data))
}

func (f FlexibleFloat) Float64() float64 {
	return float64(f)
}

// Pricing holds per-token pricing (usually expressed in dollars per token)
type Pricing struct {
	Prompt          FlexibleFloat `json:"prompt"`
	Completion      FlexibleFloat `json:"completion"`
	InputCacheRead  FlexibleFloat `json:"input_cache_read"`
	InputCacheWrite FlexibleFloat `json:"input_cache_write"`
	Request         FlexibleFloat `json:"request"`
	Image           FlexibleFloat `json:"image"`
	Discount        FlexibleFloat `json:"discount"`
}

// Endpoint represents a provider hosting a specific model
type Endpoint struct {
	Name                    string         `json:"name"`
	ModelID                 string         `json:"model_id"`
	ModelName               string         `json:"model_name"`
	ContextLength           int64          `json:"context_length"`
	Pricing                 Pricing        `json:"pricing"`
	ProviderName            string         `json:"provider_name"`
	Tag                     string         `json:"tag"`
	Quantization            string         `json:"quantization"`
	MaxCompletionTokens     int64          `json:"max_completion_tokens"`
	Status                  int            `json:"status"` // 0: healthy/online
	UptimeLast5m            *FlexibleFloat `json:"uptime_last_5m"`
	UptimeLast30m           *FlexibleFloat `json:"uptime_last_30m"`
	UptimeLast1d            *FlexibleFloat `json:"uptime_last_1d"`
	SupportsImplicitCaching bool           `json:"supports_implicit_caching"`
	LatencyLast30m          *FlexibleFloat `json:"latency_last_30m"`
	ThroughputLast30m       *FlexibleFloat `json:"throughput_last_30m"`
}

// EndpointsResponse is the wrapper returned by /api/v1/models/{model_id}/endpoints
type EndpointsResponse struct {
	Data struct {
		ID        string     `json:"id"`
		Name      string     `json:"name"`
		Endpoints []Endpoint `json:"endpoints"`
	} `json:"data"`
}

// ModelSummary holds basic info from the all-models list
type ModelSummary struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	ContextLength int64   `json:"context_length"`
	Pricing       Pricing `json:"pricing"`
}

// ModelsListResponse is returned by /api/v1/models
type ModelsListResponse struct {
	Data []ModelSummary `json:"data"`
}
