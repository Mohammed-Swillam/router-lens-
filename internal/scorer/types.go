package scorer

import (
	"strings"

	"routerlens/internal/api"
)

// Profile names
const (
	ProfileBalanced        = "balanced"
	ProfileCoding          = "coding"
	ProfileLowLatency      = "low_latency"
	ProfileBatchThroughput = "batch_throughput"
	ProfileCustom          = "custom"
)

// ScoringConfig holds all weights, cache lifecycle assumptions, and SLA filtering thresholds
type ScoringConfig struct {
	Profile string `json:"profile"` // balanced, coding, low_latency, batch_throughput, custom

	// Scoring Weights (must sum to 1.0 when normalized)
	PriceWeight   float64 `json:"price_weight"`   // w_cost
	QuantWeight   float64 `json:"quant_weight"`   // w_quant
	RelWeight     float64 `json:"rel_weight"`     // w_rel
	TTFTWeight    float64 `json:"ttft_weight"`    // w_ttft
	LatencyWeight float64 `json:"latency_weight"` // w_lat
	TPSWeight     float64 `json:"tps_weight"`     // w_tps

	// Token Mix & Cache Economics
	PromptRatio       float64 `json:"prompt_ratio"`        // Default: 0.75 (3:1 prompt:completion ratio)
	CompletionRatio   float64 `json:"completion_ratio"`    // Default: 0.25
	CacheHitRate      float64 `json:"cache_hit_rate"`      // Base hit rate: default 0.80
	CacheWriteRatio   float64 `json:"cache_write_ratio"`   // Fraction of prompt tokens requiring cache write: default 0.10
	CacheTTL          float64 `json:"cache_ttl"`           // Cache TTL in seconds: default 300s (5m)
	RequestInterval   float64 `json:"request_interval"`    // Mean request interval in seconds: default 30s
	StickyRoutingProb float64 `json:"sticky_routing_prob"` // Probability of hitting same provider: default 1.0

	// SLA & Eligibility Constraints
	MinUptime      float64 `json:"min_uptime"`      // Minimum required uptime % (e.g. 90.0)
	MinTPS         float64 `json:"min_tps"`         // Minimum tokens/sec (0 = no requirement)
	MaxTTFT        float64 `json:"max_ttft"`        // Max TTFT in ms (0 = no requirement)
	IncludeOffline bool    `json:"include_offline"` // Include offline or status != 0 endpoints

	// Pareto Dominance Filtering
	EnablePareto  bool    `json:"enable_pareto"`  // Enable e-Pareto dominance filtering
	ParetoEpsilon float64 `json:"pareto_epsilon"` // Tolerance slack (default 0.02 = 2%)

	TopN int `json:"top_n"` // Default: 3
}

// DefaultConfig provides recommended production settings under the 'balanced' profile
func DefaultConfig() ScoringConfig {
	cfg := ScoringConfig{
		Profile:           ProfileBalanced,
		PromptRatio:       0.75,
		CompletionRatio:   0.25,
		CacheHitRate:      0.80,
		CacheWriteRatio:   0.10,
		CacheTTL:          300.0,
		RequestInterval:   30.0,
		StickyRoutingProb: 1.0,
		MinUptime:         90.0,
		MinTPS:            0.0,
		MaxTTFT:           0.0,
		IncludeOffline:    false,
		EnablePareto:      false,
		ParetoEpsilon:     0.02,
		TopN:              3,
	}
	cfg.ApplyProfile(ProfileBalanced)
	return cfg
}

// ApplyProfile configures the pre-calibrated weight profiles
func (c *ScoringConfig) ApplyProfile(profileName string) {
	c.Profile = strings.ToLower(strings.TrimSpace(profileName))
	switch c.Profile {
	case ProfileCoding:
		c.PriceWeight = 0.20
		c.QuantWeight = 0.45
		c.RelWeight = 0.20
		c.TTFTWeight = 0.10
		c.TPSWeight = 0.05
		c.LatencyWeight = 0.00
	case ProfileLowLatency:
		c.PriceWeight = 0.20
		c.QuantWeight = 0.15
		c.RelWeight = 0.15
		c.TTFTWeight = 0.35
		c.TPSWeight = 0.15
		c.LatencyWeight = 0.00
	case ProfileBatchThroughput:
		c.PriceWeight = 0.55
		c.QuantWeight = 0.15
		c.RelWeight = 0.10
		c.TTFTWeight = 0.00
		c.TPSWeight = 0.20
		c.LatencyWeight = 0.00
	case ProfileBalanced:
		fallthrough
	default:
		c.Profile = ProfileBalanced
		c.PriceWeight = 0.40
		c.QuantWeight = 0.25
		c.RelWeight = 0.15
		c.TTFTWeight = 0.10
		c.TPSWeight = 0.10
		c.LatencyWeight = 0.00
	}
}

// ScoredProvider represents an evaluated endpoint with full explainability diagnostics
type ScoredProvider struct {
	Rank         int    `json:"rank"`
	ProviderName string `json:"provider_name"`
	Tag          string `json:"tag"`
	EndpointName string `json:"endpoint_name"`

	// Eligibility & Pareto status
	Eligible             bool     `json:"eligible"`
	IneligibilityReasons []string `json:"ineligibility_reasons,omitempty"`
	ParetoDominated      bool     `json:"pareto_dominated"`
	DominatedBy          []string `json:"dominated_by,omitempty"`

	// Quantization & Verified Provider
	Quantization         string  `json:"quantization"`
	QuantScore           float64 `json:"quant_score"`
	IsVerifiedFirstParty bool    `json:"is_verified_first_party"`

	// Pricing Metrics ($ per 1M tokens)
	PromptPricePerM          float64 `json:"prompt_price_per_m"`
	CacheReadPricePerM       float64 `json:"cache_read_price_per_m"`
	CacheWritePricePerM      float64 `json:"cache_write_price_per_m"`
	EffectivePromptPricePerM float64 `json:"effective_prompt_price_per_m"`
	CompletionPricePerM      float64 `json:"completion_price_per_m"`
	BlendedCostPerM          float64 `json:"blended_cost_per_m"`
	EffectiveCacheHitProb    float64 `json:"effective_cache_hit_prob"`
	IsFreeTier               bool    `json:"is_free_tier"`

	// Telemetry Metrics
	Uptime30m           float64 `json:"uptime_30m"`
	Uptime1d            float64 `json:"uptime_1d"`
	BlendedUptime       float64 `json:"blended_uptime"`
	Latency30m          float64 `json:"latency_30m"`    // TTFT / Latency in seconds (0 = not reported)
	Throughput30m       float64 `json:"throughput_30m"` // tokens/sec (0 = not reported)
	ContextLength       int64   `json:"context_length"`
	MaxCompletionTokens int64   `json:"max_completion_tokens"`
	Status              int     `json:"status"`
	SupportsCaching     bool    `json:"supports_caching"`

	// Individual Dimension Scores [0.0, 1.0]
	CostScore       float64 `json:"cost_score"`
	RelScore        float64 `json:"rel_score"`
	TTFTScore       float64 `json:"ttft_score"`
	LatencyScore    float64 `json:"latency_score"`
	TPSScore        float64 `json:"tps_score"`
	CompositeScore  float64 `json:"composite_score"`

	RawEndpoint api.Endpoint `json:"-"`
}

// ModelEvaluationResult contains evaluation results, diagnostics, and cohort metadata
type ModelEvaluationResult struct {
	ModelID                         string           `json:"model_id"`
	ModelName                       string           `json:"model_name"`
	TotalEndpoints                  int              `json:"total_endpoints"`
	ActiveEndpoints                 int              `json:"active_endpoints"`
	EligibleEndpoints               int              `json:"eligible_endpoints"`
	CostCompetitivenessObservable   bool             `json:"cost_competitiveness_observable"`
	HasFreeProviders                bool             `json:"has_free_providers"`
	MinBlendedCostPerM              float64          `json:"min_blended_cost_per_m"`
	MaxBlendedCostPerM              float64          `json:"max_blended_cost_per_m"`
	MinPaidBlendedCostPerM          float64          `json:"min_paid_blended_cost_per_m"`
	ParetoFilteredCount             int              `json:"pareto_filtered_count"`
	TopProviders                    []ScoredProvider `json:"top_providers"`
	AllProviders                    []ScoredProvider `json:"all_providers,omitempty"`
}
