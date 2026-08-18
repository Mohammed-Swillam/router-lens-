package scorer

import (
	"math"
	"testing"

	"routerlens/internal/api"
)

func TestEvaluateQuantization(t *testing.T) {
	tests := []struct {
		quant        string
		tag          string
		provider     string
		wantScore    float64
		wantLossless bool
	}{
		{"fp16", "", "Generic Provider", 1.00, false},
		{"bf16", "", "Generic Provider", 1.00, false},
		{"none", "", "Generic Provider", 1.00, false},
		{"16-bit", "", "Generic Provider", 1.00, false},
		{"fp8", "", "Generic Provider", 0.60, false},
		{"int8", "", "Generic Provider", 0.60, false},
		{"8-bit", "", "Generic Provider", 0.60, false},
		{"fp4", "", "Generic Provider", 0.20, false},
		{"int4", "", "Generic Provider", 0.20, false},
		{"awq", "", "Generic Provider", 0.20, false},
		{"gptq", "", "Generic Provider", 0.20, false},
		{"unknown", "deepinfra/fp4", "Generic Provider", 0.20, false},
		{"unknown", "novita/fp8", "Generic Provider", 0.60, false},
		{"unknown", "amazon-bedrock/us-east-1", "Amazon Bedrock", 1.00, true}, // Verified first-party rule
		{"", "", "OpenAI", 1.00, true},                                       // Verified first-party rule
		{"unknown", "", "Azure", 1.00, true},                                  // Verified first-party rule
		{"unknown", "", "Google Vertex", 1.00, true},                          // Verified first-party rule
		{"unknown", "", "Anthropic", 1.00, true},                              // Verified first-party rule
		{"unknown", "", "Generic Provider", 0.30, false},
		{"", "", "Generic Provider", 0.30, false},
	}

	for _, tt := range tests {
		_, score, isFP := EvaluateQuantization(tt.quant, tt.tag, tt.provider)
		if math.Abs(score-tt.wantScore) > 0.001 {
			t.Errorf("EvaluateQuantization(%q, %q, %q) = %v, want %v", tt.quant, tt.tag, tt.provider, score, tt.wantScore)
		}
		if isFP != tt.wantLossless {
			t.Errorf("EvaluateQuantization(%q, %q, %q) isFP = %v, want %v", tt.quant, tt.tag, tt.provider, isFP, tt.wantLossless)
		}
	}
}

func TestReliabilityScoringAndBayesianPrior(t *testing.T) {
	u100 := api.FlexibleFloat(100.0)
	u95 := api.FlexibleFloat(95.0)
	u80 := api.FlexibleFloat(80.0)

	// Case 1: 100% uptime recent and historical
	epHigh := api.Endpoint{
		UptimeLast30m: &u100,
		UptimeLast1d:  &u100,
	}
	_, _, _, scoreHigh := EvaluateReliability(&epHigh)
	if scoreHigh < 0.95 {
		t.Errorf("Expected high reliability score >= 0.95, got %f", scoreHigh)
	}

	// Case 2: Degraded 95% uptime
	epMed := api.Endpoint{
		UptimeLast30m: &u95,
		UptimeLast1d:  &u95,
	}
	_, _, _, scoreMed := EvaluateReliability(&epMed)
	if scoreMed >= scoreHigh || scoreMed < 0.40 {
		t.Errorf("Expected medium reliability score, got %f", scoreMed)
	}

	// Case 3: Poor 80% uptime
	epLow := api.Endpoint{
		UptimeLast30m: &u80,
		UptimeLast1d:  &u80,
	}
	_, _, _, scoreLow := EvaluateReliability(&epLow)
	if scoreLow >= scoreMed || scoreLow > 0.30 {
		t.Errorf("Expected low reliability score <= 0.30, got %f", scoreLow)
	}

	// Monotonicity verification
	if !(scoreHigh > scoreMed && scoreMed > scoreLow) {
		t.Errorf("Reliability monotonicity failed: High(%f) > Med(%f) > Low(%f)", scoreHigh, scoreMed, scoreLow)
	}
}

func TestTTFTAndThroughputUtilities(t *testing.T) {
	// TTFT Monotonicity
	tFast := EvaluateTTFT(0.10)   // 100ms
	tTarget := EvaluateTTFT(0.50) // 500ms
	tSlow := EvaluateTTFT(2.00)   // 2.0s

	if !(tFast > tTarget && tTarget > tSlow) {
		t.Errorf("TTFT monotonicity failed: Fast(%f) > Target(%f) > Slow(%f)", tFast, tTarget, tSlow)
	}

	// TPS Monotonicity
	tpsHigh := EvaluateThroughput(150.0)
	tpsMed := EvaluateThroughput(50.0)
	tpsLow := EvaluateThroughput(10.0)

	if !(tpsHigh > tpsMed && tpsMed > tpsLow) {
		t.Errorf("TPS monotonicity failed: High(%f) > Med(%f) > Low(%f)", tpsHigh, tpsMed, tpsLow)
	}

	// Range Invariants
	if tFast < 0.0 || tFast > 1.0 || tSlow < 0.0 || tSlow > 1.0 {
		t.Errorf("TTFT scores out of [0, 1] range: fast=%f, slow=%f", tFast, tSlow)
	}
	if tpsHigh < 0.0 || tpsHigh > 1.0 || tpsLow < 0.0 || tpsLow > 1.0 {
		t.Errorf("TPS scores out of [0, 1] range: high=%f, low=%f", tpsHigh, tpsLow)
	}
}

func TestCacheLifecycleAndStickyRouting(t *testing.T) {
	u100 := api.FlexibleFloat(100.0)

	resp := &api.EndpointsResponse{
		Data: struct {
			ID        string         `json:"id"`
			Name      string         `json:"name"`
			Endpoints []api.Endpoint `json:"endpoints"`
		}{
			ID:   "test/cache-lifecycle",
			Name: "Test Cache Lifecycle",
			Endpoints: []api.Endpoint{
				{
					Name:            "Provider A",
					ProviderName:    "Provider A",
					Quantization:    "fp16",
					Pricing:         api.Pricing{Prompt: 0.000001, Completion: 0.000002, InputCacheRead: 0.0000001, InputCacheWrite: 0.0000012},
					Status:          0,
					UptimeLast30m:   &u100,
					SupportsImplicitCaching: true,
				},
			},
		},
	}

	// Scenario 1: Fast requests, within TTL, sticky routing = 1.0 (High cache survival)
	cfgSticky := DefaultConfig()
	cfgSticky.CacheTTL = 300.0
	cfgSticky.RequestInterval = 10.0 // 10s << 300s
	cfgSticky.StickyRoutingProb = 1.0
	res1 := EvaluateModel(resp, cfgSticky)

	// Scenario 2: Requests after TTL expired (Low cache survival)
	cfgExpired := DefaultConfig()
	cfgExpired.CacheTTL = 10.0
	cfgExpired.RequestInterval = 300.0 // 300s >> 10s
	cfgExpired.StickyRoutingProb = 1.0
	res2 := EvaluateModel(resp, cfgExpired)

	if res1.TopProviders[0].BlendedCostPerM >= res2.TopProviders[0].BlendedCostPerM {
		t.Errorf("Expected warm cache blended cost (%f) < expired cache blended cost (%f)",
			res1.TopProviders[0].BlendedCostPerM, res2.TopProviders[0].BlendedCostPerM)
	}
}

func TestZeroPriceAndFreeTierCohorts(t *testing.T) {
	u100 := api.FlexibleFloat(100.0)

	resp := &api.EndpointsResponse{
		Data: struct {
			ID        string         `json:"id"`
			Name      string         `json:"name"`
			Endpoints []api.Endpoint `json:"endpoints"`
		}{
			ID:   "test/mixed-pricing",
			Name: "Mixed Free and Paid",
			Endpoints: []api.Endpoint{
				{
					Name:          "Free Provider",
					ProviderName:  "Free Provider",
					Quantization:  "fp16",
					Pricing:       api.Pricing{Prompt: 0, Completion: 0},
					Status:        0,
					UptimeLast30m: &u100,
				},
				{
					Name:          "Paid Cheap",
					ProviderName:  "Paid Cheap",
					Quantization:  "fp16",
					Pricing:       api.Pricing{Prompt: 0.000001, Completion: 0.000001}, // $1.00/M
					Status:        0,
					UptimeLast30m: &u100,
				},
				{
					Name:          "Paid Expensive",
					ProviderName:  "Paid Expensive",
					Quantization:  "fp16",
					Pricing:       api.Pricing{Prompt: 0.000002, Completion: 0.000002}, // $2.00/M
					Status:        0,
					UptimeLast30m: &u100,
				},
			},
		},
	}

	result := EvaluateModel(resp, DefaultConfig())

	if !result.HasFreeProviders {
		t.Errorf("Expected HasFreeProviders = true")
	}

	// Free provider must get CostScore = 1.00
	if math.Abs(result.AllProviders[0].CostScore-1.00) > 0.001 {
		t.Errorf("Expected Free Provider cost score = 1.00, got %f", result.AllProviders[0].CostScore)
	}

	// Paid Cheap provider should get 0.90 (differentiated from Free, but top of paid tier)
	// Paid Expensive provider should get 0.45 (exactly half of Paid Cheap!)
	var paidCheap, paidExp *ScoredProvider
	for i := range result.AllProviders {
		if result.AllProviders[i].ProviderName == "Paid Cheap" {
			paidCheap = &result.AllProviders[i]
		}
		if result.AllProviders[i].ProviderName == "Paid Expensive" {
			paidExp = &result.AllProviders[i]
		}
	}

	if paidCheap == nil || paidExp == nil {
		t.Fatalf("Failed to locate paid providers in evaluation results")
	}

	if math.Abs(paidCheap.CostScore-0.90) > 0.05 {
		t.Errorf("Expected Paid Cheap cost score ~0.90, got %f", paidCheap.CostScore)
	}
	if math.Abs(paidExp.CostScore-0.45) > 0.05 {
		t.Errorf("Expected Paid Expensive cost score ~0.45, got %f", paidExp.CostScore)
	}
}

func TestSingleProviderCompetitivenessObservable(t *testing.T) {
	u100 := api.FlexibleFloat(100.0)

	resp := &api.EndpointsResponse{
		Data: struct {
			ID        string         `json:"id"`
			Name      string         `json:"name"`
			Endpoints []api.Endpoint `json:"endpoints"`
		}{
			ID:   "single/provider",
			Name: "Single Provider Model",
			Endpoints: []api.Endpoint{
				{
					Name:          "Solo Provider",
					ProviderName:  "Solo Provider",
					Quantization:  "fp16",
					Pricing:       api.Pricing{Prompt: 0.000005, Completion: 0.000010},
					Status:        0,
					UptimeLast30m: &u100,
				},
			},
		},
	}

	result := EvaluateModel(resp, DefaultConfig())
	if result.CostCompetitivenessObservable {
		t.Errorf("Expected CostCompetitivenessObservable = false for single provider cohort")
	}
}

func TestWorkloadProfilesApplication(t *testing.T) {
	cfg := DefaultConfig()

	cfg.ApplyProfile(ProfileCoding)
	if cfg.QuantWeight != 0.45 || cfg.PriceWeight != 0.20 {
		t.Errorf("Coding profile weights mismatch: quant=%f, price=%f", cfg.QuantWeight, cfg.PriceWeight)
	}

	cfg.ApplyProfile(ProfileLowLatency)
	if cfg.TTFTWeight != 0.35 || cfg.PriceWeight != 0.20 {
		t.Errorf("LowLatency profile weights mismatch: ttft=%f, price=%f", cfg.TTFTWeight, cfg.PriceWeight)
	}

	cfg.ApplyProfile(ProfileBatchThroughput)
	if cfg.PriceWeight != 0.55 || cfg.TPSWeight != 0.20 {
		t.Errorf("BatchThroughput profile weights mismatch: price=%f, tps=%f", cfg.PriceWeight, cfg.TPSWeight)
	}
}
