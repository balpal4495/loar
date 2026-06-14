package retrieval

import "strings"

// Intent classifies the type of a user query to guide retrieval strategy.
// Classification is internal; users never choose a mode explicitly.
type Intent string

const (
	IntentCausal     Intent = "causal_analysis"
	IntentTimeline   Intent = "timeline"
	IntentComparison Intent = "comparison"
	IntentEvidence   Intent = "evidence"
	IntentGeneral    Intent = "general"
)

// String implements fmt.Stringer.
func (i Intent) String() string { return string(i) }

// causalSignals are words that indicate a causal analysis query.
var causalSignals = []string{"why", "because", "cause", "reason", "reliable", "trust"}

// timelineSignals are words that indicate a timeline query.
var timelineSignals = []string{"when", "first", "last", "history", "timeline", "before", "after"}

// comparisonSignals are words that indicate a comparison query.
var comparisonSignals = []string{"compare", "versus", "vs", "differ", "difference", "better", "worse"}

// evidenceSignals are words that indicate an evidence query.
var evidenceSignals = []string{"evidence", "support", "show", "proof", "signal", "observation"}

// DetectIntent classifies a natural-language query into an Intent.
// The classification is based on keyword matching and is intentionally simple;
// it is designed to be replaced with more sophisticated intent detection.
func DetectIntent(query string) Intent {
	lower := strings.ToLower(query)
	words := strings.Fields(lower)

	scores := map[Intent]int{
		IntentCausal:     0,
		IntentTimeline:   0,
		IntentComparison: 0,
		IntentEvidence:   0,
	}

	for _, w := range words {
		w = strings.Trim(w, "?.,!\"'")
		for _, s := range causalSignals {
			if w == s {
				scores[IntentCausal]++
			}
		}
		for _, s := range timelineSignals {
			if w == s {
				scores[IntentTimeline]++
			}
		}
		for _, s := range comparisonSignals {
			if w == s {
				scores[IntentComparison]++
			}
		}
		for _, s := range evidenceSignals {
			if w == s {
				scores[IntentEvidence]++
			}
		}
	}

	best := IntentGeneral
	bestScore := 0
	for intent, score := range scores {
		if score > bestScore {
			bestScore = score
			best = intent
		}
	}
	return best
}
