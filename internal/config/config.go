package config

import (
	"flag"
	"time"
)

type Config struct {
	RedisURL         string
	PollingTime      time.Duration
	SysBacklightPath string

	// Brightness values for each state
	VeryLowBrightness  int
	LowBrightness      int
	MidBrightness      int
	HighBrightness     int
	VeryHighBrightness int

	// Upward transition thresholds (lux values to transition to next higher state)
	VeryLowToLowThreshold   int
	LowToMidThreshold       int
	MidToHighThreshold      int
	HighToVeryHighThreshold int

	// Downward transition thresholds (lux values to transition to next lower state)
	LowToVeryLowThreshold   int
	MidToLowThreshold       int
	HighToMidThreshold      int
	VeryHighToHighThreshold int

	HysteresisThreshold int
}

func New() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.RedisURL, "redis-url", "redis://192.168.7.1:6379", "Redis URL")
	flag.DurationVar(&cfg.PollingTime, "polling-time", 1*time.Second, "Polling interval for illuminance value")
	flag.StringVar(&cfg.SysBacklightPath, "backlight-path", "/sys/class/backlight/backlight/brightness", "Path to backlight brightness file")

	// Brightness levels
	flag.IntVar(&cfg.VeryLowBrightness, "very-low-brightness", 9350, "Brightness value for VERY_LOW state")
	flag.IntVar(&cfg.LowBrightness, "low-brightness", 9500, "Brightness value for LOW state")
	flag.IntVar(&cfg.MidBrightness, "mid-brightness", 9700, "Brightness value for MID state")
	flag.IntVar(&cfg.HighBrightness, "high-brightness", 9950, "Brightness value for HIGH state")
	flag.IntVar(&cfg.VeryHighBrightness, "very-high-brightness", 10240, "Brightness value for VERY_HIGH state")

	// Upward thresholds
	flag.IntVar(&cfg.VeryLowToLowThreshold, "very-low-to-low-threshold", 8, "Illuminance threshold to transition from VERY_LOW to LOW (lux)")
	flag.IntVar(&cfg.LowToMidThreshold, "low-to-mid-threshold", 18, "Illuminance threshold to transition from LOW to MID (lux)")
	flag.IntVar(&cfg.MidToHighThreshold, "mid-to-high-threshold", 40, "Illuminance threshold to transition from MID to HIGH (lux)")
	flag.IntVar(&cfg.HighToVeryHighThreshold, "high-to-very-high-threshold", 80, "Illuminance threshold to transition from HIGH to VERY_HIGH (lux)")

	// Downward thresholds
	flag.IntVar(&cfg.LowToVeryLowThreshold, "low-to-very-low-threshold", 5, "Illuminance threshold to transition from LOW to VERY_LOW (lux)")
	flag.IntVar(&cfg.MidToLowThreshold, "mid-to-low-threshold", 15, "Illuminance threshold to transition from MID to LOW (lux)")
	flag.IntVar(&cfg.HighToMidThreshold, "high-to-mid-threshold", 35, "Illuminance threshold to transition from HIGH to MID (lux)")
	flag.IntVar(&cfg.VeryHighToHighThreshold, "very-high-to-high-threshold", 70, "Illuminance threshold to transition from VERY_HIGH to HIGH (lux)")

	flag.IntVar(&cfg.HysteresisThreshold, "hysteresis-threshold", 512, "Minimum brightness change required to trigger Redis update (prevents jitter)")

	return cfg
}

func (c *Config) Parse() {
	flag.Parse()
}
