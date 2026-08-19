# RouterLens 🔍

> High-performance Go CLI that evaluates OpenRouter model providers using cache lifecycle economics, quantization tiers, Bayesian reliability, and $\epsilon$-Pareto filtering to find the optimal endpoint for any model.

---

## Key Features

- **Multi-Dimensional Scoring Engine**:
  - **Cost & Cache Economics**: Includes prompt price, completion price, cache read discounts (`input_cache_read`), and cache write overhead (`input_cache_write`).
  - **Cache Lifecycle Economics**: Models cache TTL expiration ($e^{-\Delta t / \text{TTL}}$) and sticky-routing cache locality ($P_{\text{sticky}}$).
  - **Quantization Quality**: Strict empirical tiers ($1.0, 0.6, 0.2$) with verified first-party registry (OpenAI, Azure, Bedrock, Google, Anthropic $\to 1.00$ lossless).
  - **Bayesian Multi-Window Reliability**: Blends 30m and 1-day uptime with prior smoothing and nonlinear high-availability utility curves.
  - **Performance Telemetry**: Monotonic TTFT (Time-To-First-Token) and throughput (tokens/sec) utility functions.
- **$\epsilon$-Pareto Dominance Filtering**: Automatically eliminates strictly dominated providers (with 2% tolerance slack) while guaranteeing non-empty frontiers.
- **Workload Weight Profiles**: Pre-calibrated for `balanced`, `coding`, `low_latency`, and `batch_throughput` workloads.
- **Zero-Price & Single-Provider Semantics**: Free tier isolation without paid cohort distortion, and explicit single-provider diagnostic observability.
- **Rich Terminal Table & JSON Output**: ANSI colorized Unicode tables with full $C/Q/R/T/P$ diagnostic score breakdowns.

---

## Quick Start

### Build
```bash
go build -o routerlens.exe .
```

### Basic Usage
```bash
# Check a single model with default balanced profile
./routerlens.exe deepseek/deepseek-v4-flash

# Check multiple models simultaneously
./routerlens.exe deepseek/deepseek-v4-flash openai/gpt-5.6-luna poolside/laguna-s-2.1

# Coding profile (prioritizes unquantized precision & reliability)
./routerlens.exe -profile coding deepseek/deepseek-v4-flash

# Low-latency profile with Pareto filtering and 95% minimum uptime SLA
./routerlens.exe -profile low_latency -pareto -min-uptime 95.0 openai/gpt-5.6-luna

# Export diagnostic JSON output
./routerlens.exe -json -profile balanced deepseek/deepseek-v4-flash
```

---

## Workload Profiles

| Profile | Focus | Cost ($w_C$) | Quant ($w_Q$) | Rel ($w_R$) | TTFT ($w_T$) | TPS ($w_P$) |
|---|---|:---:|:---:|:---:|:---:|:---:|
| **`balanced`** (default) | Production balanced tradeoff | `0.40` | `0.25` | `0.15` | `0.10` | `0.10` |
| **`coding`** | Reasoning accuracy & lossless precision | `0.20` | `0.45` | `0.20` | `0.10` | `0.05` |
| **`low_latency`** | Interactive UI & voice agent response | `0.20` | `0.15` | `0.15` | `0.35` | `0.15` |
| **`batch_throughput`** | High-volume async data pipelines | `0.55` | `0.15` | `0.10` | `0.00` | `0.20` |

---

## CLI Flags

| Flag | Default | Description |
|---|---|---|
| `-profile` | `balanced` | Workload profile (`balanced`, `coding`, `low_latency`, `batch_throughput`, `custom`) |
| `-compact` | `false` | Force compact table layout (fits in standard $\le 90$ column terminals) |
| `-wide` | `false` | Force full multi-column dashboard ($\ge 125$ columns) |
| `-card` | `false` | Force card-based view for narrow/split terminal windows ($\le 65$ columns) |
| `-pareto` | `false` | Enable $\epsilon$-Pareto dominance filtering |
| `-pareto-eps` | `0.02` | Tolerance margin for Pareto dominance (default 2%) |
| `-min-uptime` | `90.0` | Minimum uptime SLA percentage threshold |
| `-min-tps` | `0.0` | Minimum required throughput in tokens/sec |
| `-max-ttft` | `0.0` | Maximum allowable TTFT in milliseconds |
| `-cache-hit` | `0.80` | Estimated prompt cache hit rate (0.0 to 1.0) |
| `-cache-write-ratio` | `0.10` | Fraction of prompt tokens requiring cache write |
| `-cache-ttl` | `300.0` | Cache retention TTL in seconds |
| `-req-interval` | `30.0` | Mean request interval in seconds |
| `-sticky-prob` | `1.0` | Probability of routing to the same provider |
| `-top` | `3` | Number of top recommendations to display |
| `-json` | `false` | Output results in JSON format |
| `-key` | `""` | OpenRouter API Key (or set `OPENROUTER_API_KEY` env) |

---

## Testing

Run the full automated test suite:
```bash
go test ./... -v
```
