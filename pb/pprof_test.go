package pb

import (
	"math"
	"testing"
)

func TestAnalyzeCPUProfile(t *testing.T) {
	// Create a sample profile
	profile := &Profile{
		StringTable: []string{"", "samples", "cpu", "nanoseconds", "main", "foo", "bar"},
		SampleType: []*ValueType{
			{Type: 1, Unit: 2}, // samples
			{Type: 2, Unit: 3}, // cpu, nanoseconds
		},
		Function: []*Function{
			{Id: 1, Name: 4}, // main
			{Id: 2, Name: 5}, // foo
			{Id: 3, Name: 6}, // bar
		},
		Location: []*Location{
			{Id: 1, Line: []*Line{{FunctionId: 1}}},
			{Id: 2, Line: []*Line{{FunctionId: 2}}},
			{Id: 3, Line: []*Line{{FunctionId: 3}}},
		},
		Sample: []*Sample{
			{
				LocationId: []uint64{1, 2},         // main->foo
				Value:      []int64{13, 130000000}, // 13 samples, 130ms CPU time
			},
			{
				LocationId: []uint64{1, 3},       // main->bar
				Value:      []int64{7, 70000000}, // 7 samples, 70ms CPU time
			},
		},
	}

	nodes, err := AnalyzeCPUProfile(profile, false)
	if err != nil {
		t.Fatalf("AnalyzeCPUProfile failed: %v", err)
	}

	// Verify main function
	main := nodes["main"]
	if main == nil {
		t.Fatal("main function not found")
	}
	if main.TotalCPU != 100.0 {
		t.Errorf("Expected main total CPU 100%%, got %.2f%%", main.TotalCPU)
	}

	// Verify foo function
	foo := nodes["foo"]
	if foo == nil {
		t.Fatal("foo function not found")
	}
	expectedFooCPU := (130000000.0 / 200000000.0) * 100 // 130ms / 200ms total
	if !almostEqual(foo.TotalCPU, expectedFooCPU, 0.01) {
		t.Errorf("Expected foo CPU %.2f%%, got %.2f%%", expectedFooCPU, foo.TotalCPU)
	}

	// Verify bar function
	bar := nodes["bar"]
	if bar == nil {
		t.Fatal("bar function not found")
	}
	expectedBarCPU := (70000000.0 / 200000000.0) * 100 // 70ms / 200ms total
	if !almostEqual(bar.TotalCPU, expectedBarCPU, 0.01) {
		t.Errorf("Expected bar CPU %.2f%%, got %.2f%%", expectedBarCPU, bar.TotalCPU)
	}
}

func almostEqual(a, b, tolerance float64) bool {
	return math.Abs(a-b) <= tolerance
}

func TestAnalyzeCPUProfileWithEmptySamples(t *testing.T) {
	profile := &Profile{
		StringTable: []string{"", "cpu", "nanoseconds"},
		SampleType: []*ValueType{
			{Type: 1, Unit: 2}, // cpu, nanoseconds
		},
		Sample: []*Sample{},
	}

	_, err := AnalyzeCPUProfile(profile, false)
	if err == nil {
		t.Error("Expected error for empty samples, got nil")
	}
}

func TestAnalyzeCPUProfileWithNilProfile(t *testing.T) {
	_, err := AnalyzeCPUProfile(nil, false)
	if err == nil {
		t.Error("Expected error for nil profile, got nil")
	}
}

func TestAnalyzeCPUProfileWithInvalidSampleType(t *testing.T) {
	profile := &Profile{
		StringTable: []string{"", "memory", "bytes"},
		SampleType: []*ValueType{
			{Type: 1, Unit: 2}, // memory, bytes
		},
	}

	_, err := AnalyzeCPUProfile(profile, false)
	if err == nil {
		t.Error("Expected error for non-CPU profile, got nil")
	}
}
