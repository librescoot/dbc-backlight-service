package backlight

import (
	"log"
	"os"
	"reflect"
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
	return New(tmp, logger, defaultCurve, 0.15, 0.2)
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

func TestParseLevels(t *testing.T) {
	levels, err := ParseLevels("low:1300 medium:4000 high:10240")
	if err != nil {
		t.Fatal(err)
	}
	if len(levels) != 3 {
		t.Fatalf("expected 3 levels, got %d", len(levels))
	}
	if levels["low"] != 1300 || levels["medium"] != 4000 || levels["high"] != 10240 {
		t.Errorf("unexpected levels: %v", levels)
	}
}

func TestParseLevelsPercentages(t *testing.T) {
	levels, err := ParseLevels("low:5% medium:28% high:100%")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"low": 512, "medium": 2867, "high": 10240}
	if !reflect.DeepEqual(levels, want) {
		t.Errorf("ParseLevels() = %v, want %v", levels, want)
	}
}

func TestParseLevelsErrors(t *testing.T) {
	tests := []string{
		"",
		"low",
		"low:bad",
		"low:%",
		"low:5.5%",
		"low:101%",
		"nocolon 2:200",
	}
	for _, s := range tests {
		if _, err := ParseLevels(s); err == nil {
			t.Errorf("expected error for %q", s)
		}
	}
}

func TestSetManualSnapsToTarget(t *testing.T) {
	m := newTestManager(t) // hardware brightness seeded at 5000

	// A manual pick applies immediately, no ramp.
	m.SetManual(10240)
	if m.Output() != 10240 {
		t.Errorf("expected immediate snap to 10240, got %d", m.Output())
	}

	// And it writes the new level straight to the backlight file.
	data, _ := os.ReadFile(m.backlightPath)
	val, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	if val != 10240 {
		t.Errorf("expected backlight file at 10240, got %d", val)
	}
}

func TestSetManualSnapsDown(t *testing.T) {
	m := newTestManager(t)
	m.SetManual(1300)
	if m.Output() != 1300 {
		t.Errorf("expected immediate snap to 1300, got %d", m.Output())
	}
}

func TestSetLuxDoesNotTouchHardware(t *testing.T) {
	m := newTestManager(t)
	m.AdjustBacklight(1.0) // initialize and write

	before, _ := os.ReadFile(m.backlightPath)
	m.SetLux(200) // moves the target only
	after, _ := os.ReadFile(m.backlightPath)

	if string(before) != string(after) {
		t.Errorf("SetLux wrote to sysfs: %q -> %q", before, after)
	}
	if m.Target() == m.Output() {
		t.Error("expected SetLux to move the target away from the output")
	}
}

func TestTickRampsWithoutNewSamples(t *testing.T) {
	m := newTestManager(t)
	m.AdjustBacklight(1.0)
	m.SetLux(200)

	// The ramp must make progress on ticks alone; that is the whole point of
	// running it faster than the ~1 Hz sensor.
	start := m.Output()
	for i := 0; i < 5; i++ {
		if err := m.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if m.Output() <= start {
		t.Errorf("expected ramp progress from %d, got %d", start, m.Output())
	}

	for i := 0; i < 300; i++ {
		m.Tick()
	}
	if m.Output() != m.Target() {
		t.Errorf("expected convergence to %d, got %d", m.Target(), m.Output())
	}
}

func TestForceOffThenResumeSnaps(t *testing.T) {
	m := newTestManager(t)
	for i := 0; i < 200; i++ {
		m.AdjustBacklight(200) // settle at full brightness
	}
	full := m.Output()

	if err := m.ForceOff(); err != nil {
		t.Fatal(err)
	}
	if m.Output() != 0 {
		t.Fatalf("expected 0 after ForceOff, got %d", m.Output())
	}

	// Resuming must snap back, not crawl up at rampRate: at 5% of the
	// remaining distance per step that would take well over a hundred steps.
	m.SetLux(200)
	if err := m.Tick(); err != nil {
		t.Fatal(err)
	}
	if m.Output() != full {
		t.Errorf("expected snap back to %d after re-enable, got %d", full, m.Output())
	}

	data, _ := os.ReadFile(m.backlightPath)
	val, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	if val != full {
		t.Errorf("expected sysfs at %d, got %d", full, val)
	}
}

func TestResumeAutoSnapsToCurrentAmbientLevel(t *testing.T) {
	m := newTestManager(t)
	m.SetLux(80)
	if err := m.Tick(); err != nil {
		t.Fatal(err)
	}
	if err := m.ForceOff(); err != nil {
		t.Fatal(err)
	}

	if err := m.ResumeAuto(); err != nil {
		t.Fatal(err)
	}
	if got, want := m.Output(), 10240; got != want {
		t.Errorf("output = %d, want %d", got, want)
	}
	data, err := os.ReadFile(m.backlightPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(data)); got != "10240" {
		t.Errorf("sysfs brightness = %s, want 10240", got)
	}
}

func TestManualIgnoresAmbient(t *testing.T) {
	m := newTestManager(t)
	m.AdjustBacklight(200) // settle bright

	if err := m.SetManual(1300); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		m.SetLux(200) // full daylight
		m.Tick()
	}
	if m.Output() != 1300 {
		t.Errorf("manual level drifted to %d, want 1300", m.Output())
	}
}

func TestSetAutoResumesFromManualLevel(t *testing.T) {
	m := newTestManager(t)
	m.AdjustBacklight(200)
	m.SetManual(1300)

	// Ambient stayed bright throughout, so going back to auto should target
	// the bright end again and ramp there from the manual level.
	for i := 0; i < 20; i++ {
		m.SetLux(200)
	}
	m.SetAuto()

	if m.Target() <= 1300 {
		t.Errorf("expected auto target above the manual level, got %d", m.Target())
	}
	if m.Output() != 1300 {
		t.Errorf("expected output to still be at the manual level, got %d", m.Output())
	}
	for i := 0; i < 300; i++ {
		m.Tick()
	}
	if m.Output() != m.Target() {
		t.Errorf("expected convergence to %d, got %d", m.Target(), m.Output())
	}
}

func TestConcurrentSetLuxAndTick(t *testing.T) {
	m := newTestManager(t)
	m.AdjustBacklight(1.0)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 500; i++ {
			m.SetLux(float64(i % 100))
		}
	}()
	for i := 0; i < 500; i++ {
		m.Tick()
	}
	<-done
}
