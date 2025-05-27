package backlight

import (
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
)

type Manager struct {
	logger                   *log.Logger
	backlightPath            string
	baseIlluminanceThreshold float64
	baseBrightness           int
	luxMultiplier            float64
	brightnessIncrement      int
}

func New(
	backlightPath string,
	logger *log.Logger,
	baseIlluminance float64,
	baseBrightness int,
	luxMultiplier float64,
	brightnessIncrement int,
) *Manager {
	return &Manager{
		logger:                   logger,
		backlightPath:            backlightPath,
		baseIlluminanceThreshold: baseIlluminance,
		baseBrightness:           baseBrightness,
		luxMultiplier:            luxMultiplier,
		brightnessIncrement:      brightnessIncrement,
	}
}

func (m *Manager) SetBrightness(value int) error {
	// Write to the backlight file
	err := os.WriteFile(m.backlightPath, []byte(strconv.Itoa(value)), 0644)
	if err != nil {
		return fmt.Errorf("failed to write to backlight file: %v", err)
	}
	return nil
}

func (m *Manager) GetCurrentBrightness() (int, error) {
	// Read the current brightness value
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

// AdjustBacklight calculates and sets the backlight brightness based on a mathematical curve.
// Minimum backlight is 8192 up to 15 lux.
// Above 15 lux, brightness increases by 2048 for every 3x increase in lux.
func (m *Manager) AdjustBacklight(illuminance int) error {
	m.logger.Printf("AdjustBacklight called. Current illuminance sensor value: %d", illuminance)

	const maxBrightness = 65535

	var targetBrightness int

	currentIlluminanceFloat := float64(illuminance)

	if currentIlluminanceFloat <= m.baseIlluminanceThreshold {
		targetBrightness = m.baseBrightness
		m.logger.Printf("Illuminance %d <= %.1f lux. Setting brightness to base value: %d",
			illuminance, m.baseIlluminanceThreshold, targetBrightness)
	} else {
		// Calculate n_continuous = log_base_luxMultiplier(illuminance / baseIlluminanceThreshold)
		// This n_continuous value represents how many "luxMultiplier factors" the current illuminance is above the base threshold.
		n_continuous := math.Log(currentIlluminanceFloat/m.baseIlluminanceThreshold) / math.Log(m.luxMultiplier)

		// Brightness = baseBrightness + n_continuous * brightnessIncrement
		calculatedBrightnessFloat := float64(m.baseBrightness) + n_continuous*float64(m.brightnessIncrement)

		targetBrightness = int(math.Round(calculatedBrightnessFloat))

		// Cap at maxBrightness
		if targetBrightness > maxBrightness {
			targetBrightness = maxBrightness
		}

		// Ensure that for illuminance > baseIlluminanceThreshold, the brightness is at least baseBrightness.
		if targetBrightness < m.baseBrightness {
			targetBrightness = m.baseBrightness
		}

		m.logger.Printf("Illuminance %d > %.1f lux. Calculated n_continuous: %.4f. Target brightness: %d (capped at %d)",
			illuminance, m.baseIlluminanceThreshold, n_continuous, targetBrightness, maxBrightness)
	}

	return m.SetBrightness(targetBrightness)
}
