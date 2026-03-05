package config

import (
	"flag"
	"time"
)

type Config struct {
	RedisURL         string
	PollingTime      time.Duration
	SysBacklightPath string
	SensorPath       string
	Curve            string
	RampRate         float64
	Debug            bool
}

func New() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.RedisURL, "redis-url", "redis://192.168.7.1:6379", "Redis URL")
	flag.DurationVar(&cfg.PollingTime, "polling-time", 50*time.Millisecond, "Polling interval for illuminance sensor")
	flag.StringVar(&cfg.SysBacklightPath, "backlight-path", "/sys/class/backlight/backlight/brightness", "Path to backlight brightness file")
	flag.StringVar(&cfg.SensorPath, "sensor-path", "", "Path to IIO illuminance input (e.g. /sys/bus/iio/devices/iio:device0/in_illuminance_input). If empty, reads from Redis.")
	flag.StringVar(&cfg.Curve, "curve", "0:256 0.5:1024 1:1750 2:2300 5:3200 10:4200 20:5600 35:7100 50:8300 80:10240", "Lux-to-brightness curve as lux:brightness pairs")
	flag.Float64Var(&cfg.RampRate, "ramp-rate", 0.15, "Fraction of remaining distance to move per tick (0..1)")
	flag.BoolVar(&cfg.Debug, "debug", false, "Enable debug logging")

	return cfg
}

func (c *Config) Parse() {
	flag.Parse()
}
