package main

import (
	"testing"
	"time"
)

func TestSummarizeDurations(t *testing.T) {
	stats := summarize([]float64{4, 1, 3, 2})
	if stats.Mean != 2.5 || stats.Median != 2.5 || stats.P95 != 3.8499999999999996 || stats.Min != 1 || stats.Max != 4 {
		t.Fatalf("unexpected duration summary: %#v", stats)
	}
	if stats.CV <= 0 {
		t.Fatalf("expected non-zero coefficient of variation, got %#v", stats)
	}
}

func TestSyntheticEditorScenarioExercisesTraceGates(t *testing.T) {
	root := t.TempDir()
	files, err := writeFixture(root, 10)
	if err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	h, err := initialiseHandler(root, 10*time.Second)
	if err != nil {
		t.Fatalf("initialize handler: %v", err)
	}
	measurements, err := (scenario{handler: h, root: root, files: files}).run(10 * time.Second)
	if err != nil {
		t.Fatalf("run scenario: %v", err)
	}

	report := latencyReport{}
	report.Measurements.ColdStart = summarize([]float64{1})
	report.Measurements.CacheReuseSave = summarize(measurements["cache_reuse"])
	report.Measurements.BodyEdit = summarize(measurements["body_edit"])
	report.Measurements.DependencyEdit = summarize(measurements["dependency_edit"])
	report.Measurements.Cancellation = summarize(measurements["cancellation"])
	report.Measurements.StaleResultDrop = summarize(measurements["stale_drop"])
	report.Measurements.FullFallbackEdit = summarize(measurements["full_fallback"])
	report.Accounting = h.EditorTrace()
	report.Gates.MaxColdStartMs = 5000
	report.Gates.MaxEditMs = 1000
	report.Gates.MaxCancelMs = 250
	if failures := validateReport(report); len(failures) != 0 {
		t.Fatalf("expected synthetic editor trace gates to pass, got %v", failures)
	}
}
