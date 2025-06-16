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
// The calculated brightness is then mapped to the device's usable range (9400-10240).
func (m *Manager) AdjustBacklight(illuminance int) error {
	m.logger.Printf("AdjustBacklight called. Current illuminance sensor value: %d", illuminance)

	const maxBrightness = 65535
	const deviceMinUsable = 9350
	const deviceMaxUsable = 10240

	var calculatedBrightness int

	currentIlluminanceFloat := float64(illuminance)

	if currentIlluminanceFloat <= m.baseIlluminanceThreshold {
		calculatedBrightness = m.baseBrightness
		m.logger.Printf("Illuminance %d <= %.1f lux. Setting brightness to base value: %d",
			illuminance, m.baseIlluminanceThreshold, calculatedBrightness)
	} else {
		// Calculate n_continuous = log_base_luxMultiplier(illuminance / baseIlluminanceThreshold)
		// This n_continuous value represents how many "luxMultiplier factors" the current illuminance is above the base threshold.
		n_continuous := math.Log(currentIlluminanceFloat/m.baseIlluminanceThreshold) / math.Log(m.luxMultiplier)

		// Brightness = baseBrightness + n_continuous * brightnessIncrement
		calculatedBrightnessFloat := float64(m.baseBrightness) + n_continuous*float64(m.brightnessIncrement)

		calculatedBrightness = int(math.Round(calculatedBrightnessFloat))

		// Cap at maxBrightness
		if calculatedBrightness > maxBrightness {
			calculatedBrightness = maxBrightness
		}

		// Ensure that for illuminance > baseIlluminanceThreshold, the brightness is at least baseBrightness.
		if calculatedBrightness < m.baseBrightness {
			calculatedBrightness = m.baseBrightness
		}

		m.logger.Printf("Illuminance %d > %.1f lux. Calculated n_continuous: %.4f. Calculated brightness: %d (capped at %d)",
			illuminance, m.baseIlluminanceThreshold, n_continuous, calculatedBrightness, maxBrightness)
	}

	// Map to device's usable range with aggressive curve for sunlight
	// Low light: 9400-9800 (small range for darkness)
	// Sunlight: 9900+ (ramp up quickly when illuminance increases)
	var targetBrightness int
	
	if currentIlluminanceFloat <= m.baseIlluminanceThreshold {
		targetBrightness = deviceMinUsable
	} else {
		// Calculate how far above base illuminance we are
		illuminanceRatio := currentIlluminanceFloat / m.baseIlluminanceThreshold
		
		if illuminanceRatio <= 2.0 {
			// Transition zone: 9600-9900 over 2x illuminance increase
			transitionProgress := (illuminanceRatio - 1.0) / 1.0 // 0 to 1 as ratio goes from 1 to 2
			targetBrightness = 9600 + int(transitionProgress*300) // 9600 to 9900
		} else {
			// Sunlight zone: 9900-10240, ramp up quickly
			sunlightProgress := math.Min((illuminanceRatio-2.0)/8.0, 1.0) // Normalize over next 8x increase
			targetBrightness = 9900 + int(sunlightProgress*float64(deviceMaxUsable-9900))
		}
	}

	// Ensure we stay within device bounds
	if targetBrightness < deviceMinUsable {
		targetBrightness = deviceMinUsable
	}
	if targetBrightness > deviceMaxUsable {
		targetBrightness = deviceMaxUsable
	}

	m.logger.Printf("Mapped illuminance %.1f lux to device brightness %d (range %d-%d)",
		currentIlluminanceFloat, targetBrightness, deviceMinUsable, deviceMaxUsable)

	return m.SetBrightness(targetBrightness)
}
