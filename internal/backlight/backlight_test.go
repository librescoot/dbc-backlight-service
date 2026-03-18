package backlight

import (
	"log"
	"os"
	"strconv"
	"strings"
	"testing"
)

var defaultCurve = []Point{
	{0, 400},
	{0.5, 1300},
	{1, 2200},
	{2, 2900},
	{5, 4000},
	{10, 5200},
	{20, 7000},
	{35, 8600},
	{50, 9600},
	{80, 10240},
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	tmp := t.TempDir() + "/brightness"
	os.WriteFile(tmp, []byte("5000"), 0644)
	logger := log.New(os.Stderr, "test: ", 0)
	return New(tmp, logger, defaultCurve, 0.15)
}

func TestParseCurve(t *testing.T) {
	curve, err := ParseCurve("0.5:1024 2:1500 5:3000 35:10240")
	if err != nil {
		t.Fatal(err)
	}
	if len(curve) != 4 {
		t.Fatalf("expected 4 points, got %d", len(curve))
	}
	if curve[0].Lux != 0.5 || curve[0].Brightness != 1024 {
		t.Errorf("first point: got %v", curve[0])
	}
}

func TestParseCurveSorts(t *testing.T) {
	curve, err := ParseCurve("35:10240 0.5:1024 5:3000")
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
	if b := m.Interpolate(-1); b != 400 {
		t.Errorf("below min: got %d, want 400", b)
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

	// Midpoint between 0.5:1300 and 1:2200 → lux=0.75 → brightness=1750
	b := m.Interpolate(0.75)
	if b != 1750 {
		t.Errorf("midpoint 0.5-1: got %d, want 1750", b)
	}
}

func TestRampGradual(t *testing.T) {
	m := newTestManager(t)
	// Initialize with a low lux reading
	m.AdjustBacklight(1.0)

	// Now jump to bright — should ramp, not jump
	m.AdjustBacklight(200)
	if m.Output() >= 10240 {
		t.Errorf("expected gradual ramp, got instant jump to %d", m.Output())
	}
	initial := m.Output()
	if initial <= m.Interpolate(1.0) {
		t.Errorf("expected upward movement, got %d", initial)
	}
}

func TestRampConverges(t *testing.T) {
	m := newTestManager(t)
	m.AdjustBacklight(1.0) // initialize
	for i := 0; i < 200; i++ {
		m.AdjustBacklight(200)
	}
	if m.Output() != 10240 {
		t.Errorf("expected convergence to 10240, got %d", m.Output())
	}
}

func TestRampDown(t *testing.T) {
	m := newTestManager(t)
	// Initialize and converge high
	for i := 0; i < 200; i++ {
		m.AdjustBacklight(200)
	}
	peak := m.Output()
	// Now ramp down — need several ticks for EMA to converge
	for i := 0; i < 50; i++ {
		m.AdjustBacklight(0)
	}
	if m.Output() >= peak {
		t.Errorf("expected downward movement from %d, got %d", peak, m.Output())
	}
}

func TestWriteOnRamp(t *testing.T) {
	m := newTestManager(t)
	m.AdjustBacklight(1.0) // initialize
	m.AdjustBacklight(200) // ramp towards 10240

	data, _ := os.ReadFile(m.backlightPath)
	val, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	if val <= m.Interpolate(1.0) {
		t.Errorf("expected file to be updated during ramp, got %d", val)
	}
}
