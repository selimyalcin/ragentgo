package tabular

import (
	"fmt"
	"math"
	"strings"

	"github.com/selimyalcin/ragentgo/document"
)

const (
	promptExampleRows = 8
	sourceRows        = 40
)

// Result is a deterministic aggregate over retrieved labeled rows.
type Result struct {
	Op       Op
	Field    string
	Filters  []string
	RowCount int
	Value    float64
	Hits     []document.SearchResult
}

// Try aggregates labeled table rows when the question asks for a total,
// count, average, min, or max. It returns nil when the question is a
// lookup or the rows do not share a clear numeric field.
func Try(question string, hits []document.SearchResult) *Result {
	op, ok := Detect(question)
	if !ok || len(hits) == 0 {
		return nil
	}
	records := mergeRecords(hits)
	if len(records) == 0 {
		return nil
	}
	qTokens := unique(tokenize(question))
	filters := inferFilters(qTokens, records)
	matched := applyFilters(records, filters)
	if len(matched) == 0 {
		return nil
	}
	field := ""
	if op != OpCount {
		field = pickMeasure(qTokens, matched)
		if field == "" {
			return nil
		}
	}
	values := make([]float64, 0, len(matched))
	used := make([]document.SearchResult, 0, len(matched))
	for _, rec := range matched {
		if op == OpCount {
			used = append(used, rec.hit)
			continue
		}
		raw, ok := rec.fields[field]
		if !ok {
			continue
		}
		n, ok := ParseAmount(raw)
		if !ok {
			continue
		}
		values = append(values, n)
		used = append(used, rec.hit)
	}
	if op == OpCount {
		return &Result{
			Op:       op,
			Filters:  formatFilters(filters),
			RowCount: len(used),
			Value:    float64(len(used)),
			Hits:     used,
		}
	}
	if len(values) == 0 {
		return nil
	}
	return &Result{
		Op:       op,
		Field:    field,
		Filters:  formatFilters(filters),
		RowCount: len(values),
		Value:    reduce(op, values),
		Hits:     used,
	}
}

// PromptHits are a short sample so the LLM can cite rows without re-summing.
func (r *Result) PromptHits() []document.SearchResult {
	if r == nil || len(r.Hits) <= promptExampleRows {
		if r == nil {
			return nil
		}
		return r.Hits
	}
	return r.Hits[:promptExampleRows]
}

// SourceHits are the matching rows returned to the API caller.
func (r *Result) SourceHits() []document.SearchResult {
	if r == nil {
		return nil
	}
	if len(r.Hits) <= sourceRows {
		return r.Hits
	}
	return r.Hits[:sourceRows]
}

// Block is an authoritative numeric result injected above retrieved context.
func (r *Result) Block() string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("COMPUTED RESULT (authoritative; do not recalculate from a subset of rows):\n")
	fmt.Fprintf(&b, "- operation: %s\n", r.Op)
	if r.Field != "" {
		fmt.Fprintf(&b, "- field: %s\n", r.Field)
	}
	if len(r.Filters) > 0 {
		fmt.Fprintf(&b, "- filters: %s\n", strings.Join(r.Filters, "; "))
	}
	fmt.Fprintf(&b, "- rows_used: %d\n", r.RowCount)
	fmt.Fprintf(&b, "- value: %s\n", formatValue(r.Value))
	return b.String()
}

type record struct {
	hit    document.SearchResult
	fields map[string]string
	docID  string
}

func mergeRecords(hits []document.SearchResult) []record {
	byID := map[string]*record{}
	order := make([]string, 0, len(hits))
	for _, hit := range hits {
		id := hit.Chunk.DocumentID
		if id == "" {
			id = hit.Chunk.ID
		}
		fields := Fields(hit.Chunk.Content)
		if len(fields) == 0 {
			continue
		}
		if rec, ok := byID[id]; ok {
			for k, v := range fields {
				if v != "" {
					rec.fields[k] = v
				}
			}
			continue
		}
		rec := &record{hit: hit, fields: fields, docID: id}
		byID[id] = rec
		order = append(order, id)
	}
	out := make([]record, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out
}

func inferFilters(qTokens []string, records []record) map[string]string {
	filters := map[string]string{}
	for _, tok := range qTokens {
		if _, skip := stopTokens[tok]; skip {
			continue
		}
		if _, skip := opTokens[tok]; skip {
			continue
		}
		if _, measure := measureTokens[tok]; measure {
			continue
		}
		field, val, ok := bestValueMatch(tok, records)
		if !ok {
			continue
		}
		filters[field] = val
	}
	return filters
}

func bestValueMatch(tok string, records []record) (field, value string, ok bool) {
	type cand struct {
		field, value string
		score        int
	}
	best := cand{}
	seen := false
	for _, rec := range records {
		for k, v := range rec.fields {
			s := valueScore(tok, k, v)
			if s <= 0 {
				continue
			}
			if !seen || s > best.score {
				best = cand{field: k, value: v, score: s}
				seen = true
			}
		}
	}
	return best.field, best.value, seen
}

func valueScore(tok, key, val string) int {
	if fold(key) == "record" {
		return 0
	}
	score := 0
	fv := fold(val)
	if fv == tok {
		score = 8
	} else {
		hit := false
		for _, w := range tokenize(val) {
			if w == tok {
				hit = true
				break
			}
		}
		if !hit {
			return 0
		}
		score = 5
	}
	fk := fold(key)
	switch {
	case fk == "ay" || fk == "month" || strings.Contains(fk, "donem") || strings.Contains(fk, "period"):
		score += 4
	case strings.Contains(fk, "tarih") || strings.Contains(fk, "date"):
		score += 2
	}
	if _, isMonth := monthTokens[tok]; isMonth {
		score += 3
	}
	return score
}

func applyFilters(records []record, filters map[string]string) []record {
	if len(filters) == 0 {
		return records
	}
	out := make([]record, 0, len(records))
	for _, rec := range records {
		if matchFilters(rec.fields, filters) {
			out = append(out, rec)
		}
	}
	return out
}

func matchFilters(fields map[string]string, filters map[string]string) bool {
	for k, want := range filters {
		got, ok := fields[k]
		if !ok {
			return false
		}
		if fold(got) != fold(want) && !containsToken(got, fold(want)) {
			return false
		}
	}
	return true
}

func containsToken(val, tok string) bool {
	for _, w := range tokenize(val) {
		if w == tok {
			return true
		}
	}
	return false
}

func pickMeasure(qTokens []string, records []record) string {
	wanted := make([]string, 0, len(qTokens))
	for _, tok := range qTokens {
		if _, skip := stopTokens[tok]; skip {
			continue
		}
		if _, skip := opTokens[tok]; skip {
			continue
		}
		if _, month := monthTokens[tok]; month {
			continue
		}
		wanted = append(wanted, tok)
	}
	scores := map[string]int{}
	for _, rec := range records {
		for k, v := range rec.fields {
			if _, ok := ParseAmount(v); !ok {
				continue
			}
			s := fieldScore(k, wanted)
			if s > scores[k] {
				scores[k] = s
			}
		}
	}
	best, bestScore := "", 0
	for k, s := range scores {
		if s > bestScore || (s == bestScore && (best == "" || k < best)) {
			best, bestScore = k, s
		}
	}
	if bestScore <= 0 {
		return ""
	}
	return best
}

func fieldScore(name string, wanted []string) int {
	nameToks := tokenize(name)
	fn := fold(name)
	score := 0
	for _, t := range wanted {
		matched := false
		for _, n := range nameToks {
			if n == t || strings.HasPrefix(n, t) || strings.HasPrefix(t, n) {
				score += 4
				matched = true
				break
			}
		}
		if !matched && strings.Contains(fn, t) {
			score += 2
			matched = true
		}
		if matched {
			if _, ok := measureTokens[t]; ok {
				score += 2
			}
		}
	}
	return score
}

func reduce(op Op, values []float64) float64 {
	switch op {
	case OpMin:
		n := values[0]
		for _, v := range values[1:] {
			if v < n {
				n = v
			}
		}
		return n
	case OpMax:
		n := values[0]
		for _, v := range values[1:] {
			if v > n {
				n = v
			}
		}
		return n
	case OpAvg:
		return reduce(OpSum, values) / float64(len(values))
	default:
		var s float64
		for _, v := range values {
			s += v
		}
		return s
	}
}

func formatFilters(filters map[string]string) []string {
	out := make([]string, 0, len(filters))
	for k, v := range filters {
		out = append(out, k+"="+v)
	}
	return out
}

func formatValue(n float64) string {
	if math.Abs(n-math.Round(n)) < 0.0000001 {
		return fmt.Sprintf("%.0f", math.Round(n))
	}
	return fmt.Sprintf("%.2f", n)
}
