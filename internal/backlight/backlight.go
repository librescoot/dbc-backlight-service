package backlight

import (
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Point represents a lux→brightness mapping on the interpolation curve.
type Point struct {
	Lux        float64
	Brightness int
}

// ParseCurve parses a curve string of "lux:brightness" pairs.
// Example: "0.5:1024 2:1500 5:3000 35:10240"
func ParseCurve(s string) ([]Point, error) {
	fields := strings.Fields(s)
	if len(fields) < 2 {
		return nil, fmt.Errorf("curve needs at least 2 points, got %d", len(fields))
	}

	points := make([]Point, 0, len(fields))
	for _, f := range fields {
		parts := strings.SplitN(f, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid point %q (expected lux:brightness)", f)
		}
		lux, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return nil, fmt.Errorf("invalid lux value %q: %v", parts[0], err)
		}
		brightness, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid brightness value %q: %v", parts[1], err)
		}
		points = append(points, Point{Lux: lux, Brightness: brightness})
	}

	sort.Slice(points, func(i, j int) bool {
		return points[i].Lux < points[j].Lux
	})

	return points, nil
}

type Manager struct {
	logger         *log.Logger
	backlightPath  string
	curve          []Point
	output         int     // current brightness written to sysfs
	target         int     // desired brightness from interpolation
	smoothedLux    float64 // EMA-filtered lux value
	luxAlpha       float64 // EMA smoothing factor for lux input (0..1)
	rampRate       float64 // fraction of remaining distance per tick (0..1)
	targetDeadband int     // minimum brightness change to update target (anti-flicker)
	initialized    bool
}

func New(backlightPath string, logger *log.Logger, curve []Point, rampRate float64) *Manager {
	m := &Manager{
		logger:        logger,
		backlightPath: backlightPath,
		curve:         curve,
		output:        -1,
		target:        -1,
		smoothedLux:   -1,
		luxAlpha:       0.2, // smooth lux input: 20% new, 80% old
		rampRate:       rampRate,
		targetDeadband: 150, // ignore target changes smaller than this (anti-flicker)
	}

	if brightness, err := m.readBrightness(); err == nil {
		m.output = brightness
		m.target = brightness
		m.logger.Printf("Initialized from hardware brightness %d", brightness)
	} else {
		m.logger.Printf("Could not read hardware brightness: %v", err)
	}

	return m
}

// Interpolate returns the brightness for a given lux value by linearly
// interpolating between the two surrounding curve points.
func (m *Manager) Interpolate(lux float64) int {
	if lux <= m.curve[0].Lux {
		return m.curve[0].Brightness
	}
	last := m.curve[len(m.curve)-1]
	if lux >= last.Lux {
		return last.Brightness
	}

	for i := 1; i < len(m.curve); i++ {
		if lux <= m.curve[i].Lux {
			p0 := m.curve[i-1]
			p1 := m.curve[i]
			t := (lux - p0.Lux) / (p1.Lux - p0.Lux)
			b := float64(p0.Brightness) + t*float64(p1.Brightness-p0.Brightness)
			return int(math.Round(b))
		}
	}

	return last.Brightness
}

// AdjustBacklight smooths the lux input, computes a target brightness,
// then ramps the output towards it.
func (m *Manager) AdjustBacklight(lux float64) error {
	// Smooth the lux input with EMA to reject single-sample spikes
	if m.smoothedLux < 0 {
		m.smoothedLux = lux
	} else {
		m.smoothedLux = m.luxAlpha*lux + (1-m.luxAlpha)*m.smoothedLux
	}

	newTarget := m.Interpolate(m.smoothedLux)

	if !m.initialized {
		m.target = newTarget
		m.output = newTarget
		m.initialized = true
		m.logger.Printf("lux=%.1f → brightness %d (initial)", lux, m.output)
		return m.writeBrightness(m.output)
	}

	// Only update target if the change exceeds the deadband to prevent
	// oscillation from sensor noise at interpolation boundaries.
	delta := newTarget - m.target
	if delta < 0 {
		delta = -delta
	}
	if delta > m.targetDeadband {
		m.target = newTarget
	}

	if m.target == m.output {
		return nil
	}

	// Ramp towards target
	diff := float64(m.target - m.output)
	step := int(math.Round(diff * m.rampRate))

	// Snap to target when close enough (avoids ±1 jitter at convergence)
	if step == 0 {
		m.output = m.target
	} else {
		m.output += step
	}

	return m.writeBrightness(m.output)
}

func (m *Manager) Target() int  { return m.target }
func (m *Manager) Output() int  { return m.output }

func (m *Manager) GetCurrentBrightness() (int, error) {
	return m.readBrightness()
}

func (m *Manager) readBrightness() (int, error) {
	data, err := os.ReadFile(m.backlightPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read backlight file: %v", err)
	}
	value, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid brightness value: %v", err)
	}
	return value, nil
}

func (m *Manager) writeBrightness(value int) error {
	return os.WriteFile(m.backlightPath, []byte(strconv.Itoa(value)), 0644)
}
