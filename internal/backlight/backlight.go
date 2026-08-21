package backlight

import (
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
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

// ParseLevels parses a manual level map of "name:brightness" pairs.
// Example: "low:1300 medium:4000 high:10240"
func ParseLevels(s string) (map[string]int, error) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil, fmt.Errorf("levels must have at least one entry")
	}

	levels := make(map[string]int, len(fields))
	for _, f := range fields {
		parts := strings.SplitN(f, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid level %q (expected name:brightness)", f)
		}
		brightness, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid brightness value %q: %v", parts[1], err)
		}
		levels[parts[0]] = brightness
	}

	return levels, nil
}

// Manager owns the lux->brightness state. SetLux runs on the sensor goroutine
// (roughly 1 Hz, paced by the blocking sysfs read) and Tick runs on the ramp
// goroutine (tens of Hz), so every field below is guarded by mu.
type Manager struct {
	logger         *log.Logger
	backlightPath  string
	curve          []Point
	mu             sync.Mutex
	output         int     // brightness the ramp has reached
	written        int     // last value actually written to sysfs
	target         int     // desired brightness from interpolation
	smoothedLux    float64 // EMA-filtered lux value
	luxAlpha       float64 // EMA smoothing factor per sample (0..1)
	rampRate       float64 // fraction of remaining distance per ramp tick (0..1)
	targetDeadband int     // minimum brightness change to update target (anti-flicker)
	manual         bool    // fixed level selected; ambient light is ignored
	initialized    bool
}

func New(backlightPath string, logger *log.Logger, curve []Point, rampRate, luxAlpha float64) *Manager {
	m := &Manager{
		logger:         logger,
		backlightPath:  backlightPath,
		curve:          curve,
		output:         -1,
		written:        -1,
		target:         -1,
		smoothedLux:    -1,
		luxAlpha:       luxAlpha, // smooth lux input via EMA; lower is slower/less flickery
		rampRate:       rampRate,
		targetDeadband: 150, // ignore target changes smaller than this (anti-flicker)
	}

	if brightness, err := m.readBrightness(); err == nil {
		m.output = brightness
		m.written = brightness
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

// SetLux feeds one ambient light sample in. It smooths the input and moves the
// target, but never touches the hardware: Tick does that. Called from the
// sensor goroutine at whatever rate the hardware can actually deliver.
func (m *Manager) SetLux(lux float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Smooth the lux input with EMA to reject single-sample spikes
	if m.smoothedLux < 0 {
		m.smoothedLux = lux
	} else {
		m.smoothedLux = m.luxAlpha*lux + (1-m.luxAlpha)*m.smoothedLux
	}

	if m.manual {
		return // keep the EMA warm for the switch back to auto, but don't steer
	}

	newTarget := m.Interpolate(m.smoothedLux)

	if !m.initialized {
		m.target = newTarget
		m.output = newTarget
		m.initialized = true
		m.logger.Printf("lux=%.1f -> brightness %d (initial)", lux, m.output)
		return
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
}

// Tick moves the output one ramp-step toward the target and writes it if it
// moved. Cheap and non-blocking, so it can run far faster than the sensor.
func (m *Manager) Tick() error {
	m.mu.Lock()
	if m.output != m.target {
		diff := float64(m.target - m.output)
		step := int(math.Round(diff * m.rampRate))
		if step == 0 {
			m.output = m.target
		} else {
			m.output += step
		}
	}
	out, written := m.output, m.written
	m.mu.Unlock()

	if out == written {
		return nil
	}
	if err := m.writeBrightness(out); err != nil {
		return err
	}

	m.mu.Lock()
	m.written = out
	m.mu.Unlock()
	return nil
}

// AdjustBacklight feeds a sample in and immediately ramps one step. Only the
// Redis-sourced path uses it, where reads are cheap and there is no reason to
// split the two across goroutines.
func (m *Manager) AdjustBacklight(lux float64) error {
	m.SetLux(lux)
	return m.Tick()
}

// SetManual pins the brightness to a fixed level and applies it immediately.
// A manual selection is a deliberate user choice, so it snaps rather than
// ramping (auto mode keeps the smooth ambient ramp via SetLux/Tick).
func (m *Manager) SetManual(level int) error {
	m.mu.Lock()
	m.manual = true
	m.target = level
	m.output = level
	needsWrite := m.written != level
	m.mu.Unlock()

	if !needsWrite {
		return nil
	}
	if err := m.writeBrightness(level); err != nil {
		return err
	}

	m.mu.Lock()
	m.written = level
	m.mu.Unlock()
	return nil
}

// SetAuto hands control back to the ambient light curve. The output ramps from
// wherever the manual level left it rather than jumping.
func (m *Manager) SetAuto() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.manual = false
	if m.smoothedLux >= 0 {
		// Re-derive the target now instead of waiting up to a full sensor
		// interval for the next sample.
		m.target = m.Interpolate(m.smoothedLux)
		m.initialized = true
	}
}

func (m *Manager) Target() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.target
}

func (m *Manager) Output() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.output
}

// ForceOff writes brightness 0 for an external override. It clears initialized
// so that re-enabling snaps back to the ambient level the way a cold start
// does: ramping up from 0 at rampRate takes well over a minute, which reads as
// a broken display rather than a smooth fade.
func (m *Manager) ForceOff() error {
	m.mu.Lock()
	m.output = 0
	m.target = 0
	m.initialized = false
	m.mu.Unlock()

	if err := m.writeBrightness(0); err != nil {
		return err
	}

	m.mu.Lock()
	m.written = 0
	m.mu.Unlock()
	return nil
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
