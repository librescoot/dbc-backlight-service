package backlight

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

// BrightnessLevel represents the current brightness state
type BrightnessLevel int

const (
	LevelVeryLow BrightnessLevel = iota
	LevelLow
	LevelMid
	LevelHigh
	LevelVeryHigh
)

func (l BrightnessLevel) String() string {
	switch l {
	case LevelVeryLow:
		return "VERY_LOW"
	case LevelLow:
		return "LOW"
	case LevelMid:
		return "MID"
	case LevelHigh:
		return "HIGH"
	case LevelVeryHigh:
		return "VERY_HIGH"
	default:
		return "UNKNOWN"
	}
}

// StateConfig defines brightness value and transition thresholds for each state
type StateConfig struct {
	Brightness    int
	ThresholdUp   int // lux value to transition to next higher state
	ThresholdDown int // lux value to transition to next lower state
}

type Manager struct {
	logger        *log.Logger
	backlightPath string
	currentLevel  BrightnessLevel
	states        map[BrightnessLevel]StateConfig
}

func New(
	backlightPath string,
	logger *log.Logger,
	veryLowBrightness int,
	lowBrightness int,
	midBrightness int,
	highBrightness int,
	veryHighBrightness int,
	veryLowToLowThreshold int,
	lowToMidThreshold int,
	midToHighThreshold int,
	highToVeryHighThreshold int,
	lowToVeryLowThreshold int,
	midToLowThreshold int,
	highToMidThreshold int,
	veryHighToHighThreshold int,
) *Manager {
	states := map[BrightnessLevel]StateConfig{
		LevelVeryLow: {
			Brightness:    veryLowBrightness,
			ThresholdUp:   veryLowToLowThreshold,
			ThresholdDown: 0, // No lower state
		},
		LevelLow: {
			Brightness:    lowBrightness,
			ThresholdUp:   lowToMidThreshold,
			ThresholdDown: lowToVeryLowThreshold,
		},
		LevelMid: {
			Brightness:    midBrightness,
			ThresholdUp:   midToHighThreshold,
			ThresholdDown: midToLowThreshold,
		},
		LevelHigh: {
			Brightness:    highBrightness,
			ThresholdUp:   highToVeryHighThreshold,
			ThresholdDown: highToMidThreshold,
		},
		LevelVeryHigh: {
			Brightness:    veryHighBrightness,
			ThresholdUp:   0, // No higher state
			ThresholdDown: veryHighToHighThreshold,
		},
	}

	return &Manager{
		logger:        logger,
		backlightPath: backlightPath,
		currentLevel:  LevelMid, // Start at medium level
		states:        states,
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

// AdjustBacklight adjusts the backlight brightness using a discrete state machine
// with hysteresis to prevent rapid oscillation between brightness levels.
func (m *Manager) AdjustBacklight(illuminance int) error {
	m.logger.Printf("AdjustBacklight called. Current illuminance: %d lux, current state: %s",
		illuminance, m.currentLevel)

	previousLevel := m.currentLevel
	currentState := m.states[m.currentLevel]

	// Check for state transitions based on hysteresis thresholds
	switch m.currentLevel {
	case LevelVeryLow:
		if currentState.ThresholdUp > 0 && illuminance > currentState.ThresholdUp {
			m.currentLevel = LevelLow
			m.logger.Printf("Transitioning VERY_LOW → LOW (illuminance %d > %d)",
				illuminance, currentState.ThresholdUp)
		}

	case LevelLow:
		if currentState.ThresholdUp > 0 && illuminance > currentState.ThresholdUp {
			m.currentLevel = LevelMid
			m.logger.Printf("Transitioning LOW → MID (illuminance %d > %d)",
				illuminance, currentState.ThresholdUp)
		} else if currentState.ThresholdDown > 0 && illuminance < currentState.ThresholdDown {
			m.currentLevel = LevelVeryLow
			m.logger.Printf("Transitioning LOW → VERY_LOW (illuminance %d < %d)",
				illuminance, currentState.ThresholdDown)
		}

	case LevelMid:
		if currentState.ThresholdUp > 0 && illuminance > currentState.ThresholdUp {
			m.currentLevel = LevelHigh
			m.logger.Printf("Transitioning MID → HIGH (illuminance %d > %d)",
				illuminance, currentState.ThresholdUp)
		} else if currentState.ThresholdDown > 0 && illuminance < currentState.ThresholdDown {
			m.currentLevel = LevelLow
			m.logger.Printf("Transitioning MID → LOW (illuminance %d < %d)",
				illuminance, currentState.ThresholdDown)
		}

	case LevelHigh:
		if currentState.ThresholdUp > 0 && illuminance > currentState.ThresholdUp {
			m.currentLevel = LevelVeryHigh
			m.logger.Printf("Transitioning HIGH → VERY_HIGH (illuminance %d > %d)",
				illuminance, currentState.ThresholdUp)
		} else if currentState.ThresholdDown > 0 && illuminance < currentState.ThresholdDown {
			m.currentLevel = LevelMid
			m.logger.Printf("Transitioning HIGH → MID (illuminance %d < %d)",
				illuminance, currentState.ThresholdDown)
		}

	case LevelVeryHigh:
		if currentState.ThresholdDown > 0 && illuminance < currentState.ThresholdDown {
			m.currentLevel = LevelHigh
			m.logger.Printf("Transitioning VERY_HIGH → HIGH (illuminance %d < %d)",
				illuminance, currentState.ThresholdDown)
		}
	}

	// Set brightness if state changed
	newState := m.states[m.currentLevel]
	if m.currentLevel != previousLevel {
		m.logger.Printf("State changed: %s → %s, setting brightness to %d",
			previousLevel, m.currentLevel, newState.Brightness)
		return m.SetBrightness(newState.Brightness)
	}

	m.logger.Printf("Staying in state %s (brightness %d)", m.currentLevel, newState.Brightness)
	return nil
}

// GetCurrentLevel returns the current brightness level state (useful for testing)
func (m *Manager) GetCurrentLevel() BrightnessLevel {
	return m.currentLevel
}

// SetCurrentLevel sets the current brightness level state (useful for testing)
func (m *Manager) SetCurrentLevel(level BrightnessLevel) {
	m.currentLevel = level
}
