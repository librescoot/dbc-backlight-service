package backlight

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReadMaxBrightness(t *testing.T) {
	path := filepath.Join(t.TempDir(), "max_brightness")
	if err := os.WriteFile(path, []byte("32768\n"), 0644); err != nil {
		t.Fatal(err)
	}

	max, err := ReadMaxBrightness(path)
	if err != nil {
		t.Fatal(err)
	}
	if max != 32768 {
		t.Errorf("max = %d, want 32768", max)
	}
}

func TestReadMaxBrightnessRejectsNonPositiveValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "max_brightness")
	if err := os.WriteFile(path, []byte("0\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := ReadMaxBrightness(path); err == nil {
		t.Fatal("ReadMaxBrightness accepted zero")
	}
}

func TestScaleCurve(t *testing.T) {
	curve := []Point{{Lux: 0, Brightness: 400}, {Lux: 80, Brightness: 10240}}
	got := ScaleCurve(curve, 32768)
	want := []Point{{Lux: 0, Brightness: 1280}, {Lux: 80, Brightness: 32768}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ScaleCurve() = %v, want %v", got, want)
	}
}

func TestScaleLevels(t *testing.T) {
	got := ScaleLevels(map[string]int{"low": 1300, "high": 10240}, 32768)
	want := map[string]int{"low": 4160, "high": 32768}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ScaleLevels() = %v, want %v", got, want)
	}
}
