package backlight

import (
	"fmt"
	"log"
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
			{Name: "MAX", Value: 0xffff, MinIlluminance: 10000, MaxIlluminance: 999999}, // MaxIlluminance can be kept very high
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

// AdjustBacklight determines the appropriate backlight level based on illuminance
func (m *Manager) AdjustBacklight(illuminance int) error {
	m.logger.Printf("Current illuminance value: %d", illuminance)
	
	// Find current level index
	currentIndex := -1
	for i, level := range m.levels {
		if level.Name == m.currentLevel {
			currentIndex = i
			break
		}
	}
	
	if currentIndex == -1 {
		// If current level not found, default to middle level
		currentIndex = len(m.levels) / 2
		m.logger.Printf("Unknown current level, defaulting to %s", m.levels[currentIndex].Name)
	}
	
	// Check if we need to change level
	currentLevel := m.levels[currentIndex]
	
	// If illuminance is below min threshold for current level, go down a level
	if illuminance < currentLevel.MinIlluminance && currentIndex > 0 {
		nextLevel := m.levels[currentIndex-1]
		m.logger.Printf("Decreasing brightness from %s to %s (illuminance %d < min %d)", 
			currentLevel.Name, nextLevel.Name, illuminance, currentLevel.MinIlluminance)
		return m.SetLevel(nextLevel.Name)
	}
	
	// If illuminance is above max threshold for current level, go up a level
	if illuminance > currentLevel.MaxIlluminance && currentIndex < len(m.levels)-1 {
		nextLevel := m.levels[currentIndex+1]
		m.logger.Printf("Increasing brightness from %s to %s (illuminance %d > max %d)", 
			currentLevel.Name, nextLevel.Name, illuminance, currentLevel.MaxIlluminance)
		return m.SetLevel(nextLevel.Name)
	}
	
	m.logger.Printf("Staying at %s level (illuminance %d within range %d-%d)", 
		currentLevel.Name, illuminance, currentLevel.MinIlluminance, currentLevel.MaxIlluminance)
	
	return nil
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
