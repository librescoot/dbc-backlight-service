package config

import (
	"flag"
	"time"
)

type Config struct {
	RedisURL          string
	PollingTime       time.Duration
	SensorInterval    time.Duration
	SysBacklightPath  string
	MaxBrightnessPath string
	SensorPath        string
	Curve             string
	ManualLevels      string
	RampRate          float64
	LuxAlpha          float64
	Debug             bool
}

func New() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.RedisURL, "redis-url", "redis://192.168.7.1:6379", "Redis URL")
	flag.DurationVar(&cfg.PollingTime, "polling-time", 50*time.Millisecond, "Interval between backlight ramp steps")
	flag.DurationVar(&cfg.SensorInterval, "sensor-interval", 100*time.Millisecond, "Minimum interval between ambient light samples. The OPT3001 has no data-ready interrupt on the DBC, so a read blocks for the integration time and this only acts as a floor (~170ms/sample at the 0.1s integration the unit file selects).")
	flag.StringVar(&cfg.SysBacklightPath, "backlight-path", "/sys/class/backlight/backlight/brightness", "Path to backlight brightness file")
	flag.StringVar(&cfg.MaxBrightnessPath, "max-brightness-path", "/sys/class/backlight/backlight/max_brightness", "Path to backlight max_brightness file")
	flag.StringVar(&cfg.SensorPath, "sensor-path", "", "Path to IIO illuminance input (e.g. /sys/bus/iio/devices/iio:device0/in_illuminance_input). If empty, reads from Redis.")
	flag.StringVar(&cfg.Curve, "curve", "0:400 0.5:1300 1:2200 2:2900 5:4000 10:5200 20:7000 35:8600 50:9600 80:10240", "Lux-to-brightness curve as lux:brightness pairs on a normalized 0..10240 scale")
	flag.StringVar(&cfg.ManualLevels, "manual-levels", "low:1300 medium:4000 high:10240", "Manual backlight levels as name:brightness pairs on a normalized 0..10240 scale")
	flag.Float64Var(&cfg.RampRate, "ramp-rate", 0.05, "Fraction of remaining distance to move per ramp step (0..1)")
	flag.Float64Var(&cfg.LuxAlpha, "lux-alpha", 0.1, "EMA smoothing factor applied per lux sample (0..1); lower is slower/less flickery")
	flag.BoolVar(&cfg.Debug, "debug", false, "Enable debug logging")

	return cfg
}

func (c *Config) Parse() {
	flag.Parse()
}
