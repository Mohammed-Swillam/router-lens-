package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"routerlens/internal/api"
	"routerlens/internal/scorer"
	"routerlens/internal/ui"
)

func main() {
	// CLI Profile & Mode Flags
	profileFlag := flag.String("profile", "balanced", "Workload weight profile: 'balanced', 'coding', 'low_latency', 'batch_throughput', 'custom'")
	paretoFlag := flag.Bool("pareto", false, "Enable e-Pareto dominance filtering (slack e=0.02)")
	paretoEpsFlag := flag.Float64("pareto-eps", 0.02, "Tolerance slack margin for e-Pareto filtering (default 0.02 = 2%)")

	// Display Layout Flags
	compactFlag := flag.Bool("compact", false, "Force compact table layout (< 85 columns)")
	wideFlag := flag.Bool("wide", false, "Force wide multi-column table layout (>= 125 columns)")
	cardFlag := flag.Bool("card", false, "Force card-based layout for narrow or windowed terminals")

	// Custom Weight Override Flags
	priceWeightFlag := flag.Float64("price-weight", -1.0, "Override weight for cost score (0.0 to 1.0)")
	quantWeightFlag := flag.Float64("quant-weight", -1.0, "Override weight for quantization quality score (0.0 to 1.0)")
	relWeightFlag := flag.Float64("rel-weight", -1.0, "Override weight for reliability score (0.0 to 1.0)")
	ttftWeightFlag := flag.Float64("ttft-weight", -1.0, "Override weight for TTFT latency score (0.0 to 1.0)")
	latWeightFlag := flag.Float64("lat-weight", -1.0, "Override weight for total latency score (0.0 to 1.0)")
	tpsWeightFlag := flag.Float64("tps-weight", -1.0, "Override weight for throughput score (0.0 to 1.0)")

	// Cache Lifecycle & Economic Flags
	promptRatioFlag := flag.Float64("prompt-ratio", 0.75, "Ratio of prompt tokens in blended cost (default 0.75 = 3:1 prompt:completion)")
	compRatioFlag := flag.Float64("comp-ratio", 0.25, "Ratio of completion tokens in blended cost (default 0.25)")
	cacheHitRateFlag := flag.Float64("cache-hit", 0.80, "Estimated prompt cache hit rate (0.0 to 1.0, default 0.80 = 80%)")
	cacheWriteRatioFlag := flag.Float64("cache-write-ratio", 0.10, "Fraction of prompt tokens requiring cache write (default 0.10)")
	cacheTTLFlag := flag.Float64("cache-ttl", 300.0, "Cache TTL in seconds (default 300s = 5m)")
	reqIntervalFlag := flag.Float64("req-interval", 30.0, "Mean request interval in seconds (default 30s)")
	stickyProbFlag := flag.Float64("sticky-prob", 1.0, "Probability of routing to same cache provider (default 1.0)")

	// SLA & Filtering Flags
	minUptimeFlag := flag.Float64("min-uptime", 90.0, "Minimum uptime SLA % threshold (default 90.0%)")
	minTPSFlag := flag.Float64("min-tps", 0.0, "Minimum required throughput in tokens/sec (0 = no requirement)")
	maxTTFTFlag := flag.Float64("max-ttft", 0.0, "Maximum allowable TTFT in milliseconds (0 = no requirement)")
	includeOfflineFlag := flag.Bool("include-offline", false, "Include offline or degraded endpoints in scoring")

	// Standard CLI Flags
	modelsFlag := flag.String("models", "", "Comma-separated list of model IDs")
	topNFlag := flag.Int("top", 3, "Number of top providers to recommend")
	apiKeyFlag := flag.String("key", "", "OpenRouter API Key (optional, or set OPENROUTER_API_KEY env)")
	jsonOutputFlag := flag.Bool("json", false, "Output results in JSON format")
	baseURLFlag := flag.String("base-url", api.DefaultBaseURL, "OpenRouter API base URL")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "RouterLens — Intelligent OpenRouter Model Provider Scoring & Ranking Engine\n\n")
		fmt.Fprintf(os.Stderr, "Evaluates OpenRouter providers using a multi-dimensional formula:\n")
		fmt.Fprintf(os.Stderr, "  - Cost & Cache Lifecycle Economics (Read, Write, TTL, Sticky Routing)\n")
		fmt.Fprintf(os.Stderr, "  - Quantization Quality (with First-Party Lossless Registry)\n")
		fmt.Fprintf(os.Stderr, "  - Multi-window Bayesian Reliability\n")
		fmt.Fprintf(os.Stderr, "  - Monotonic TTFT and Throughput Performance Utilities\n")
		fmt.Fprintf(os.Stderr, "  - Optional e-Pareto Dominance Filtering\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  routerlens [flags] [model_id_1] [model_id_2] ...\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  routerlens deepseek/deepseek-v4-flash\n")
		fmt.Fprintf(os.Stderr, "  routerlens -profile coding deepseek/deepseek-v4-flash openai/gpt-5.6-luna\n")
		fmt.Fprintf(os.Stderr, "  routerlens -profile low_latency -pareto deepseek/deepseek-v4-flash\n")
		fmt.Fprintf(os.Stderr, "  routerlens -json deepseek/deepseek-v4-flash\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	// Gather Model IDs from flags and positional arguments
	var modelIDs []string
	if *modelsFlag != "" {
		for _, m := range strings.Split(*modelsFlag, ",") {
			m = strings.TrimSpace(m)
			if m != "" {
				modelIDs = append(modelIDs, m)
			}
		}
	}

	for _, arg := range flag.Args() {
		arg = strings.TrimSpace(arg)
		if arg != "" {
			modelIDs = append(modelIDs, arg)
		}
	}

	// If no models provided, fall back to interactive prompt
	if len(modelIDs) == 0 {
		if *jsonOutputFlag {
			fmt.Fprintln(os.Stderr, "Error: No model IDs provided for JSON output mode. Specify models as arguments or via -models.")
			os.Exit(1)
		}

		fmt.Print("Enter OpenRouter Model ID (e.g. deepseek/deepseek-v4-flash, or comma-separated list): ")
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read input: %v\n", err)
			os.Exit(1)
		}
		input = strings.TrimSpace(input)
		if input == "" {
			fmt.Fprintln(os.Stderr, "No model specified. Exiting.")
			os.Exit(0)
		}

		for _, m := range strings.Split(input, ",") {
			m = strings.TrimSpace(m)
			if m != "" {
				modelIDs = append(modelIDs, m)
			}
		}
	}

	apiKey := *apiKeyFlag
	if apiKey == "" {
		apiKey = os.Getenv("OPENROUTER_API_KEY")
	}

	// Initialize Configuration with selected profile
	cfg := scorer.DefaultConfig()
	cfg.ApplyProfile(*profileFlag)
	cfg.TopN = *topNFlag
	cfg.PromptRatio = *promptRatioFlag
	cfg.CompletionRatio = *compRatioFlag
	cfg.CacheHitRate = *cacheHitRateFlag
	cfg.CacheWriteRatio = *cacheWriteRatioFlag
	cfg.CacheTTL = *cacheTTLFlag
	cfg.RequestInterval = *reqIntervalFlag
	cfg.StickyRoutingProb = *stickyProbFlag
	cfg.MinUptime = *minUptimeFlag
	cfg.MinTPS = *minTPSFlag
	cfg.MaxTTFT = *maxTTFTFlag
	cfg.IncludeOffline = *includeOfflineFlag
	cfg.EnablePareto = *paretoFlag
	cfg.ParetoEpsilon = *paretoEpsFlag

	// Apply explicit weight overrides if provided
	if *priceWeightFlag >= 0.0 {
		cfg.PriceWeight = *priceWeightFlag
		cfg.Profile = scorer.ProfileCustom
	}
	if *quantWeightFlag >= 0.0 {
		cfg.QuantWeight = *quantWeightFlag
		cfg.Profile = scorer.ProfileCustom
	}
	if *relWeightFlag >= 0.0 {
		cfg.RelWeight = *relWeightFlag
		cfg.Profile = scorer.ProfileCustom
	}
	if *ttftWeightFlag >= 0.0 {
		cfg.TTFTWeight = *ttftWeightFlag
		cfg.Profile = scorer.ProfileCustom
	}
	if *latWeightFlag >= 0.0 {
		cfg.LatencyWeight = *latWeightFlag
		cfg.Profile = scorer.ProfileCustom
	}
	if *tpsWeightFlag >= 0.0 {
		cfg.TPSWeight = *tpsWeightFlag
		cfg.Profile = scorer.ProfileCustom
	}

	client := api.NewClient(apiKey, *baseURLFlag)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	if !*jsonOutputFlag {
		fmt.Printf("Fetching OpenRouter endpoint data for %d model(s)...\n", len(modelIDs))
	}

	type fetchResult struct {
		index int
		model string
		resp  *api.EndpointsResponse
		err   error
	}

	resultChan := make(chan fetchResult, len(modelIDs))
	var wg sync.WaitGroup

	for i, m := range modelIDs {
		wg.Add(1)
		go func(idx int, modelID string) {
			defer wg.Done()
			resp, err := client.FetchModelEndpoints(ctx, modelID)
			resultChan <- fetchResult{
				index: idx,
				model: modelID,
				resp:  resp,
				err:   err,
			}
		}(i, m)
	}

	wg.Wait()
	close(resultChan)

	orderedResults := make([]fetchResult, len(modelIDs))
	for res := range resultChan {
		orderedResults[res.index] = res
	}

	var evaluationResults []*scorer.ModelEvaluationResult

	viewMode := ui.ViewModeAuto
	if *cardFlag {
		viewMode = ui.ViewModeCard
	} else if *compactFlag {
		viewMode = ui.ViewModeCompact
	} else if *wideFlag {
		viewMode = ui.ViewModeWide
	}

	for _, res := range orderedResults {
		if res.err != nil {
			if *jsonOutputFlag {
				evaluationResults = append(evaluationResults, &scorer.ModelEvaluationResult{
					ModelID:   res.model,
					ModelName: fmt.Sprintf("Error: %v", res.err),
				})
			} else {
				fmt.Fprintf(os.Stderr, "\n%s Error fetching '%s': %v\n", "\033[31m[ERROR]\033[0m", res.model, res.err)
			}
			continue
		}

		eval := scorer.EvaluateModel(res.resp, cfg)
		evaluationResults = append(evaluationResults, eval)

		if !*jsonOutputFlag {
			ui.RenderModelResult(os.Stdout, eval, cfg, viewMode)
		}
	}

	if *jsonOutputFlag {
		if err := ui.RenderJSON(os.Stdout, evaluationResults, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error rendering JSON: %v\n", err)
			os.Exit(1)
		}
	}
}
