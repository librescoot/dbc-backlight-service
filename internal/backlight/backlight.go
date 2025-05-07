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
	MinIllumination int // Minimum illumination to stay at this level
	MaxIllumination int // Maximum illumination to stay at this level
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
			{Name: "OFF", Value: 0x00, MinIllumination: 0, MaxIllumination: 5},
			{Name: "VERY_LOW", Value: 0x800, MinIllumination: 5, MaxIllumination: 15},
			{Name: "LOW", Value: 0x1000, MinIllumination: 15, MaxIllumination: 30},
			{Name: "MEDIUM", Value: 0x2000, MinIllumination: 30, MaxIllumination: 45},
			{Name: "HIGH", Value: 0x4000, MinIllumination: 45, MaxIllumination: 60},
			{Name: "MAX", Value: 0xffff, MinIllumination: 60, MaxIllumination: 999999},
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

// AdjustBacklight determines the appropriate backlight level based on illumination
func (m *Manager) AdjustBacklight(illumination int) error {
	m.logger.Printf("Current illumination value: %d", illumination)
	
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
	
	// If illumination is below min threshold for current level, go down a level
	if illumination < currentLevel.MinIllumination && currentIndex > 0 {
		nextLevel := m.levels[currentIndex-1]
		m.logger.Printf("Decreasing brightness from %s to %s (illumination %d < min %d)", 
			currentLevel.Name, nextLevel.Name, illumination, currentLevel.MinIllumination)
		return m.SetLevel(nextLevel.Name)
	}
	
	// If illumination is above max threshold for current level, go up a level
	if illumination > currentLevel.MaxIllumination && currentIndex < len(m.levels)-1 {
		nextLevel := m.levels[currentIndex+1]
		m.logger.Printf("Increasing brightness from %s to %s (illumination %d > max %d)", 
			currentLevel.Name, nextLevel.Name, illumination, currentLevel.MaxIllumination)
		return m.SetLevel(nextLevel.Name)
	}
	
	m.logger.Printf("Staying at %s level (illumination %d within range %d-%d)", 
		currentLevel.Name, illumination, currentLevel.MinIllumination, currentLevel.MaxIllumination)
	
	return nil
}

// FindAppropriateLevel finds the best level for a given illumination value
// Useful for initial setting
func (m *Manager) FindAppropriateLevel(illumination int) (string, error) {
	for i, level := range m.levels {
		if illumination >= level.MinIllumination && illumination <= level.MaxIllumination {
			return level.Name, nil
		}
		
		// Edge case for illumination below lowest level
		if i == 0 && illumination < level.MinIllumination {
			return level.Name, nil
		}
		
		// Edge case for illumination above highest level
		if i == len(m.levels)-1 && illumination > level.MaxIllumination {
			return level.Name, nil
		}
	}
	
	// Default to middle level if something went wrong
	middleLevel := m.levels[len(m.levels)/2].Name
	m.logger.Printf("Could not find appropriate level for illumination %d, defaulting to %s", 
		illumination, middleLevel)
	return middleLevel, nil
}