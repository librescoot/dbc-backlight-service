package backlight

import (
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
)

// BrightnessLevel defines a brightness level with its value and threshold ranges
type BrightnessLevel struct {
	Name            string
	Value           int
	MinIlluminance int // Minimum illuminance to stay at this level
	MaxIlluminance int // Maximum illuminance to stay at this level
}

type Manager struct {
	logger        *log.Logger
	backlightPath string
	levels        []BrightnessLevel
	currentLevel  string
}

func New(backlightPath string, logger *log.Logger) *Manager {
	// Using device tree brightness levels: 0x00, 0x800, 0x1000, 0x2000, 0x4000, 0xffff
	return &Manager{
		logger:        logger,
		backlightPath: backlightPath,
		levels: []BrightnessLevel{
			{Name: "OFF", Value: 0x00, MinIlluminance: 0, MaxIlluminance: 50},
			{Name: "VERY_LOW", Value: 0x800, MinIlluminance: 50, MaxIlluminance: 200},
			{Name: "LOW", Value: 0x1000, MinIlluminance: 200, MaxIlluminance: 1000},
			{Name: "MEDIUM", Value: 0x2000, MinIlluminance: 1000, MaxIlluminance: 5000},
			{Name: "HIGH", Value: 0x4000, MinIlluminance: 5000, MaxIlluminance: 10000},
			{Name: "MAX", Value: 0xffff, MinIlluminance: 10000, MaxIlluminance: 99999999}, // MaxIlluminance can be kept very high
		},
		currentLevel: "MAX", // Default starting level (matches default-brightness-level in device tree)
	}
}

// ConfigureLevels allows customizing the brightness levels
func (m *Manager) ConfigureLevels(levels []BrightnessLevel) {
	m.levels = levels
	m.logger.Printf("Configured %d brightness levels", len(levels))
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

func (m *Manager) SetLevel(levelName string) error {
	for _, level := range m.levels {
		if level.Name == levelName {
			m.logger.Printf("Setting backlight to %s level: %d", level.Name, level.Value)
			m.currentLevel = level.Name
			return m.SetBrightness(level.Value)
		}
	}
	return fmt.Errorf("unknown brightness level: %s", levelName)
}

// AdjustBacklight determines and sets an appropriate interpolated backlight level based on illuminance.
func (m *Manager) AdjustBacklight(illuminance int) error {
	m.logger.Printf("AdjustBacklight called. Current illuminance sensor value: %d", illuminance)

	if len(m.levels) == 0 {
		return fmt.Errorf("no brightness levels configured")
	}

	// Handle edge case: illuminance below the first level's minimum
	if illuminance <= m.levels[0].MinIlluminance {
		targetBrightness := m.levels[0].Value
		m.logger.Printf("Illuminance %d <= min threshold %d of first level %s. Setting brightness to %d",
			illuminance, m.levels[0].MinIlluminance, m.levels[0].Name, targetBrightness)
		m.currentLevel = m.levels[0].Name
		return m.SetBrightness(targetBrightness)
	}

	// Handle edge case: illuminance above or at the last level's maximum (or min if it's a single point)
	// For the last level, we typically don't interpolate "beyond" it, just set to its value.
	lastLevelIdx := len(m.levels) - 1
	if illuminance >= m.levels[lastLevelIdx].MaxIlluminance {
		targetBrightness := m.levels[lastLevelIdx].Value
		m.logger.Printf("Illuminance %d >= max threshold %d of last level %s. Setting brightness to %d",
			illuminance, m.levels[lastLevelIdx].MaxIlluminance, m.levels[lastLevelIdx].Name, targetBrightness)
		m.currentLevel = m.levels[lastLevelIdx].Name
		return m.SetBrightness(targetBrightness)
	}
	
	// Find the segment for interpolation
	for i := 0; i < len(m.levels); i++ {
		currentSegmentDef := m.levels[i]

		if illuminance >= currentSegmentDef.MinIlluminance && illuminance <= currentSegmentDef.MaxIlluminance {
			// Illuminance falls within this segment [MinIlluminance, MaxIlluminance]
			// Interpolate between Value of previous level (or current if i=0) and Value of current level.
			xCurrIllum := float64(illuminance)
			x0SegmentIllumStart := float64(currentSegmentDef.MinIlluminance)
			x1SegmentIllumEnd := float64(currentSegmentDef.MaxIlluminance)
			
			y1BrightnessAtSegmentEnd := float64(currentSegmentDef.Value)
			y0BrightnessAtSegmentStart := 0.0

			if i == 0 {
				// For the first segment, interpolate from its own value to its own value if it's a flat range,
				// or from a conceptual zero if MinIlluminance > 0 (not our case here).
				// Given our levels start at MinIlluminance=0, Value=0, this is fine.
				y0BrightnessAtSegmentStart = float64(m.levels[0].Value)
			} else {
				y0BrightnessAtSegmentStart = float64(m.levels[i-1].Value)
			}

			var targetBrightness int
			if x1SegmentIllumEnd == x0SegmentIllumStart { // Avoid division by zero if segment has zero width
				targetBrightness = int(math.Round(y1BrightnessAtSegmentEnd))
			} else {
				factor := (xCurrIllum - x0SegmentIllumStart) / (x1SegmentIllumEnd - x0SegmentIllumStart)
				interpolatedBrightness := y0BrightnessAtSegmentStart + factor*(y1BrightnessAtSegmentEnd-y0BrightnessAtSegmentStart)
				targetBrightness = int(math.Round(interpolatedBrightness))
			}
			
			m.logger.Printf("Illuminance %d in segment %s ([%d,%d] illum). Interpolating: prev_val=%.0f, curr_val=%.0f. Calculated brightness: %d",
				illuminance, currentSegmentDef.Name, currentSegmentDef.MinIlluminance, currentSegmentDef.MaxIlluminance,
				y0BrightnessAtSegmentStart, y1BrightnessAtSegmentEnd, targetBrightness)

			m.currentLevel = currentSegmentDef.Name // Update currentLevel to the name of the segment we are in
			return m.SetBrightness(targetBrightness)
		}
	}

	// Fallback: Should ideally not be reached if levels are configured contiguously
	// and edge cases are handled. This implies illuminance is outside all defined Min/Max ranges.
	// This could happen if illuminance is between m.levels[lastIdx].MaxIlluminance and the top edge case,
	// which means it's higher than any defined MaxIlluminance but not high enough for the >= lastLevel.MaxIlluminance.
	// This indicates a gap in level definitions or an issue with the logic.
	// For safety, set to the last level's value.
	m.logger.Printf("Warning: Illuminance %d did not fall into any defined segment after edge checks. Defaulting to last level %s (%d). Review level configuration.",
		illuminance, m.levels[lastLevelIdx].Name, m.levels[lastLevelIdx].Value)
	m.currentLevel = m.levels[lastLevelIdx].Name
	return m.SetBrightness(m.levels[lastLevelIdx].Value)
}

// FindAppropriateLevel finds the best level for a given illuminance value
// Useful for initial setting
func (m *Manager) FindAppropriateLevel(illuminance int) (string, error) {
	for i, level := range m.levels {
		if illuminance >= level.MinIlluminance && illuminance <= level.MaxIlluminance {
			return level.Name, nil
		}
		
		// Edge case for illuminance below lowest level
		if i == 0 && illuminance < level.MinIlluminance {
			return level.Name, nil
		}
		
		// Edge case for illuminance above highest level
		if i == len(m.levels)-1 && illuminance > level.MaxIlluminance {
			return level.Name, nil
		}
	}
	
	// Default to middle level if something went wrong
	middleLevel := m.levels[len(m.levels)/2].Name
	m.logger.Printf("Could not find appropriate level for illuminance %d, defaulting to %s", 
		illuminance, middleLevel)
	return middleLevel, nil
}
