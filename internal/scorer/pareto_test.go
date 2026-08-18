package scorer

import (
	"testing"
)

func TestParetoDominance(t *testing.T) {
	// Provider B strictly dominates Provider A across cost, speed, reliability
	providerA := ScoredProvider{
		ProviderName: "Provider A (Inferior)",
		CostScore:    0.50,
		QuantScore:   0.60,
		RelScore:     0.70,
		TTFTScore:    0.50,
		TPSScore:     0.50,
	}

	providerB := ScoredProvider{
		ProviderName: "Provider B (Dominant)",
		CostScore:    0.80, // strictly better
		QuantScore:   0.60, // equal
		RelScore:     0.90, // strictly better
		TTFTScore:    0.80, // strictly better
		TPSScore:     0.70, // strictly better
	}

	candidates := []ScoredProvider{providerA, providerB}
	filtered, count := ApplyParetoFilter(candidates, 0.02)

	if count != 1 {
		t.Fatalf("Expected 1 provider to be filtered out, got %d", count)
	}
	if len(filtered) != 1 {
		t.Fatalf("Expected 1 provider remaining, got %d", len(filtered))
	}
	if filtered[0].ProviderName != "Provider B (Dominant)" {
		t.Errorf("Expected Provider B to survive, got %s", filtered[0].ProviderName)
	}
}

func TestParetoTradeoffRetention(t *testing.T) {
	// Provider A is cheaper; Provider B is faster. Neither dominates.
	providerA := ScoredProvider{
		ProviderName: "Provider A (Cheaper)",
		CostScore:    0.90, // cheaper
		QuantScore:   0.60,
		RelScore:     0.80,
		TTFTScore:    0.40, // slower
		TPSScore:     0.40,
	}

	providerB := ScoredProvider{
		ProviderName: "Provider B (Faster)",
		CostScore:    0.50, // more expensive
		QuantScore:   0.60,
		RelScore:     0.80,
		TTFTScore:    0.90, // faster
		TPSScore:     0.90,
	}

	candidates := []ScoredProvider{providerA, providerB}
	filtered, count := ApplyParetoFilter(candidates, 0.02)

	if count != 0 {
		t.Errorf("Expected 0 filtered providers in tradeoff scenario, got %d", count)
	}
	if len(filtered) != 2 {
		t.Errorf("Expected both providers retained, got %d", len(filtered))
	}
}

func TestParetoIdenticalRetention(t *testing.T) {
	providerA := ScoredProvider{
		ProviderName: "Provider A",
		CostScore:    0.70,
		QuantScore:   0.60,
		RelScore:     0.80,
		TTFTScore:    0.60,
		TPSScore:     0.60,
	}

	providerB := ScoredProvider{
		ProviderName: "Provider B",
		CostScore:    0.70,
		QuantScore:   0.60,
		RelScore:     0.80,
		TTFTScore:    0.60,
		TPSScore:     0.60,
	}

	candidates := []ScoredProvider{providerA, providerB}
	filtered, count := ApplyParetoFilter(candidates, 0.02)

	if count != 0 || len(filtered) != 2 {
		t.Errorf("Expected both identical providers retained, got %d remaining, %d filtered", len(filtered), count)
	}
}
