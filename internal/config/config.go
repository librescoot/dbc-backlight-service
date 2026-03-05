package config

import (
	"flag"
	"time"
)

type Config struct {
	RedisURL         string
	PollingTime      time.Duration
	SysBacklightPath string
	Curve            string
	MinDelta         int
}

func New() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.RedisURL, "redis-url", "redis://192.168.7.1:6379", "Redis URL")
	flag.DurationVar(&cfg.PollingTime, "polling-time", 1*time.Second, "Polling interval for illuminance value")
	flag.StringVar(&cfg.SysBacklightPath, "backlight-path", "/sys/class/backlight/backlight/brightness", "Path to backlight brightness file")
	flag.StringVar(&cfg.Curve, "curve", "0.5:100 2:500 5:1500 15:4000 35:7000 80:10240", "Lux-to-brightness curve as lux:brightness pairs")
	flag.IntVar(&cfg.MinDelta, "min-delta", 50, "Minimum brightness change to trigger a write")

	return cfg
}

func (c *Config) Parse() {
	flag.Parse()
}
