package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/balpal4495/loar/internal/domain"
	"github.com/balpal4495/loar/internal/narrative"
	"github.com/balpal4495/loar/internal/retrieval"
	"github.com/spf13/cobra"
)

// NewQueryCmd returns the `loar` default command for natural-language queries.
func NewQueryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "query <question>",
		Short: "Query the knowledge store",
		Long: `Runs a natural-language query against the current project's knowledge store
and returns a narrative answer with supporting evidence.

Example:
  loar query "What is Phase 4?"`,
		Args: cobra.MinimumNArgs(1),
		RunE: runQuery,
	}
}

// NewExplainCmd returns the `loar explain` command.
// It emits a JSON context package intended for AI agent consumption.
// With --human it routes through the narrative renderer instead.
func NewExplainCmd() *cobra.Command {
	var human bool

	cmd := &cobra.Command{
		Use:   "explain <question>",
		Short: "Emit a JSON context package for AI agent consumption",
		Long: `Retrieves evidence from the knowledge store and emits a structured JSON
context package. Intended for AI agents (Copilot, Claude Code, etc.) to
consume and synthesize into a narrative answer.

The AI should:
  1. Read the observations array (full content, chronological order)
  2. Synthesize a clear answer for the user
  3. Surface any contradictions explicitly
  4. Use occurred_at dates to establish temporal context

Use --human for a readable narrative summary in the terminal.

Examples:
  loar explain "Why is Romano reliable?"
  loar explain "What is Phase 4?" | jq .observations
  loar explain "What is Phase 4?" --human`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if human {
				return runQuery(cmd, args)
			}
			return runExplain(cmd, args)
		},
	}
	cmd.Flags().BoolVar(&human, "human", false, "Render as narrative prose instead of JSON")
	return cmd
}

func runQuery(cmd *cobra.Command, args []string) error {
	pkg, err := retrieve(cmd, args)
	if err != nil {
		return err
	}

	// Sort observations chronologically — both renderers use this order.
	obs := make([]domain.Observation, len(pkg.Observations))
	copy(obs, pkg.Observations)
	sortByDate(obs)

	// Attempt NLG narrative via embedded Node.js script.
	text, err := narrative.Render(pkg, obs)
	if err != nil {
		if errors.Is(err, narrative.ErrNodeNotFound) {
			fmt.Fprintln(cmd.ErrOrStderr(), "note: install Node.js for enhanced narrative output — falling back to basic format")
		}
		// Fall back to the built-in Go renderer.
		printNarrative(cmd, pkg)
		return nil
	}
	fmt.Fprint(cmd.OutOrStdout(), text)
	return nil
}

func runExplain(cmd *cobra.Command, args []string) error {
	pkg, err := retrieve(cmd, args)
	if err != nil {
		return err
	}
	return emitJSON(cmd, pkg)
}

// printNarrative renders a ContextPackage as a readable narrative answer.
func printNarrative(cmd *cobra.Command, pkg *domain.ContextPackage) {
	w := cmd.OutOrStdout()

	// Heading.
	if len(pkg.Entities) > 0 {
		names := make([]string, 0, len(pkg.Entities))
		for _, e := range pkg.Entities {
			names = append(names, e.CanonicalName)
		}
		fmt.Fprintf(w, "%s\n%s\n\n", strings.Join(names, ", "), strings.Repeat("─", 40))
	} else {
		fmt.Fprintf(w, "%s\n%s\n\n", pkg.Query, strings.Repeat("─", 40))
	}

	if len(pkg.Observations) == 0 {
		fmt.Fprintln(w, "No evidence found for this query.")
		return
	}

	// Sort chronologically.
	obs := make([]domain.Observation, len(pkg.Observations))
	copy(obs, pkg.Observations)
	sortByDate(obs)

	// Pick the lead using entity names for scoring.
	entityNames := make([]string, 0, len(pkg.Entities))
	for _, e := range pkg.Entities {
		entityNames = append(entityNames, e.CanonicalName)
	}
	lead, rest := pickLead(obs, entityNames)
	if lead != nil {
		fmt.Fprintf(w, "%s\n\n", wordWrap(cleanContent(lead.Content), 72))
	}

	// Group remaining observations by date. Undated go into a "" bucket.
	type group struct {
		date string
		obs  []domain.Observation
	}
	var groups []group
	dateIndex := map[string]int{}

	for _, o := range rest {
		d := obsDateStr(o)
		idx, ok := dateIndex[d]
		if !ok {
			idx = len(groups)
			groups = append(groups, group{date: d})
			dateIndex[d] = idx
		}
		groups[idx].obs = append(groups[idx].obs, o)
	}

	seenContent := map[string]bool{}
	if lead != nil {
		seenContent[cleanContent(lead.Content)] = true
	}

	for _, g := range groups {
		printed := 0
		var buf []string
		for _, o := range g.obs {
			cleaned := cleanContent(o.Content)
			line := firstSentence(cleaned, 140)
			// Skip short fragments and near-duplicates.
			if len([]rune(line)) < 40 {
				continue
			}
			if isDuplicate(line, seenContent) {
				continue
			}
			seenContent[cleaned] = true
			buf = append(buf, line)
			printed++
			if printed >= 5 {
				break
			}
		}
		if len(buf) == 0 {
			continue
		}
		if g.date != "" {
			fmt.Fprintf(w, "%s\n", g.date)
		}
		for _, line := range buf {
			fmt.Fprintf(w, "  • %s\n", line)
		}
		fmt.Fprintln(w)
	}

	// Single tension, compact — strip the trailing ellipsis from pre-truncated halves.
	if len(pkg.Contradictions) > 0 {
		c := pkg.Contradictions[0]
		parts := strings.SplitN(c.Summary, "  ↔  ", 2)
		if len(parts) == 2 {
			a := strings.TrimRight(firstSentence(cleanContent(parts[0]), 180), "…")
			b := strings.TrimRight(firstSentence(cleanContent(parts[1]), 180), "…")
			fmt.Fprintf(w, "⚡ %s\n   → %s\n\n", strings.TrimSpace(a), strings.TrimSpace(b))
		}
	}

	// Footer.
	earliest, latest := dateRange(pkg.Timeline)
	if earliest != "" && earliest != latest {
		fmt.Fprintf(w, "Based on %d observation(s) · %s → %s\n", len(obs), earliest, latest)
	} else if earliest != "" {
		fmt.Fprintf(w, "Based on %d observation(s) · %s\n", len(obs), earliest)
	} else {
		fmt.Fprintf(w, "Based on %d observation(s)\n", len(obs))
	}
}

// pickLead selects the best "lead" observation. It scores each observation and
// picks the highest scorer. Scoring criteria:
//   - Must be dated
//   - Penalise UUID-heavy content (raw IDs leaked from metadata)
//   - Prefer content that mentions a resolved entity name
//   - Prefer moderate length (sweet spot 80–400 runes after cleaning)
func pickLead(obs []domain.Observation, entityNames []string) (*domain.Observation, []domain.Observation) {
	bestIdx := -1
	bestScore := -1.0

	for i, o := range obs {
		if obsTime(o) == nil {
			continue
		}
		cleaned := cleanContent(o.Content)
		runes := []rune(cleaned)
		l := len(runes)
		if l < 40 {
			continue
		}

		// Penalise UUID-heavy content.
		if uuidDensity(cleaned) > 0.15 {
			continue
		}

		score := 0.0

		// Length sweet spot: peak at 200, taper off beyond 400.
		if l <= 200 {
			score += float64(l) / 200.0
		} else if l <= 400 {
			score += 1.0
		} else {
			score += 1.0 - float64(l-400)/800.0
		}

		// Bonus if the observation explicitly names a resolved entity.
		lower := strings.ToLower(cleaned)
		for _, name := range entityNames {
			if strings.Contains(lower, strings.ToLower(name)) {
				score += 0.5
				break
			}
		}

		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}

	if bestIdx == -1 {
		return nil, obs
	}
	lead := obs[bestIdx]
	rest := make([]domain.Observation, 0, len(obs)-1)
	for i, o := range obs {
		if i != bestIdx {
			rest = append(rest, o)
		}
	}
	return &lead, rest
}

// uuidDensity returns the fraction of whitespace-separated tokens in s that
// look like UUIDs or long hex strings, as a rough "this is metadata noise" signal.
func uuidDensity(s string) float64 {
	tokens := strings.Fields(s)
	if len(tokens) == 0 {
		return 0
	}
	uuidRe := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12},?$`)
	count := 0
	for _, t := range tokens {
		if uuidRe.MatchString(strings.ToLower(t)) {
			count++
		}
	}
	return float64(count) / float64(len(tokens))
}

// obsDateStr returns the observation's date as "2006-01-02", or "" if undated.
func obsDateStr(o domain.Observation) string {
	t := obsTime(o)
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}

// isDuplicate reports whether line is semantically covered by an already-seen
// entry — exact match or one is a prefix of the other.
func isDuplicate(line string, seen map[string]bool) bool {
	if seen[line] {
		return true
	}
	lineLower := strings.ToLower(line)
	for s := range seen {
		sLower := strings.ToLower(s)
		if strings.HasPrefix(sLower, lineLower) || strings.HasPrefix(lineLower, sLower) {
			return true
		}
	}
	return false
}

// wordWrap breaks s into lines of at most width runes at word boundaries.
func wordWrap(s string, width int) string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return s
	}
	var lines []string
	current := words[0]
	for _, w := range words[1:] {
		if len([]rune(current))+1+len([]rune(w)) > width {
			lines = append(lines, current)
			current = w
		} else {
			current += " " + w
		}
	}
	lines = append(lines, current)
	return strings.Join(lines, "\n")
}

// dateRange returns the earliest and latest date strings from timeline events.
func dateRange(events []domain.TimelineEvent) (earliest, latest string) {
	for _, ev := range events {
		var ds string
		if ev.Temporal.OccurredAt != nil {
			ds = ev.Temporal.OccurredAt.Format("2006-01-02")
		} else if ev.Temporal.ObservedAt != nil {
			ds = ev.Temporal.ObservedAt.Format("2006-01-02")
		}
		if ds == "" {
			continue
		}
		if earliest == "" || ds < earliest {
			earliest = ds
		}
		if ds > latest {
			latest = ds
		}
	}
	return
}
// emitJSON serialises the context package as indented JSON for AI consumption.
// All observation content is included in full — no truncation.
func emitJSON(cmd *cobra.Command, pkg *domain.ContextPackage) error {
	type obsJSON struct {
		Content    string  `json:"content"`
		OccurredAt *string `json:"occurred_at,omitempty"`
		SourceID   string  `json:"source_id,omitempty"`
	}
	type entityJSON struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	type contradictionJSON struct {
		Summary string `json:"summary"`
	}
	type dateRangeJSON struct {
		Earliest string `json:"earliest,omitempty"`
		Latest   string `json:"latest,omitempty"`
	}
	type output struct {
		Query          string              `json:"query"`
		Entities       []entityJSON        `json:"entities"`
		Observations   []obsJSON           `json:"observations"`
		Contradictions []contradictionJSON `json:"contradictions,omitempty"`
		Confidence     float64             `json:"confidence"`
		DateRange      dateRangeJSON       `json:"date_range,omitempty"`
	}

	obs := make([]domain.Observation, len(pkg.Observations))
	copy(obs, pkg.Observations)
	sortByDate(obs)

	obsOut := make([]obsJSON, 0, len(obs))
	for _, o := range obs {
		var ts *string
		if t := obsTime(o); t != nil {
			s := t.Format(time.RFC3339)
			ts = &s
		}
		obsOut = append(obsOut, obsJSON{
			Content:    o.Content,
			OccurredAt: ts,
			SourceID:   o.SourceID,
		})
	}

	entOut := make([]entityJSON, 0, len(pkg.Entities))
	for _, e := range pkg.Entities {
		entOut = append(entOut, entityJSON{Name: e.CanonicalName, Type: e.Type})
	}

	conOut := make([]contradictionJSON, 0, len(pkg.Contradictions))
	for _, c := range pkg.Contradictions {
		conOut = append(conOut, contradictionJSON{Summary: c.Summary})
	}

	earliestDate, latestDate := dateRange(pkg.Timeline)

	out := output{
		Query:          pkg.Query,
		Entities:       entOut,
		Observations:   obsOut,
		Contradictions: conOut,
		Confidence:     pkg.Confidence,
		DateRange:      dateRangeJSON{Earliest: earliestDate, Latest: latestDate},
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func sortByDate(obs []domain.Observation) {
	for i := 1; i < len(obs); i++ {
		for j := i; j > 0; j-- {
			ai := obsTime(obs[j-1])
			aj := obsTime(obs[j])
			if ai == nil || (aj != nil && aj.Before(*ai)) {
				obs[j-1], obs[j] = obs[j], obs[j-1]
			} else {
				break
			}
		}
	}
}

func obsTime(o domain.Observation) *time.Time {
	if o.Temporal.OccurredAt != nil {
		return o.Temporal.OccurredAt
	}
	return o.Temporal.ObservedAt
}

// fieldPrefixRe matches leading field-name prefixes produced by buildContent.
// Handles both "fieldname: " (snake_case) and "Title Case Label: " patterns.
// No length cap needed for snake_case — underscores only, no spaces.
// Title-case multi-word labels are capped at 3 words to avoid false positives.
var fieldPrefixRe = regexp.MustCompile(`(?m)^(?:\w[\w_]*|(?:[A-Za-z][a-z]+ ){1,3}[A-Za-z][a-z]+):\s*`)

// isoTimestampRe matches ISO-8601 timestamp tokens embedded in content.
var isoTimestampRe = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z?\b`)

// commaNoiseRe collapses 2+ consecutive commas/semicolons from empty-array concatenation.
var commaNoiseRe = regexp.MustCompile(`(?:[,;]\s*){2,}`)

// cleanContent removes field-name prefixes that buildContent injects and
// normalises the result to plain single-line prose.
func cleanContent(s string) string {
	cleaned := fieldPrefixRe.ReplaceAllString(s, "")
	cleaned = isoTimestampRe.ReplaceAllString(cleaned, "")
	cleaned = commaNoiseRe.ReplaceAllString(cleaned, "")
	cleaned = strings.ReplaceAll(cleaned, "\n", " ")
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	return cleaned
}

// firstSentence returns up to maxRunes runes, stopping at the first sentence
// boundary if one exists within the limit, otherwise truncating.
// A '.' is only treated as a sentence end when followed by a space or end of
// string — this prevents breaking on 'finnhub.ts', '0.5', '[1aab2424]' etc.
func firstSentence(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	for i, r := range runes[:maxRunes] {
		if i <= 20 {
			continue
		}
		switch r {
		case '!', '?':
			return string(runes[:i+1])
		case '.':
			// Only a sentence boundary when followed by whitespace or end.
			if i+1 >= len(runes) || runes[i+1] == ' ' || runes[i+1] == '\n' {
				return string(runes[:i+1])
			}
		}
	}
	return string(runes[:maxRunes]) + "…"
}

// retrieve runs the full retrieval pipeline and returns the ContextPackage.
func retrieve(cmd *cobra.Command, args []string) (*domain.ContextPackage, error) {
	question := strings.Join(args, " ")
	ctx := cmd.Context()

	db, cfg, err := openStore(cmd)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer db.Close()

	proj, err := db.GetProjectByName(ctx, cfg.Project)
	if err != nil {
		return nil, fmt.Errorf("query: project %q not found; run \"loar project use %s\" first", cfg.Project, cfg.Project)
	}

	engine := retrieval.New(db)
	return engine.Query(ctx, proj.ID, question)
}

