package backlight

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const ReferenceMaxBrightness = 10240

// ReadMaxBrightness reads the range exported by the kernel backlight device.
func ReadMaxBrightness(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read max brightness: %w", err)
	}

	max, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("parse max brightness: %w", err)
	}
	if max <= 0 {
		return 0, fmt.Errorf("max brightness must be positive, got %d", max)
	}
	return max, nil
}

// ScaleCurve maps normalized curve values onto the kernel backlight range.
func ScaleCurve(curve []Point, maxBrightness int) []Point {
	scaled := make([]Point, len(curve))
	for i, point := range curve {
		scaled[i] = Point{
			Lux:        point.Lux,
			Brightness: scaleBrightness(point.Brightness, maxBrightness),
		}
	}
	return scaled
}

// ScaleLevels maps normalized manual levels onto the kernel backlight range.
func ScaleLevels(levels map[string]int, maxBrightness int) map[string]int {
	scaled := make(map[string]int, len(levels))
	for name, level := range levels {
		scaled[name] = scaleBrightness(level, maxBrightness)
	}
	return scaled
}

func scaleBrightness(brightness, maxBrightness int) int {
	return int((int64(brightness)*int64(maxBrightness) + ReferenceMaxBrightness/2) / ReferenceMaxBrightness)
}
