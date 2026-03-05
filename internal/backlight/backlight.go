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
// Example: "0.5:100 2:400 5:1200 35:10240"
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
	logger        *log.Logger
	backlightPath string
	curve         []Point
	current       int // last written brightness value
	minDelta      int // minimum change to trigger a write
}

func New(backlightPath string, logger *log.Logger, curve []Point, minDelta int) *Manager {
	m := &Manager{
		logger:        logger,
		backlightPath: backlightPath,
		curve:         curve,
		current:       -1,
		minDelta:      minDelta,
	}

	if brightness, err := m.readBrightness(); err == nil {
		m.current = brightness
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

// AdjustBacklight computes the target brightness for the given lux value
// and writes it to the backlight sysfs file if the change exceeds minDelta.
func (m *Manager) AdjustBacklight(lux float64) error {
	target := m.Interpolate(lux)

	delta := target - m.current
	if delta < 0 {
		delta = -delta
	}

	if m.current >= 0 && delta < m.minDelta {
		return nil
	}

	m.logger.Printf("lux=%.1f → brightness %d (was %d)", lux, target, m.current)
	m.current = target
	return m.writeBrightness(target)
}

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
