package config

import (
	"flag"
	"time"
)

type Config struct {
	RedisURL                   string
	PollingTime                time.Duration
	SysBacklightPath           string
	FormulaBaseIlluminance     float64
	FormulaBaseBrightness      int
	FormulaLuxMultiplier       float64
	FormulaBrightnessIncrement int
	HysteresisThreshold        int
}

func New() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.RedisURL, "redis-url", "redis://192.168.7.1:6379", "Redis URL")
	flag.DurationVar(&cfg.PollingTime, "polling-time", 1*time.Second, "Polling interval for illuminance value")
	flag.StringVar(&cfg.SysBacklightPath, "backlight-path", "/sys/class/backlight/backlight/brightness", "Path to backlight brightness file")

	// Formula parameters
	flag.Float64Var(&cfg.FormulaBaseIlluminance, "base-illuminance", 15.0, "Base illuminance threshold for the formula (lux)")
	flag.IntVar(&cfg.FormulaBaseBrightness, "base-brightness", 8192, "Base brightness value for the formula")
	flag.Float64Var(&cfg.FormulaLuxMultiplier, "lux-multiplier", 3.0, "Lux multiplier (log base) for the formula")
	flag.IntVar(&cfg.FormulaBrightnessIncrement, "brightness-increment", 2048, "Brightness increment step for the formula")
	flag.IntVar(&cfg.HysteresisThreshold, "hysteresis-threshold", 512, "Minimum brightness change required to trigger Redis update (prevents jitter)")

	return cfg
}

func (c *Config) Parse() {
	flag.Parse()
}
