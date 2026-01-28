package backlight

import (
	"log"
	"os"
	"strconv"
	"strings"
	"testing"
)

// newTestManager creates a Manager with default thresholds pointing at a temp file.
// Uses the same defaults as config.go.
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	tmp := t.TempDir() + "/brightness"
	os.WriteFile(tmp, []byte("9700"), 0644) // MID brightness

	logger := log.New(os.Stderr, "test: ", 0)
	return New(
		tmp, logger,
		9350,  // veryLow
		9500,  // low
		9700,  // mid
		9950,  // high
		10240, // veryHigh
		8,     // veryLow→low
		18,    // low→mid
		40,    // mid→high
		80,    // high→veryHigh
		5,     // low→veryLow
		15,    // mid→low
		35,    // high→mid
		70,    // veryHigh→high
	)
}

func TestInitialStateFromHardware(t *testing.T) {
	tests := []struct {
		name       string
		brightness int
		want       BrightnessLevel
	}{
		{"very low brightness", 9350, LevelVeryLow},
		{"low brightness", 9500, LevelLow},
		{"mid brightness", 9700, LevelMid},
		{"high brightness", 9950, LevelHigh},
		{"very high brightness", 10240, LevelVeryHigh},
		{"between low and mid", 9600, LevelLow},
		{"between mid and high", 9800, LevelMid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir() + "/brightness"
			os.WriteFile(tmp, []byte(strconv.Itoa(tt.brightness)), 0644)

			logger := log.New(os.Stderr, "test: ", 0)
			m := New(tmp, logger,
				9350, 9500, 9700, 9950, 10240,
				8, 18, 40, 80,
				5, 15, 35, 70,
			)

			if m.GetCurrentLevel() != tt.want {
				t.Errorf("got %s, want %s", m.GetCurrentLevel(), tt.want)
			}
		})
	}
}

func TestInitialStateFallback(t *testing.T) {
	logger := log.New(os.Stderr, "test: ", 0)
	m := New("/nonexistent/path", logger,
		9350, 9500, 9700, 9950, 10240,
		8, 18, 40, 80,
		5, 15, 35, 70,
	)

	if m.GetCurrentLevel() != LevelMid {
		t.Errorf("expected MID fallback, got %s", m.GetCurrentLevel())
	}
}

func TestUpwardTransitions(t *testing.T) {
	tests := []struct {
		name       string
		start      BrightnessLevel
		illuminance int
		want       BrightnessLevel
	}{
		{"veryLow to low", LevelVeryLow, 9, LevelLow},
		{"low to mid", LevelLow, 19, LevelMid},
		{"mid to high", LevelMid, 41, LevelHigh},
		{"high to veryHigh", LevelHigh, 81, LevelVeryHigh},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestManager(t)
			m.SetCurrentLevel(tt.start)
			if err := m.AdjustBacklight(tt.illuminance); err != nil {
				t.Fatal(err)
			}
			if m.GetCurrentLevel() != tt.want {
				t.Errorf("got %s, want %s", m.GetCurrentLevel(), tt.want)
			}
		})
	}
}

func TestDownwardTransitions(t *testing.T) {
	tests := []struct {
		name       string
		start      BrightnessLevel
		illuminance int
		want       BrightnessLevel
	}{
		{"low to veryLow", LevelLow, 4, LevelVeryLow},
		{"mid to low", LevelMid, 14, LevelLow},
		{"high to mid", LevelHigh, 34, LevelMid},
		{"veryHigh to high", LevelVeryHigh, 69, LevelHigh},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestManager(t)
			m.SetCurrentLevel(tt.start)
			if err := m.AdjustBacklight(tt.illuminance); err != nil {
				t.Fatal(err)
			}
			if m.GetCurrentLevel() != tt.want {
				t.Errorf("got %s, want %s", m.GetCurrentLevel(), tt.want)
			}
		})
	}
}

func TestHysteresisNoTransition(t *testing.T) {
	tests := []struct {
		name        string
		start       BrightnessLevel
		illuminance int
	}{
		{"mid stays at exact up threshold", LevelMid, 40},
		{"mid stays at exact down threshold", LevelMid, 15},
		{"mid stays in dead zone", LevelMid, 25},
		{"low stays between thresholds", LevelLow, 10},
		{"high stays between thresholds", LevelHigh, 50},
		{"veryLow stays below up threshold", LevelVeryLow, 8},
		{"veryHigh stays above down threshold", LevelVeryHigh, 70},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestManager(t)
			m.SetCurrentLevel(tt.start)
			if err := m.AdjustBacklight(tt.illuminance); err != nil {
				t.Fatal(err)
			}
			if m.GetCurrentLevel() != tt.start {
				t.Errorf("expected no transition from %s, got %s", tt.start, m.GetCurrentLevel())
			}
		})
	}
}

func TestNoFileWriteWithoutTransition(t *testing.T) {
	m := newTestManager(t)
	m.SetCurrentLevel(LevelMid)

	// Write a known value to the file
	os.WriteFile(m.backlightPath, []byte("12345"), 0644)

	// Adjust with value in dead zone — should not write
	if err := m.AdjustBacklight(25); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(m.backlightPath)
	if strings.TrimSpace(string(data)) != "12345" {
		t.Errorf("file was written when no transition occurred: got %q", string(data))
	}
}

func TestFileWriteOnTransition(t *testing.T) {
	m := newTestManager(t)
	m.SetCurrentLevel(LevelMid)

	if err := m.AdjustBacklight(41); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(m.backlightPath)
	val, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	if val != 9950 { // HIGH brightness
		t.Errorf("expected 9950, got %d", val)
	}
}

func TestOscillationStability(t *testing.T) {
	m := newTestManager(t)
	m.SetCurrentLevel(LevelMid)

	// Feed alternating values near the mid→high boundary (threshold=40)
	// and the mid→low boundary (threshold=15)
	// Values within the dead zone should not cause transitions.
	values := []int{38, 16, 39, 17, 38, 16}
	for _, v := range values {
		if err := m.AdjustBacklight(v); err != nil {
			t.Fatal(err)
		}
		if m.GetCurrentLevel() != LevelMid {
			t.Fatalf("unexpected transition to %s at illuminance %d", m.GetCurrentLevel(), v)
		}
	}
}

func TestClosestLevel(t *testing.T) {
	m := newTestManager(t)

	tests := []struct {
		brightness int
		want       BrightnessLevel
	}{
		{9350, LevelVeryLow},
		{9425, LevelVeryLow}, // midpoint between 9350 and 9500 → closer to veryLow
		{9426, LevelLow},     // just past midpoint
		{9700, LevelMid},
		{10240, LevelVeryHigh},
		{0, LevelVeryLow},
		{99999, LevelVeryHigh},
	}

	for _, tt := range tests {
		got := m.closestLevel(tt.brightness)
		if got != tt.want {
			t.Errorf("closestLevel(%d) = %s, want %s", tt.brightness, got, tt.want)
		}
	}
}

func TestBrightnessLevelString(t *testing.T) {
	tests := []struct {
		level BrightnessLevel
		want  string
	}{
		{LevelVeryLow, "VERY_LOW"},
		{LevelLow, "LOW"},
		{LevelMid, "MID"},
		{LevelHigh, "HIGH"},
		{LevelVeryHigh, "VERY_HIGH"},
		{BrightnessLevel(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		if got := tt.level.String(); got != tt.want {
			t.Errorf("BrightnessLevel(%d).String() = %q, want %q", tt.level, got, tt.want)
		}
	}
}
