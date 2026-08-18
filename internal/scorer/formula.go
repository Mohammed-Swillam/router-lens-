package scorer

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"routerlens/internal/api"
)

var (
	quantBitRegex = regexp.MustCompile(`(?i)\b(?:(?:fp|int|q|bf)(\d+)|(\d+)[-_ ]?bit)\b`)

	// Verified first-party cloud providers that host unquantized / full precision weights by default
	firstPartyProviders = []string{
		"openai",
		"azure",
		"amazon bedrock",
		"amazon-bedrock",
		"bedrock",
		"google",
		"google vertex",
		"vertex",
		"anthropic",
	}
)

// IsVerifiedFirstParty checks if the provider name belongs to a verified first-party registry
func IsVerifiedFirstParty(providerName string) bool {
	p := strings.ToLower(strings.TrimSpace(providerName))
	for _, fp := range firstPartyProviders {
		if strings.Contains(p, fp) {
			return true
		}
	}
	return false
}

// EvaluateQuantization assigns a quality score (0.0 to 1.0) with verified first-party detection
func EvaluateQuantization(quantStr string, tagStr string, providerName string) (string, float64, bool) {
	q := strings.TrimSpace(strings.ToLower(quantStr))
	tag := strings.TrimSpace(strings.ToLower(tagStr))
	isFP := IsVerifiedFirstParty(providerName)

	// If quantization is unknown/empty, check if tag has clues (e.g., 'deepinfra/fp4' or 'novita/fp8')
	if (q == "" || q == "unknown" || q == "null") && tag != "" {
		parts := strings.Split(tag, "/")
		if len(parts) > 1 {
			tagCandidate := parts[len(parts)-1]
			if isLikelyQuantization(tagCandidate) {
				q = tagCandidate
			}
		}
	}

	// Verified First-Party Provider Rule:
	// If provider is first-party (OpenAI, Azure, Bedrock, Google, Anthropic) and has no explicit quantization,
	// it is served in lossless full precision
	if (q == "" || q == "unknown" || q == "null") && isFP {
		return "lossless (first-party)", 1.00, true
	}

	if q == "" || q == "unknown" || q == "null" {
		return "unknown", 0.30, isFP
	}

	// Exact matches for full precision / unquantized
	switch q {
	case "none", "full", "fp16", "bf16", "f16", "16-bit", "fp32", "f32", "32-bit":
		return q, 1.00, isFP
	case "fp8", "int8", "8-bit", "q8", "q8_0", "q8_k", "fp8_e4m3", "fp8_e5m2":
		return q, 0.60, isFP
	case "fp4", "int4", "4-bit", "awq", "gptq", "q4", "q4_k_m", "q4_0", "q4_1", "exl2":
		return q, 0.20, isFP
	case "fp6", "int6", "6-bit", "q6", "q6_k":
		return q, 0.40, isFP
	case "fp5", "int5", "5-bit", "q5", "q5_k_m", "q5_0":
		return q, 0.30, isFP
	case "fp3", "int3", "3-bit", "q3", "q3_k_m", "q3_0", "fp2", "int2", "2-bit", "q2":
		return q, 0.10, isFP
	}

	// Dynamic regex detection for bit depth (e.g. 'int4_k', 'fp8_e4m3', '8-bit')
	matches := quantBitRegex.FindStringSubmatch(q)
	if len(matches) > 0 {
		bitStr := matches[1]
		if bitStr == "" && len(matches) > 2 {
			bitStr = matches[2]
		}
		if bitStr != "" {
			bits, err := strconv.Atoi(bitStr)
			if err == nil {
				switch {
				case bits >= 16:
					return q, 1.00, isFP
				case bits >= 8:
					return q, 0.60, isFP
				case bits >= 6:
					return q, 0.40, isFP
				case bits >= 5:
					return q, 0.30, isFP
				case bits >= 4:
					return q, 0.20, isFP
				default:
					return q, 0.10, isFP
				}
			}
		}
	}

	// Fallback for AWQ/GPTQ keywords in string
	if strings.Contains(q, "awq") || strings.Contains(q, "gptq") {
		return q, 0.20, isFP
	}

	return q, 0.30, isFP
}

func isLikelyQuantization(s string) bool {
	s = strings.ToLower(s)
	if quantBitRegex.MatchString(s) {
		return true
	}
	switch s {
	case "none", "full", "fp16", "bf16", "f16", "fp8", "int8", "fp4", "int4", "awq", "gptq", "exl2":
		return true
	}
	return false
}

// EvaluateReliability computes a Bayesian-smoothed multi-window reliability score [0.0, 1.0]
func EvaluateReliability(ep *api.Endpoint) (float64, float64, float64, float64) {
	u30m := 100.0
	if ep.UptimeLast30m != nil {
		u30m = ep.UptimeLast30m.Float64()
	}

	u1d := u30m
	if ep.UptimeLast1d != nil {
		u1d = ep.UptimeLast1d.Float64()
	}

	// Multi-window blending: 70% recent (30m) + 30% historical (1d)
	blendedUptime := (0.70 * u30m) + (0.30 * u1d)

	// Bayesian Smoothing Prior:
	// Prior expects 99.5% uptime with weight equivalent to 10 observation units
	priorMean := 99.5
	priorWeight := 10.0
	observedWeight := 90.0
	smoothedUptime := (blendedUptime*observedWeight + priorMean*priorWeight) / (observedWeight + priorWeight)

	// Non-linear utility curve tailored for high-availability systems:
	var score float64
	switch {
	case smoothedUptime >= 99.9:
		score = 0.95 + ((smoothedUptime - 99.9) / 0.10) * 0.05
	case smoothedUptime >= 98.0:
		score = 0.60 + ((smoothedUptime - 98.0) / 1.90) * 0.35
	case smoothedUptime >= 90.0:
		score = 0.20 + ((smoothedUptime - 90.0) / 8.00) * 0.40
	default:
		score = (smoothedUptime / 90.0) * 0.20
	}

	if score > 1.0 {
		score = 1.0
	}
	if score < 0.0 {
		score = 0.0
	}

	return u30m, u1d, blendedUptime, score
}

// EvaluateTTFT computes a monotonic utility score for Time To First Token latency [0.0, 1.0]
func EvaluateTTFT(latencySec float64) float64 {
	if latencySec <= 0.0 {
		// Missing/unreported telemetry: impute neutral baseline (0.60)
		return 0.60
	}

	// Target TTFT = 0.50 seconds (500ms)
	targetSec := 0.50
	if latencySec <= targetSec {
		// Excellent latency: scales smoothly from 1.0 (0s) to 0.85 (0.5s)
		return 1.0 - (0.15 * (latencySec / targetSec))
	}

	// Latency beyond target decays monotonically
	excess := latencySec - targetSec
	score := 0.85 / (1.0 + (1.5 * excess))
	if score < 0.0 {
		return 0.0
	}
	return score
}

// EvaluateThroughput computes a monotonic utility score for tokens/second [0.0, 1.0]
func EvaluateThroughput(tps float64) float64 {
	if tps <= 0.0 {
		// Missing/unreported telemetry: impute neutral baseline (0.60)
		return 0.60
	}

	// Target TPS = 50 tok/s, High Benchmark = 150 tok/s
	targetTPS := 50.0
	highTPS := 150.0

	// Monotonic saturation utility: TPS / (TPS + target) normalized against high benchmark
	factor := (highTPS + targetTPS) / highTPS
	score := (tps / (tps + targetTPS)) * factor

	if score > 1.0 {
		score = 1.0
	}
	if score < 0.0 {
		score = 0.0
	}
	return score
}

// EvaluateModel evaluates, filters, scores, and ranks all available providers for a model
func EvaluateModel(resp *api.EndpointsResponse, cfg ScoringConfig) *ModelEvaluationResult {
	if resp == nil {
		return nil
	}

	result := &ModelEvaluationResult{
		ModelID:        resp.Data.ID,
		ModelName:      resp.Data.Name,
		TotalEndpoints: len(resp.Data.Endpoints),
	}

	if result.ModelName == "" {
		result.ModelName = resp.Data.ID
	}

	var candidates []ScoredProvider

	// 1. Data Validation, SLA Eligibility Filtering, and Feature Extraction
	for _, ep := range resp.Data.Endpoints {
		var ineligibilityReasons []string

		// Status Check
		if !cfg.IncludeOffline && ep.Status != 0 {
			ineligibilityReasons = append(ineligibilityReasons, "provider status != 0 (offline or degraded)")
		}

		// Reliability Telemetry & Score
		u30m, u1d, blendedUptime, relScore := EvaluateReliability(&ep)
		if !cfg.IncludeOffline && ep.UptimeLast30m != nil && u30m <= 0.0 {
			ineligibilityReasons = append(ineligibilityReasons, "30m uptime is 0.0%")
		}
		if cfg.MinUptime > 0.0 && blendedUptime < cfg.MinUptime {
			ineligibilityReasons = append(ineligibilityReasons, "blended uptime below SLA threshold")
		}

		// Performance Telemetry
		latSec := 0.0
		if ep.LatencyLast30m != nil {
			latSec = ep.LatencyLast30m.Float64()
		}
		if cfg.MaxTTFT > 0.0 && latSec > 0.0 && (latSec*1000.0) > cfg.MaxTTFT {
			ineligibilityReasons = append(ineligibilityReasons, "TTFT exceeds maximum threshold")
		}

		tps := 0.0
		if ep.ThroughputLast30m != nil {
			tps = ep.ThroughputLast30m.Float64()
		}
		if cfg.MinTPS > 0.0 && tps > 0.0 && tps < cfg.MinTPS {
			ineligibilityReasons = append(ineligibilityReasons, "throughput below minimum threshold")
		}

		ttftScore := EvaluateTTFT(latSec)
		tpsScore := EvaluateThroughput(tps)

		// Pricing Extraction ($ per 1M tokens)
		promptPerM := ep.Pricing.Prompt.Float64() * 1_000_000
		compPerM := ep.Pricing.Completion.Float64() * 1_000_000
		rawCacheReadPerM := ep.Pricing.InputCacheRead.Float64() * 1_000_000
		rawCacheWritePerM := ep.Pricing.InputCacheWrite.Float64() * 1_000_000

		// Fallback: If cache read price is not provided or 0 but prompt price is non-zero,
		// standard prompt price applies on cache hits
		cacheReadPerM := rawCacheReadPerM
		if cacheReadPerM <= 0 && promptPerM > 0 {
			cacheReadPerM = promptPerM
		}

		cacheWritePerM := rawCacheWritePerM
		if cacheWritePerM <= 0 && promptPerM > 0 {
			cacheWritePerM = promptPerM
		}

		// Cache Lifecycle Economics:
		// P(survives) = e^(-dt / TTL)
		pSurvives := 1.0
		if cfg.CacheTTL > 0 && cfg.RequestInterval >= 0 {
			pSurvives = math.Exp(-cfg.RequestInterval / cfg.CacheTTL)
		}
		pSticky := cfg.StickyRoutingProb
		if pSticky < 0.0 {
			pSticky = 0.0
		}
		if pSticky > 1.0 {
			pSticky = 1.0
		}

		effCacheHitProb := pSurvives * pSticky * cfg.CacheHitRate
		effPromptPerM := (effCacheHitProb * cacheReadPerM) + ((1.0 - effCacheHitProb) * promptPerM) + (cfg.CacheWriteRatio * cacheWritePerM)
		blendedPerM := (cfg.PromptRatio * effPromptPerM) + (cfg.CompletionRatio * compPerM)

		// Quantization & Verified Provider Evaluation
		pName := ep.ProviderName
		if pName == "" {
			pName = ep.Name
		}
		quantName, quantScore, isFP := EvaluateQuantization(ep.Quantization, ep.Tag, pName)

		isFree := (promptPerM <= 0 && compPerM <= 0)

		isEligible := len(ineligibilityReasons) == 0

		sp := ScoredProvider{
			ProviderName:             pName,
			Tag:                      ep.Tag,
			EndpointName:             ep.Name,
			Eligible:                 isEligible,
			IneligibilityReasons:     ineligibilityReasons,
			Quantization:             quantName,
			QuantScore:               quantScore,
			IsVerifiedFirstParty:     isFP,
			PromptPricePerM:          promptPerM,
			CacheReadPricePerM:       cacheReadPerM,
			CacheWritePricePerM:      cacheWritePerM,
			EffectivePromptPricePerM: effPromptPerM,
			CompletionPricePerM:      compPerM,
			BlendedCostPerM:          blendedPerM,
			EffectiveCacheHitProb:    effCacheHitProb,
			IsFreeTier:               isFree,
			Uptime30m:                u30m,
			Uptime1d:                 u1d,
			BlendedUptime:            blendedUptime,
			Latency30m:               latSec,
			Throughput30m:            tps,
			ContextLength:            ep.ContextLength,
			MaxCompletionTokens:      ep.MaxCompletionTokens,
			Status:                   ep.Status,
			SupportsCaching:          ep.SupportsImplicitCaching,
			RelScore:                 relScore,
			TTFTScore:                ttftScore,
			TPSScore:                 tpsScore,
			RawEndpoint:              ep,
		}

		candidates = append(candidates, sp)
	}

	result.ActiveEndpoints = len(candidates)

	// Filter down to eligible candidates
	var eligible []ScoredProvider
	for _, c := range candidates {
		if c.Eligible {
			eligible = append(eligible, c)
		}
	}
	result.EligibleEndpoints = len(eligible)

	if len(eligible) == 0 {
		result.AllProviders = candidates
		return result
	}

	// 2. Zero-Price & Single-Provider Cost Normalization
	var paidCosts []float64
	hasFree := false
	for _, c := range eligible {
		if c.IsFreeTier {
			hasFree = true
		} else {
			paidCosts = append(paidCosts, c.BlendedCostPerM)
		}
	}

	result.HasFreeProviders = hasFree
	result.CostCompetitivenessObservable = (len(paidCosts) >= 2)

	minPaidCost := 0.0
	maxCost := 0.0
	if len(paidCosts) > 0 {
		minPaidCost = paidCosts[0]
		maxCost = paidCosts[0]
		for _, cost := range paidCosts {
			if cost < minPaidCost {
				minPaidCost = cost
			}
			if cost > maxCost {
				maxCost = cost
			}
		}
	}

	result.MinBlendedCostPerM = 0.0
	if len(paidCosts) > 0 && !hasFree {
		result.MinBlendedCostPerM = minPaidCost
	}
	result.MinPaidBlendedCostPerM = minPaidCost
	result.MaxBlendedCostPerM = maxCost

	// Calculate Cost Score (S_cost) for each eligible candidate
	for i := range eligible {
		if eligible[i].IsFreeTier {
			// Free provider receives maximum cost score
			eligible[i].CostScore = 1.00
		} else {
			if minPaidCost > 0 {
				baseScore := minPaidCost / eligible[i].BlendedCostPerM
				if hasFree {
					// If both free and paid providers coexist, paid providers score proportionally up to 0.90
					eligible[i].CostScore = baseScore * 0.90
				} else {
					eligible[i].CostScore = baseScore
				}
			} else {
				eligible[i].CostScore = 1.00
			}
		}

		if eligible[i].CostScore > 1.0 {
			eligible[i].CostScore = 1.0
		}
		if eligible[i].CostScore < 0.0 {
			eligible[i].CostScore = 0.0
		}
	}

	// 3. Optional e-Pareto Dominance Filtering
	if cfg.EnablePareto && len(eligible) > 1 {
		eligible, result.ParetoFilteredCount = ApplyParetoFilter(eligible, cfg.ParetoEpsilon)
	}

	// 4. Normalized Weights and Composite Score
	totalWeight := cfg.PriceWeight + cfg.QuantWeight + cfg.RelWeight + cfg.TTFTWeight + cfg.LatencyWeight + cfg.TPSWeight
	if totalWeight <= 0 {
		totalWeight = 1.0
	}
	wCost := cfg.PriceWeight / totalWeight
	wQuant := cfg.QuantWeight / totalWeight
	wRel := cfg.RelWeight / totalWeight
	wTTFT := cfg.TTFTWeight / totalWeight
	wLat := cfg.LatencyWeight / totalWeight
	wTPS := cfg.TPSWeight / totalWeight

	for i := range eligible {
		compScore := (wCost * eligible[i].CostScore) +
			(wQuant * eligible[i].QuantScore) +
			(wRel * eligible[i].RelScore) +
			(wTTFT * eligible[i].TTFTScore) +
			(wLat * eligible[i].LatencyScore) +
			(wTPS * eligible[i].TPSScore)

		if compScore > 1.0 {
			compScore = 1.0
		}
		if compScore < 0.0 {
			compScore = 0.0
		}
		eligible[i].CompositeScore = compScore
	}

	// 5. Deterministic Multi-Stage Tie-Breaking
	const epsilon = 1e-6
	sort.Slice(eligible, func(i, j int) bool {
		// 1. Composite Score (desc)
		if math.Abs(eligible[i].CompositeScore-eligible[j].CompositeScore) > epsilon {
			return eligible[i].CompositeScore > eligible[j].CompositeScore
		}
		// 2. Reliability Score (desc)
		if math.Abs(eligible[i].RelScore-eligible[j].RelScore) > epsilon {
			return eligible[i].RelScore > eligible[j].RelScore
		}
		// 3. TTFT Score (desc)
		if math.Abs(eligible[i].TTFTScore-eligible[j].TTFTScore) > epsilon {
			return eligible[i].TTFTScore > eligible[j].TTFTScore
		}
		// 4. Throughput Score (desc)
		if math.Abs(eligible[i].TPSScore-eligible[j].TPSScore) > epsilon {
			return eligible[i].TPSScore > eligible[j].TPSScore
		}
		// 5. Context Length (desc)
		if eligible[i].ContextLength != eligible[j].ContextLength {
			return eligible[i].ContextLength > eligible[j].ContextLength
		}
		// 6. Prompt Price (asc)
		if math.Abs(eligible[i].PromptPricePerM-eligible[j].PromptPricePerM) > epsilon {
			return eligible[i].PromptPricePerM < eligible[j].PromptPricePerM
		}
		// 7. Lexical Provider Name & Tag for 100% deterministic stability
		if eligible[i].ProviderName != eligible[j].ProviderName {
			return eligible[i].ProviderName < eligible[j].ProviderName
		}
		return eligible[i].Tag < eligible[j].Tag
	})

	// Assign Ranks
	for i := range eligible {
		eligible[i].Rank = i + 1
	}

	result.AllProviders = eligible

	// Top N
	topLimit := cfg.TopN
	if topLimit > len(eligible) {
		topLimit = len(eligible)
	}
	result.TopProviders = eligible[:topLimit]

	return result
}
