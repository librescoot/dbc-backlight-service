package service

import (
	"io"
	"log"
	"os"
	"strconv"
	"testing"

	"github.com/librescoot/dbc-backlight-service/internal/backlight"
)

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
