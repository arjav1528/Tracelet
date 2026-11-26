package trace

import (
	"sort"
	"strconv"
)

type TreeRow struct {
	Span         SpanSummary `json:"span"`
	Depth        int         `json:"depth"`
	ParentSpanID string      `json:"display_parent_span_id,omitempty"`
	HasChildren  bool        `json:"has_children"`
}

type Tree struct {
	Rows              []TreeRow     `json:"rows"`
	Warnings          []TreeWarning `json:"warnings"`
	StartTimeUnixNano string        `json:"start_time_unix_nano"`
	EndTimeUnixNano   string        `json:"end_time_unix_nano"`
	DurationNano      string        `json:"duration_nano"`
}

// BuildTree requires all metadata pages for one revision before classifying missing parents.
func BuildTree(input []SpanSummary) Tree {
	result := Tree{Rows: []TreeRow{}, Warnings: []TreeWarning{}}
	spans := append([]SpanSummary(nil), input...)
	sort.SliceStable(spans, func(i, j int) bool {
		a, _ := strconv.ParseInt(spans[i].StartTimeUnixNano, 10, 64)
		b, _ := strconv.ParseInt(spans[j].StartTimeUnixNano, 10, 64)
		if a != b {
			return a < b
		}
		if spans[i].SpanID != spans[j].SpanID {
			return spans[i].SpanID < spans[j].SpanID
		}
		return spans[i].TraceID < spans[j].TraceID
	})
	n := len(spans)
	parent := make([]int, n)
	starts := make([]int64, n)
	ends := make([]int64, n)
	valid := make([]bool, n)
	indices := make(map[string]int, n)
	warn := func(code string, i int, message string) {
		result.Warnings = append(result.Warnings, TreeWarning{Code: code, SpanID: spans[i].SpanID, Message: message})
	}
	var first, last int64
	hasTime := false
	for i, span := range spans {
		parent[i] = -1
		key := span.TraceID + "\x00" + span.SpanID
		if _, exists := indices[key]; exists {
			warn("duplicate_span", i, "Duplicate span metadata is displayed separately.")
		} else {
			indices[key] = i
		}
		start, e1 := strconv.ParseInt(span.StartTimeUnixNano, 10, 64)
		end, e2 := strconv.ParseInt(span.EndTimeUnixNano, 10, 64)
		if e1 != nil || e2 != nil || start < 0 || end < start {
			warn("invalid_time", i, "Span timing metadata is invalid.")
			continue
		}
		valid[i] = true
		starts[i] = start
		ends[i] = end
		if !hasTime || start < first {
			first = start
		}
		if !hasTime || end > last {
			last = end
		}
		hasTime = true
	}
	hasRoot := false
	for i, span := range spans {
		if span.ParentSpanID == "" {
			hasRoot = true
			continue
		}
		if p, ok := indices[span.TraceID+"\x00"+span.ParentSpanID]; ok {
			parent[i] = p
		} else {
			warn("missing_parent", i, "Parent span was not received.")
		}
	}
	if n > 0 && !hasRoot {
		result.Warnings = append(result.Warnings, TreeWarning{Code: "missing_root", Message: "No root span was received."})
	}
	colors := make([]uint8, n)
	for i := range spans {
		if colors[i] != 0 {
			continue
		}
		chain := []int{}
		current := i
		for current >= 0 && colors[current] == 0 {
			colors[current] = 1
			chain = append(chain, current)
			current = parent[current]
		}
		if current >= 0 && colors[current] == 1 {
			cut := current
			for node := parent[current]; node != current; node = parent[node] {
				if node < cut {
					cut = node
				}
			}
			node := current
			for {
				warn("cycle", node, "Cyclic parent relationship was broken for display.")
				node = parent[node]
				if node == current {
					break
				}
			}
			parent[cut] = -1
		}
		for _, node := range chain {
			colors[node] = 2
		}
	}
	children := make([][]int, n)
	roots := []int{}
	for i, p := range parent {
		if p < 0 {
			roots = append(roots, i)
			continue
		}
		children[p] = append(children[p], i)
		if valid[i] && valid[p] && (starts[i] < starts[p] || ends[i] > ends[p]) {
			warn("time_skew", i, "Span interval extends beyond its parent interval.")
		}
	}
	type frame struct{ index, depth int }
	stack := make([]frame, 0, n)
	for i := len(roots) - 1; i >= 0; i-- {
		stack = append(stack, frame{roots[i], 0})
	}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		row := TreeRow{Span: spans[current.index], Depth: current.depth, HasChildren: len(children[current.index]) > 0}
		if p := parent[current.index]; p >= 0 {
			row.ParentSpanID = spans[p].SpanID
		}
		result.Rows = append(result.Rows, row)
		descendants := children[current.index]
		for i := len(descendants) - 1; i >= 0; i-- {
			stack = append(stack, frame{descendants[i], current.depth + 1})
		}
	}
	if hasTime {
		result.StartTimeUnixNano = strconv.FormatInt(first, 10)
		result.EndTimeUnixNano = strconv.FormatInt(last, 10)
		result.DurationNano = strconv.FormatInt(last-first, 10)
	}
	return result
}
