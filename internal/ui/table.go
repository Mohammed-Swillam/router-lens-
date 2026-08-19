package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"

	"routerlens/internal/scorer"
)

// ViewMode defines terminal rendering style
type ViewMode string

const (
	ViewModeAuto    ViewMode = "auto"
	ViewModeCompact ViewMode = "compact"
	ViewModeWide    ViewMode = "wide"
	ViewModeCard    ViewMode = "card"
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

// stringVisualWidth computes visible terminal character width (ANSI escape code safe)
func stringVisualWidth(s string) int {
	clean := stripAnsi(s)
	width := 0
	for _, r := range clean {
		// Emoji & CJK full-width detection
		if r > 0x1F000 || (r >= 0x4E00 && r <= 0x9FFF) {
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
	return string(runes[:maxLen-2]) + ".."
}

// DetectTerminalWidth returns detected column width or fallback
func DetectTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		return 90 // Standard comfortable default
	}
	return width
}

// RenderModelResult prints an adaptive, beautiful layout based on terminal size and mode
func RenderModelResult(w io.Writer, res *scorer.ModelEvaluationResult, cfg scorer.ScoringConfig, mode ViewMode) {
	if res == nil {
		return
	}

	termWidth := DetectTerminalWidth()

	// Resolve auto mode
	resolvedMode := mode
	if resolvedMode == ViewModeAuto || resolvedMode == "" {
		if termWidth < 75 {
			resolvedMode = ViewModeCard
		} else if termWidth < 125 {
			resolvedMode = ViewModeCompact
		} else {
			resolvedMode = ViewModeWide
		}
	}

	// Model Header Banner
	fmt.Fprintf(w, "\n%s %s\n", colorize("▶ MODEL:", ColorBold+ColorCyan), colorize(res.ModelID, ColorBold+ColorWhite))
	if res.ModelName != "" && res.ModelName != res.ModelID {
		fmt.Fprintf(w, "  %s %s\n", colorize("Name:", ColorDim), res.ModelName)
	}

	paretoInfo := ""
	if cfg.EnablePareto {
		paretoInfo = fmt.Sprintf(" | Pareto: %d filtered", res.ParetoFilteredCount)
	}

	fmt.Fprintf(w, "  %s Total: %d | Active: %d | Eligible: %s%s\n",
		colorize("Status:", ColorDim),
		res.TotalEndpoints,
		res.ActiveEndpoints,
		colorize(fmt.Sprintf("%d", res.EligibleEndpoints), ColorGreen),
		colorize(paretoInfo, ColorPurple))

	fmt.Fprintf(w, "  %s Profile: %s [Cost: %.0f%%, Quant: %.0f%%, Rel: %.0f%%, TTFT: %.0f%%, TPS: %.0f%%] | Cache: %.0f%%\n",
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

	switch resolvedMode {
	case ViewModeCard:
		renderCardLayout(w, res, cfg)
	case ViewModeWide:
		renderWideTable(w, res, cfg)
	case ViewModeCompact:
		fallthrough
	default:
		renderCompactTable(w, res, cfg)
	}

	// Footer Summary & Diagnosis
	if len(res.TopProviders) > 0 {
		top := res.TopProviders[0]
		priceDesc := formatPrice(top.BlendedCostPerM) + " blended/M"
		if top.IsFreeTier {
			priceDesc = "FREE"
		}
		cacheDesc := ""
		if top.CacheReadPricePerM > 0 {
			cacheDesc = fmt.Sprintf(" (Cache: %s/M)", formatPrice(top.CacheReadPricePerM))
		}

		fmt.Fprintf(w, "  %s Top Choice: %s (%s, %s%s, Score: %.1f%%)\n",
			colorize("★", ColorBold+ColorYellow),
			colorize(top.ProviderName, ColorBold+ColorWhite),
			top.Quantization,
			priceDesc,
			cacheDesc,
			top.CompositeScore*100,
		)
	}

	if !res.CostCompetitivenessObservable && len(res.TopProviders) == 1 {
		fmt.Fprintf(w, "  %s %s\n",
			colorize("ℹ", ColorCyan),
			colorize("Single provider available; cost competitiveness is non-comparable.", ColorDim))
	}
	fmt.Fprintln(w)
}

// renderCompactTable renders a sleek ~82-column table guaranteed not to wrap on standard terminal windows
func renderCompactTable(w io.Writer, res *scorer.ModelEvaluationResult, cfg scorer.ScoringConfig) {
	rankW := 6
	providerW := 18
	scoreW := 12
	costW := 11
	quantW := 12
	uptimeW := 9
	contextW := 8

	cols := []int{rankW, providerW, scoreW, costW, quantW, uptimeW, contextW}

	printDivider := func(left, mid, right, line string) {
		var parts []string
		for _, cw := range cols {
			parts = append(parts, strings.Repeat(line, cw+2))
		}
		fmt.Fprintf(w, "%s%s%s\n", left, strings.Join(parts, mid), right)
	}

	printDivider("┌", "┬", "┐", "─")
	fmt.Fprintf(w, "│ %s │ %s │ %s │ %s │ %s │ %s │ %s │\n",
		padRight(colorize("RANK", ColorBold), rankW),
		padRight(colorize("PROVIDER", ColorBold), providerW),
		padLeft(colorize("SCORE", ColorBold), scoreW),
		padLeft(colorize("BLENDED $/M", ColorBold), costW),
		padRight(colorize("QUANT", ColorBold), quantW),
		padLeft(colorize("UPTIME", ColorBold), uptimeW),
		padLeft(colorize("CONTEXT", ColorBold), contextW),
	)
	printDivider("├", "┼", "┤", "─")

	for _, p := range res.TopProviders {
		rankDisplay := formatRank(p.Rank)
		providerName := truncateString(p.ProviderName, providerW)

		scoreStr := fmt.Sprintf("%.1f%%", p.CompositeScore*100)
		if p.Rank == 1 {
			scoreStr = colorize("● "+scoreStr, ColorBold+ColorGreen)
		} else if p.Rank == 2 {
			scoreStr = colorize("● "+scoreStr, ColorCyan)
		} else {
			scoreStr = colorize("● "+scoreStr, ColorPurple)
		}

		costStr := formatPrice(p.BlendedCostPerM)
		if p.IsFreeTier {
			costStr = colorize("FREE", ColorBold+ColorGreen)
		}

		quantDisplay := formatQuant(p.Quantization, p.QuantScore, p.IsVerifiedFirstParty, quantW)
		uptimeStr := formatUptime(p.BlendedUptime)
		ctxStr := formatContextLength(p.ContextLength)

		fmt.Fprintf(w, "│ %s │ %s │ %s │ %s │ %s │ %s │ %s │\n",
			padRight(rankDisplay, rankW),
			padRight(providerName, providerW),
			padLeft(scoreStr, scoreW),
			padLeft(costStr, costW),
			padRight(quantDisplay, quantW),
			padLeft(uptimeStr, uptimeW),
			padLeft(ctxStr, contextW),
		)
	}
	printDivider("└", "┴", "┘", "─")

	// Diagnostic breakdown summary underneath table
	fmt.Fprintf(w, "  %s\n", colorize("Breakdown (Cost / Quant / Rel / TTFT / TPS):", ColorDim))
	for _, p := range res.TopProviders {
		fmt.Fprintf(w, "    %s %-16s │ C:%.2f  Q:%.2f  R:%.2f  T:%.2f  P:%.2f │ Prompt: %s/M  Cache: %s/M\n",
			formatRankShort(p.Rank),
			truncateString(p.ProviderName, 16),
			p.CostScore, p.QuantScore, p.RelScore, p.TTFTScore, p.TPSScore,
			formatPrice(p.PromptPricePerM),
			formatPrice(p.CacheReadPricePerM),
		)
	}
	fmt.Fprintln(w)
}

// renderWideTable renders full multi-column dashboard for large screens (>= 125 cols)
func renderWideTable(w io.Writer, res *scorer.ModelEvaluationResult, cfg scorer.ScoringConfig) {
	rankW := 6
	providerW := 18
	quantW := 12
	promptW := 11
	cacheW := 11
	compW := 11
	blendedW := 12
	scoreBreakdownW := 19
	totalScoreW := 15
	uptimeW := 9
	contextW := 8

	cols := []int{rankW, providerW, quantW, promptW, cacheW, compW, blendedW, scoreBreakdownW, totalScoreW, uptimeW, contextW}

	printDivider := func(left, mid, right, line string) {
		var parts []string
		for _, cw := range cols {
			parts = append(parts, strings.Repeat(line, cw+2))
		}
		fmt.Fprintf(w, "%s%s%s\n", left, strings.Join(parts, mid), right)
	}

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
		rankDisplay := formatRank(p.Rank)
		providerName := truncateString(p.ProviderName, providerW)
		quantDisplay := formatQuant(p.Quantization, p.QuantScore, p.IsVerifiedFirstParty, quantW)

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

		uptimeStr := formatUptime(p.BlendedUptime)
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
	fmt.Fprintln(w)
}

// renderCardLayout renders stacked cards for very small/narrow windows (< 75 cols)
func renderCardLayout(w io.Writer, res *scorer.ModelEvaluationResult, cfg scorer.ScoringConfig) {
	for _, p := range res.TopProviders {
		rankColor := ColorYellow
		if p.Rank == 2 {
			rankColor = ColorCyan
		} else if p.Rank >= 3 {
			rankColor = ColorPurple
		}

		fmt.Fprintf(w, "┌── %s %s %s\n",
			colorize(fmt.Sprintf("#%d", p.Rank), ColorBold+rankColor),
			colorize(p.ProviderName, ColorBold+ColorWhite),
			colorize(fmt.Sprintf("[Score: %.1f%%]", p.CompositeScore*100), ColorBold+ColorGreen),
		)

		costStr := formatPrice(p.BlendedCostPerM) + "/M"
		if p.IsFreeTier {
			costStr = "FREE"
		}
		cacheStr := formatPrice(p.CacheReadPricePerM) + "/M"

		fmt.Fprintf(w, "│  %s %-12s │  %s %-10s │  %s %-8s\n",
			colorize("Blended:", ColorDim), costStr,
			colorize("Cache:", ColorDim), cacheStr,
			colorize("Quant:", ColorDim), p.Quantization,
		)
		fmt.Fprintf(w, "│  %s %-12s │  %s %-10s │  %s %-8s\n",
			colorize("Uptime:", ColorDim), fmt.Sprintf("%.1f%%", p.BlendedUptime),
			colorize("Context:", ColorDim), formatContextLength(p.ContextLength),
			colorize("Prompt:", ColorDim), formatPrice(p.PromptPricePerM)+"/M",
		)
		fmt.Fprintf(w, "│  %s C:%.2f  Q:%.2f  R:%.2f  T:%.2f  P:%.2f\n",
			colorize("Scores:", ColorDim),
			p.CostScore, p.QuantScore, p.RelScore, p.TTFTScore, p.TPSScore,
		)
		fmt.Fprintf(w, "└─────────────────────────────────────────────────────────────\n\n")
	}
}

func formatRank(rank int) string {
	switch rank {
	case 1:
		return colorize("#1 TOP", ColorBold+ColorYellow)
	case 2:
		return colorize("#2", ColorBold+ColorCyan)
	case 3:
		return colorize("#3", ColorBold+ColorPurple)
	default:
		return colorize(fmt.Sprintf("#%d", rank), ColorDim)
	}
}

func formatRankShort(rank int) string {
	switch rank {
	case 1:
		return colorize("#1", ColorBold+ColorYellow)
	case 2:
		return colorize("#2", ColorBold+ColorCyan)
	case 3:
		return colorize("#3", ColorBold+ColorPurple)
	default:
		return colorize(fmt.Sprintf("#%d", rank), ColorDim)
	}
}

func formatQuant(quant string, score float64, isFP bool, maxLen int) string {
	display := quant
	if isFP && (display == "" || display == "unknown") {
		display = "fp-lossless"
	}
	if display == "" {
		display = "unknown"
	}
	display = truncateString(display, maxLen)

	if score >= 0.99 {
		return colorize(display, ColorGreen)
	} else if score >= 0.59 {
		return colorize(display, ColorYellow)
	}
	return colorize(display, ColorDim)
}

func formatUptime(uptime float64) string {
	str := fmt.Sprintf("%.1f%%", uptime)
	if uptime >= 99.0 {
		return colorize(str, ColorGreen)
	} else if uptime >= 95.0 {
		return colorize(str, ColorYellow)
	}
	return colorize(str, ColorDim)
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
