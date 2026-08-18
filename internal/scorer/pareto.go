package scorer

// ApplyParetoFilter removes e-dominated providers across 5 dimensions:
// 1. Cost Score (higher is better)
// 2. Quantization Score (higher is better)
// 3. Reliability Score (higher is better)
// 4. TTFT Score (higher is better)
// 5. Throughput Score (higher is better)
//
// Returns the non-dominated subset and the count of filtered providers.
// Guarantees that at least one provider is always preserved.
func ApplyParetoFilter(candidates []ScoredProvider, epsilon float64) ([]ScoredProvider, int) {
	if len(candidates) <= 1 {
		return candidates, 0
	}
	if epsilon < 0.0 {
		epsilon = 0.02
	}

	dominated := make([]bool, len(candidates))
	dominatedBy := make([][]string, len(candidates))
	filteredCount := 0

	for i := 0; i < len(candidates); i++ {
		for j := 0; j < len(candidates); j++ {
			if i == j {
				continue
			}

			// Check if candidate j dominates candidate i
			if dominates(candidates[j], candidates[i], epsilon) {
				dominated[i] = true
				dominatedBy[i] = append(dominatedBy[i], candidates[j].ProviderName)
			}
		}
	}

	var nonDominated []ScoredProvider
	for i, c := range candidates {
		c.ParetoDominated = dominated[i]
		c.DominatedBy = dominatedBy[i]
		if !dominated[i] {
			nonDominated = append(nonDominated, c)
		} else {
			filteredCount++
		}
	}

	// Safety Guarantee: Never return an empty candidate slice
	if len(nonDominated) == 0 {
		return candidates, 0
	}

	return nonDominated, filteredCount
}

// dominates checks if provider B dominates provider A with epsilon slack
// Returns true if B is no worse than A across all 5 dimensions (within epsilon)
// AND strictly better in at least one dimension (by at least epsilon).
func dominates(b, a ScoredProvider, eps float64) bool {
	// All metrics: higher score is better
	bMetrics := [5]float64{b.CostScore, b.QuantScore, b.RelScore, b.TTFTScore, b.TPSScore}
	aMetrics := [5]float64{a.CostScore, a.QuantScore, a.RelScore, a.TTFTScore, a.TPSScore}

	strictlyBetterInAtLeastOne := false

	for k := 0; k < 5; k++ {
		// B must be no worse than A - eps
		if bMetrics[k] < (aMetrics[k] - eps) {
			return false
		}
		// Check if B is strictly better by at least eps
		if bMetrics[k] >= (aMetrics[k] + eps) {
			strictlyBetterInAtLeastOne = true
		}
	}

	return strictlyBetterInAtLeastOne
}
