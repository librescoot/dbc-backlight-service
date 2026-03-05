package backlight

import (
	"log"
	"os"
	"strconv"
	"strings"
	"testing"
)

var defaultCurve = []Point{
	{0.5, 100},
	{2, 500},
	{5, 1500},
	{15, 4000},
	{35, 7000},
	{80, 10240},
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	tmp := t.TempDir() + "/brightness"
	os.WriteFile(tmp, []byte("5000"), 0644)
	logger := log.New(os.Stderr, "test: ", 0)
	return New(tmp, logger, defaultCurve, 50)
}

func TestParseCurve(t *testing.T) {
	curve, err := ParseCurve("0.5:100 2:500 5:1500 35:10240")
	if err != nil {
		t.Fatal(err)
	}
	if len(curve) != 4 {
		t.Fatalf("expected 4 points, got %d", len(curve))
	}
	if curve[0].Lux != 0.5 || curve[0].Brightness != 100 {
		t.Errorf("first point: got %v", curve[0])
	}
	if curve[3].Lux != 35 || curve[3].Brightness != 10240 {
		t.Errorf("last point: got %v", curve[3])
	}
}

func TestParseCurveSorts(t *testing.T) {
	curve, err := ParseCurve("35:10240 0.5:100 5:1500")
	if err != nil {
		t.Fatal(err)
	}
	if curve[0].Lux != 0.5 || curve[2].Lux != 35 {
		t.Errorf("curve not sorted: %v", curve)
	}
}

func TestParseCurveErrors(t *testing.T) {
	tests := []string{
		"",
		"1:100",
		"bad:100 2:200",
		"1:bad 2:200",
		"nocolon 2:200",
	}
	for _, s := range tests {
		if _, err := ParseCurve(s); err == nil {
			t.Errorf("expected error for %q", s)
		}
	}
}

func TestInterpolateBelowMin(t *testing.T) {
	m := newTestManager(t)
	if b := m.Interpolate(0); b != 100 {
		t.Errorf("below min: got %d, want 100", b)
	}
}

func TestInterpolateAboveMax(t *testing.T) {
	m := newTestManager(t)
	if b := m.Interpolate(200); b != 10240 {
		t.Errorf("above max: got %d, want 10240", b)
	}
}

func TestInterpolateExactPoints(t *testing.T) {
	m := newTestManager(t)
	for _, p := range defaultCurve {
		if b := m.Interpolate(p.Lux); b != p.Brightness {
			t.Errorf("at lux=%.1f: got %d, want %d", p.Lux, b, p.Brightness)
		}
	}
}

func TestInterpolateMidpoints(t *testing.T) {
	m := newTestManager(t)

	// Midpoint between 0.5:100 and 2:500 → lux=1.25 → brightness=300
	b := m.Interpolate(1.25)
	if b != 300 {
		t.Errorf("midpoint 0.5-2: got %d, want 300", b)
	}

	// Midpoint between 35:7000 and 80:10240 → lux=57.5 → brightness=8620
	b = m.Interpolate(57.5)
	if b != 8620 {
		t.Errorf("midpoint 35-80: got %d, want 8620", b)
	}
}

func TestAdjustWritesOnChange(t *testing.T) {
	m := newTestManager(t)
	m.current = 100

	if err := m.AdjustBacklight(35); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(m.backlightPath)
	val, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	if val != 7000 {
		t.Errorf("expected 7000, got %d", val)
	}
}

func TestAdjustSkipsSmallDelta(t *testing.T) {
	m := newTestManager(t)
	m.current = 7000

	// Write a sentinel to detect writes
	os.WriteFile(m.backlightPath, []byte("99999"), 0644)

	// Lux=35 → brightness=7000, delta=0 → should skip
	if err := m.AdjustBacklight(35); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(m.backlightPath)
	if strings.TrimSpace(string(data)) != "99999" {
		t.Error("file was written when delta was below threshold")
	}
}
