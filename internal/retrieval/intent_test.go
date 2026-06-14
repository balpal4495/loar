package retrieval_test

import (
	"testing"

	"github.com/balpal4495/loar/internal/retrieval"
)

var intentTests = []struct {
	query    string
	expected retrieval.Intent
}{
	{"Why is Romano reliable?", retrieval.IntentCausal},
	{"When did Arsenal first show interest?", retrieval.IntentTimeline},
	{"Compare Romano and Ornstein.", retrieval.IntentComparison},
	{"Show evidence that supports this signal.", retrieval.IntentEvidence},
	{"Tell me about transfers.", retrieval.IntentGeneral},
}

func TestDetectIntent(t *testing.T) {
	for _, tt := range intentTests {
		t.Run(tt.query, func(t *testing.T) {
			got := retrieval.DetectIntent(tt.query)
			if got != tt.expected {
				t.Errorf("DetectIntent(%q) = %q, want %q", tt.query, got, tt.expected)
			}
		})
	}
}

func TestIntentString(t *testing.T) {
	if retrieval.IntentCausal.String() != "causal_analysis" {
		t.Errorf("IntentCausal.String() = %q, want causal_analysis", retrieval.IntentCausal.String())
	}
	if retrieval.IntentTimeline.String() != "timeline" {
		t.Errorf("IntentTimeline.String() = %q, want timeline", retrieval.IntentTimeline.String())
	}
}
