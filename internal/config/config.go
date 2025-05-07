package config

import (
	"flag"
	"time"
)

type Config struct {
	RedisURL       string
	PollingTime    time.Duration
	SysBacklightPath string
	LowBrightness  int
	MidBrightness  int
	HighBrightness int
}

func New() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.RedisURL, "redis-url", "redis://192.168.7.1:6379", "Redis URL")
	flag.DurationVar(&cfg.PollingTime, "polling-time", 1*time.Second, "Polling interval for illumination value")
	flag.StringVar(&cfg.SysBacklightPath, "backlight-path", "/sys/class/backlight/backlight/brightness", "Path to backlight brightness file")
	flag.IntVar(&cfg.LowBrightness, "low-brightness", 9500, "Low brightness level")
	flag.IntVar(&cfg.MidBrightness, "mid-brightness", 9950, "Medium brightness level")
	flag.IntVar(&cfg.HighBrightness, "high-brightness", 10240, "High brightness level")

	return cfg
}

func (c *Config) Parse() {
	flag.Parse()
}