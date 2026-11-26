package trace

import (
	"fmt"
	"reflect"
	"testing"
)

func treeSpan(id, parent string, start, end int64) SpanSummary {
	return SpanSummary{TraceID: "trace", SpanID: id, ParentSpanID: parent, StartTimeUnixNano: fmt.Sprint(start), EndTimeUnixNano: fmt.Sprint(end), DurationNano: fmt.Sprint(end - start)}
}

func TestBuildTreeKeepsRootsOrphansAndOverlaps(t *testing.T) {
	spans := []SpanSummary{treeSpan("c", "r", 10, 90), treeSpan("orphan", "absent", 30, 40), treeSpan("r", "", 0, 100), treeSpan("b", "r", 10, 80), treeSpan("other", "", 200, 210)}
	original := append([]SpanSummary(nil), spans...)
	tree := BuildTree(spans)
	if len(tree.Rows) != len(spans) || tree.DurationNano != "210" {
		t.Fatalf("tree=%+v", tree)
	}
	want := []string{"r", "b", "c", "orphan", "other"}
	for i, id := range want {
		if tree.Rows[i].Span.SpanID != id {
			t.Fatalf("row order=%+v", tree.Rows)
		}
	}
	if tree.Rows[1].Depth != 1 || tree.Rows[3].Depth != 0 || !tree.Rows[0].HasChildren {
		t.Fatal("wrong hierarchy")
	}
	if !reflect.DeepEqual(spans, original) {
		t.Fatal("mutated original spans")
	}
	if len(tree.Warnings) != 1 || tree.Warnings[0].Code != "missing_parent" {
		t.Fatalf("warnings=%+v", tree.Warnings)
	}
}

func TestBuildTreeBreaksCyclesDeterministically(t *testing.T) {
	spans := []SpanSummary{treeSpan("a", "b", 10, 20), treeSpan("b", "c", 10, 20), treeSpan("c", "a", 10, 20), treeSpan("self", "self", 30, 40)}
	tree := BuildTree(spans)
	if len(tree.Rows) != 4 {
		t.Fatal("cycle hid nodes")
	}
	if tree.Rows[0].Span.SpanID != "a" || tree.Rows[0].Depth != 0 || tree.Rows[0].ParentSpanID != "" {
		t.Fatalf("cycle root=%+v", tree.Rows[0])
	}
	cycles := 0
	missingRoot := false
	for _, w := range tree.Warnings {
		if w.Code == "cycle" {
			cycles++
		}
		missingRoot = missingRoot || w.Code == "missing_root"
	}
	if cycles != 4 || !missingRoot {
		t.Fatalf("cycle warnings=%+v", tree.Warnings)
	}
	reversed := []SpanSummary{spans[3], spans[2], spans[1], spans[0]}
	if !reflect.DeepEqual(tree, BuildTree(reversed)) {
		t.Fatal("cycle layout depends on arrival order")
	}
}

func TestBuildTreeDeepAndPrecise(t *testing.T) {
	const depth = 20000
	spans := make([]SpanSummary, depth)
	for i := range spans {
		parent := ""
		if i > 0 {
			parent = fmt.Sprintf("%05d", i-1)
		}
		spans[i] = treeSpan(fmt.Sprintf("%05d", i), parent, 1795679780123456789, 1795679780123456790)
	}
	tree := BuildTree(spans)
	if len(tree.Rows) != depth || tree.Rows[depth-1].Depth != depth-1 || tree.DurationNano != "1" {
		t.Fatal("deep hierarchy lost nodes or precision")
	}
}

func TestBuildTreeFlagsTimeSkewAndInvalidTime(t *testing.T) {
	invalid := treeSpan("invalid", "", 0, 1)
	invalid.StartTimeUnixNano = "not-a-time"
	tree := BuildTree([]SpanSummary{treeSpan("r", "", 10, 20), treeSpan("child", "r", 5, 30), invalid})
	codes := map[string]bool{}
	for _, w := range tree.Warnings {
		codes[w.Code] = true
	}
	if !codes["time_skew"] || !codes["invalid_time"] || len(tree.Rows) != 3 {
		t.Fatalf("warnings=%+v", tree.Warnings)
	}
}
