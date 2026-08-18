package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"routerlens/internal/scorer"
)

// ANSI color codes
const (
	ColorReset   = "\033[0m"
	ColorBold    = "\033[1m"
	ColorDim     = "\033[2m"
	ColorCyan    = "\033[36m"
	ColorGreen   = "\033[32m"
	ColorYellow  = "\033[33m"
	ColorBlue    = "\033[34m"
	ColorPurple  = "\033[35m"
	ColorGray    = "\033[90m"
	ColorWhite   = "\033[97m"
	ColorGold    = "\033[38;5;220m"
	ColorSilver  = "\033[38;5;250m"
	ColorBronze  = "\033[38;5;172m"
)

func useColor() bool {
	if _, noColor := os.LookupEnv("NO_COLOR"); noColor {
		return false
	}
	return true
}

func colorize(text string, colorCode string) string {
	if !useColor() || colorCode == "" {
		return text
	}
	return colorCode + text + ColorReset
}

// stringVisualWidth computes visible terminal character width
func stringVisualWidth(s string) int {
	clean := stripAnsi(s)
	width := 0
	for _, r := range clean {
		if r > 0x1F000 {
			width += 2
		} else {
			width += 1
		}
	}
	return width
}

func stripAnsi(s string) string {
	var b strings.Builder
	inEsc := false
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b { // ESC
			inEsc = true
			continue
		}
		if inEsc {
			if s[i] == 'm' {
				inEsc = false
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func padRight(s string, width int) string {
	vis := stringVisualWidth(s)
	if vis >= width {
		return s
	}
	return s + strings.Repeat(" ", width-vis)
}

func padLeft(s string, width int) string {
	vis := stringVisualWidth(s)
	if vis >= width {
		return s
	}
	return strings.Repeat(" ", width-vis) + s
}

func truncateString(s string, maxLen int) string {
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}

// RenderModelResult prints a formatted table of provider recommendations
func RenderModelResult(w io.Writer, res *scorer.ModelEvaluationResult, cfg scorer.ScoringConfig) {
	if res == nil {
		return
	}

	fmt.Fprintf(w, "\n%s %s\n", colorize("▶ MODEL:", ColorBold+ColorCyan), colorize(res.ModelID, ColorBold+ColorWhite))
	if res.ModelName != "" && res.ModelName != res.ModelID {
		fmt.Fprintf(w, "  %s %s\n", colorize("Name:", ColorDim), res.ModelName)
	}

	paretoInfo := ""
	if cfg.EnablePareto {
		paretoInfo = fmt.Sprintf(" | Pareto Filter: Active (%d filtered)", res.ParetoFilteredCount)
	}

	fmt.Fprintf(w, "  %s Total: %d | Active: %d | Eligible: %s%s\n",
		colorize("Status:", ColorDim),
		res.TotalEndpoints,
		res.ActiveEndpoints,
		colorize(fmt.Sprintf("%d", res.EligibleEndpoints), ColorGreen),
		colorize(paretoInfo, ColorPurple))

	fmt.Fprintf(w, "  %s Profile: %s [Cost: %.0f%%, Quant: %.0f%%, Rel: %.0f%%, TTFT: %.0f%%, TPS: %.0f%%] | Cache Hit: %.0f%%\n",
		colorize("Weights:", ColorDim),
		colorize(strings.ToUpper(cfg.Profile), ColorBold+ColorYellow),
		cfg.PriceWeight*100,
		cfg.QuantWeight*100,
		cfg.RelWeight*100,
		cfg.TTFTWeight*100,
		cfg.TPSWeight*100,
		cfg.CacheHitRate*100)

	if len(res.TopProviders) == 0 {
		fmt.Fprintf(w, "\n  %s No eligible providers found for this model.\n\n", colorize("⚠", ColorYellow))
		return
	}

	fmt.Fprintln(w)

	// Column Widths
	rankW := 6
	providerW := 20
	quantW := 12
	promptW := 11
	cacheW := 11
	compW := 11
	blendedW := 12
	scoreBreakdownW := 19
	totalScoreW := 15
	uptimeW := 10
	contextW := 9

	cols := []int{rankW, providerW, quantW, promptW, cacheW, compW, blendedW, scoreBreakdownW, totalScoreW, uptimeW, contextW}

	printDivider := func(left, mid, right, line string) {
		var parts []string
		for _, w := range cols {
			parts = append(parts, strings.Repeat(line, w+2))
		}
		fmt.Fprintf(w, "%s%s%s\n", left, strings.Join(parts, mid), right)
	}

	// Table Header
	printDivider("┌", "┬", "┐", "─")
	fmt.Fprintf(w, "│ %s │ %s │ %s │ %s │ %s │ %s │ %s │ %s │ %s │ %s │ %s │\n",
		padRight(colorize("RANK", ColorBold), rankW),
		padRight(colorize("PROVIDER", ColorBold), providerW),
		padRight(colorize("QUANT", ColorBold), quantW),
		padLeft(colorize("PROMPT $/M", ColorBold), promptW),
		padLeft(colorize("CACHE $/M", ColorBold), cacheW),
		padLeft(colorize("COMPL $/M", ColorBold), compW),
		padLeft(colorize("BLENDED $/M", ColorBold), blendedW),
		padLeft(colorize("C / Q / R / T / P", ColorBold), scoreBreakdownW),
		padLeft(colorize("TOTAL SCORE", ColorBold), totalScoreW),
		padLeft(colorize("UPTIME", ColorBold), uptimeW),
		padLeft(colorize("CONTEXT", ColorBold), contextW),
	)
	printDivider("├", "┼", "┤", "─")

	for _, p := range res.TopProviders {
		var rankDisplay string
		switch p.Rank {
		case 1:
			rankDisplay = colorize("#1 TOP", ColorBold+ColorYellow)
		case 2:
			rankDisplay = colorize("#2", ColorBold+ColorCyan)
		case 3:
			rankDisplay = colorize("#3", ColorBold+ColorPurple)
		default:
			rankDisplay = colorize(fmt.Sprintf("#%d", p.Rank), ColorDim)
		}

		providerName := truncateString(p.ProviderName, providerW)

		quantDisplay := p.Quantization
		if p.IsVerifiedFirstParty && (quantDisplay == "" || quantDisplay == "unknown") {
			quantDisplay = "fp-lossless"
		}
		if quantDisplay == "" {
			quantDisplay = "unknown"
		}
		quantDisplay = truncateString(quantDisplay, quantW)

		// Colorize quant badge
		if p.QuantScore >= 0.99 {
			quantDisplay = colorize(quantDisplay, ColorGreen)
		} else if p.QuantScore >= 0.59 {
			quantDisplay = colorize(quantDisplay, ColorYellow)
		} else {
			quantDisplay = colorize(quantDisplay, ColorDim)
		}

		promptStr := formatPrice(p.PromptPricePerM)
		cacheStr := formatPrice(p.CacheReadPricePerM)
		compStr := formatPrice(p.CompletionPricePerM)
		blendedStr := formatPrice(p.BlendedCostPerM)
		if p.IsFreeTier {
			blendedStr = colorize("FREE", ColorBold+ColorGreen)
		}

		scoresBreakdown := fmt.Sprintf("%.2f/%.2f/%.2f/%.2f/%.2f",
			p.CostScore, p.QuantScore, p.RelScore, p.TTFTScore, p.TPSScore)
		totalScoreStr := fmt.Sprintf("%.1f%% (%.3f)", p.CompositeScore*100, p.CompositeScore)

		if p.Rank == 1 {
			totalScoreStr = colorize(totalScoreStr, ColorBold+ColorGreen)
		}

		uptimeStr := fmt.Sprintf("%.1f%%", p.BlendedUptime)
		if p.BlendedUptime >= 99.0 {
			uptimeStr = colorize(uptimeStr, ColorGreen)
		} else if p.BlendedUptime >= 95.0 {
			uptimeStr = colorize(uptimeStr, ColorYellow)
		} else {
			uptimeStr = colorize(uptimeStr, ColorDim)
		}

		ctxStr := formatContextLength(p.ContextLength)

		fmt.Fprintf(w, "│ %s │ %s │ %s │ %s │ %s │ %s │ %s │ %s │ %s │ %s │ %s │\n",
			padRight(rankDisplay, rankW),
			padRight(providerName, providerW),
			padRight(quantDisplay, quantW),
			padLeft(promptStr, promptW),
			padLeft(cacheStr, cacheW),
			padLeft(compStr, compW),
			padLeft(blendedStr, blendedW),
			padLeft(scoresBreakdown, scoreBreakdownW),
			padLeft(totalScoreStr, totalScoreW),
			padLeft(uptimeStr, uptimeW),
			padLeft(ctxStr, contextW),
		)
	}

	printDivider("└", "┴", "┘", "─")

	// Recommendation Footer Note
	if len(res.TopProviders) > 0 {
		top := res.TopProviders[0]
		priceDesc := formatPrice(top.BlendedCostPerM) + " blended $/M"
		if top.IsFreeTier {
			priceDesc = "FREE"
		}
		fmt.Fprintf(w, "  %s Top Choice: %s (%s, %s, Total Score: %.1f%%)\n",
			colorize("★", ColorBold+ColorYellow),
			colorize(top.ProviderName, ColorBold+ColorWhite),
			top.Quantization,
			priceDesc,
			top.CompositeScore*100,
		)
	}

	if !res.CostCompetitivenessObservable && len(res.TopProviders) == 1 {
		fmt.Fprintf(w, "  %s %s\n",
			colorize("ℹ", ColorCyan),
			colorize("Single provider available for this model; cost competitiveness is non-comparable.", ColorDim))
	}
	fmt.Fprintln(w)
}

func formatPrice(price float64) string {
	if price == 0 {
		return "$0.00"
	}
	if price < 0.01 {
		return fmt.Sprintf("$%.4f", price)
	}
	return fmt.Sprintf("$%.2f", price)
}

func formatContextLength(ctx int64) string {
	if ctx >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(ctx)/1_000_000.0)
	}
	if ctx >= 1_000 {
		return fmt.Sprintf("%dk", ctx/1000)
	}
	return fmt.Sprintf("%d", ctx)
}
