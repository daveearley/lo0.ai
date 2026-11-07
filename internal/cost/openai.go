package cost

// Where OpenAI's prices come from and when they were taken. GPT-5.2-Codex,
// GPT-5.1-Codex and GPT-5-Codex are shut down and no longer on the pricing
// page; their rates are from their model pages under
// developers.openai.com/api/docs/models on the same date.
const (
	OpenAIPricesSource = "https://developers.openai.com/api/docs/pricing"
	OpenAIPricesAsOf   = "2026-11-06"
)

// openAIPrices are Standard-tier rates for prompts up to 272k tokens. The
// page's 30-minute and 12-hour cache writes fill the CacheWrite5m and
// CacheWrite1h columns; models it lists no write price for bill writes as
// input, and models with no cached-input price bill cache reads as input.
var openAIPrices = map[string]Rates{
	"gpt-6-astra":   {10, 12.50, 25, 1, 50},
	"gpt-6-sol":     {2, 2.50, 5, 0.20, 10},
	"gpt-6-luna":    {0.10, 0.125, 0.25, 0.01, 0.50},
	"gpt-6.1-sol":   {2, 2.50, 5, 0.10, 10},
	"gpt-6.1-terra": {1, 1.25, 2.50, 0.05, 5},
	// GPT-5.6 Sol is at a promotional price until 30 November 2026.
	"gpt-5.6-sol":   {4, 5, 10, 0.40, 20},
	"gpt-5.6-terra": {2, 2.50, 5, 0.20, 12},
	"gpt-5.6-luna":  {0.20, 0.25, 0.50, 0.02, 1.20},
	"gpt-5.5":       noWrite(5, 0.50, 30),
	"gpt-5.5-pro":   noWrite(30, 30, 180),
	"gpt-5.4":       noWrite(2.50, 0.25, 15),
	"gpt-5.4-pro":   noWrite(30, 30, 180),
	"gpt-5.4-mini":  noWrite(0.75, 0.075, 4.50),
	"gpt-5.4-nano":  noWrite(0.20, 0.02, 1.25),
	"gpt-5.3-codex": noWrite(1.75, 0.175, 14),
	"gpt-5.2":       noWrite(1.75, 0.175, 14),
	"gpt-5.2-pro":   noWrite(21, 21, 168),
	"gpt-5.2-codex": noWrite(1.75, 0.175, 14),
	"gpt-5.1":       noWrite(1.25, 0.125, 10),
	"gpt-5.1-codex": noWrite(1.25, 0.125, 10),
	"gpt-5":         noWrite(1.25, 0.125, 10),
	"gpt-5-codex":   noWrite(1.25, 0.125, 10),
	"gpt-5-mini":    noWrite(0.25, 0.025, 2),
	"gpt-5-nano":    noWrite(0.05, 0.005, 0.40),
	"gpt-5-pro":     noWrite(15, 15, 120),
}

// openAILongContext are rates for prompts over 272k tokens, where they
// differ.
var openAILongContext = map[string]Rates{
	"gpt-6-astra":   {20, 25, 50, 2, 75},
	"gpt-6-sol":     {4, 5, 10, 0.40, 15},
	"gpt-6-luna":    {0.20, 0.25, 0.50, 0.02, 0.75},
	"gpt-6.1-sol":   {4, 5, 10, 0.20, 15},
	"gpt-6.1-terra": {2, 2.50, 5, 0.10, 7.50},
	"gpt-5.6-sol":   {8, 10, 20, 0.80, 30},
	"gpt-5.6-terra": {4, 5, 10, 0.40, 18},
	"gpt-5.6-luna":  {0.40, 0.50, 1, 0.04, 1.80},
	"gpt-5.5":       noWrite(10, 1, 45),
	"gpt-5.5-pro":   noWrite(60, 60, 270),
	"gpt-5.4":       noWrite(5, 0.50, 22.50),
	"gpt-5.4-pro":   noWrite(60, 60, 270),
}

// noWrite is the rates of a model the page lists no cache write price for.
func noWrite(input, cacheRead, output float64) Rates {
	return Rates{input, input, input, cacheRead, output}
}
