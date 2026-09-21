package service

import (
	"io"
	"log"
	"os"
	"strconv"
	"testing"

	"github.com/librescoot/dbc-backlight-service/internal/backlight"
	"github.com/librescoot/dbc-backlight-service/internal/config"
)

func TestNewScalesNormalizedLevelsToKernelRange(t *testing.T) {
	dir := t.TempDir()
	brightnessPath := dir + "/brightness"
	maxBrightnessPath := dir + "/max_brightness"
	if err := os.WriteFile(brightnessPath, []byte("0"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(maxBrightnessPath, []byte("32768"), 0644); err != nil {
		t.Fatal(err)
	}

	service, err := New(&config.Config{
		RedisURL:          "redis://127.0.0.1:6379",
		SysBacklightPath:  brightnessPath,
		MaxBrightnessPath: maxBrightnessPath,
		Curve:             "0:400 80:10240",
		ManualLevels:      "low:1300 high:10240",
		RampRate:          0.05,
		LuxAlpha:          0.1,
	}, log.New(io.Discard, "", 0), "test")
	if err != nil {
		t.Fatal(err)
	}

	if got := service.Backlight.Interpolate(0); got != 1280 {
		t.Errorf("minimum curve output = %d, want 1280", got)
	}
	if got := service.Backlight.Interpolate(80); got != 32768 {
		t.Errorf("maximum curve output = %d, want 32768", got)
	}
	if got := service.manualLevels["low"]; got != 4160 {
		t.Errorf("low manual level = %d, want 4160", got)
	}
	if got := service.manualLevels["high"]; got != 32768 {
		t.Errorf("high manual level = %d, want 32768", got)
	}
}

func TestRestoreBacklightReappliesManualLevel(t *testing.T) {
	path := t.TempDir() + "/brightness"
	if err := os.WriteFile(path, []byte("5000"), 0644); err != nil {
		t.Fatal(err)
	}
	curve, err := backlight.ParseCurve("0:400 80:10240")
	if err != nil {
		t.Fatal(err)
	}
	manager := backlight.New(path, log.New(io.Discard, "", 0), curve, 0.05, 0.1)
	service := &Service{
		Backlight:     manager,
		manualLevels:  map[string]int{"high": 10240},
		backlightMode: "high",
	}

	if err := manager.SetManual(10240); err != nil {
		t.Fatal(err)
	}
	if err := manager.ForceOff(); err != nil {
		t.Fatal(err)
	}

	manual, err := service.restoreBacklight()
	if err != nil {
		t.Fatal(err)
	}
	if !manual {
		t.Fatal("manual mode should be reported as restored")
	}
	if got := manager.Output(); got != 10240 {
		t.Errorf("output = %d, want 10240", got)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	brightness, err := strconv.Atoi(string(contents))
	if err != nil {
		t.Fatal(err)
	}
	if brightness != 10240 {
		t.Errorf("brightness = %d, want 10240", brightness)
	}
}
