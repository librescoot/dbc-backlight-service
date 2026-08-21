package service

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/librescoot/dbc-backlight-service/internal/backlight"
	"github.com/librescoot/dbc-backlight-service/internal/config"
	redisClient "github.com/librescoot/dbc-backlight-service/internal/redis"
)

type Service struct {
	Config                  *config.Config
	Redis                   *redisClient.Client
	Logger                  *log.Logger
	Backlight               *backlight.Manager
	lastPublishedBrightness int
	lastPublishedLux        float64
	luxPublishMinDelta      float64
	publishMinInterval      time.Duration
	lastPublishedAt         time.Time
	lastLoggedTarget        int
	backlightDisabled       atomic.Bool
	overrideCh              chan struct{}
	manualLevels            map[string]int
	backlightMode           string
	modeCh                  chan struct{}
}

func New(cfg *config.Config, logger *log.Logger, version string) (*Service, error) {
	redis, err := redisClient.New(cfg.RedisURL, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create Redis client: %v", err)
	}

	curve, err := backlight.ParseCurve(cfg.Curve)
	if err != nil {
		return nil, fmt.Errorf("invalid curve: %v", err)
	}

	levels, err := backlight.ParseLevels(cfg.ManualLevels)
	if err != nil {
		return nil, fmt.Errorf("invalid manual-levels: %v", err)
	}

	logger.Printf("Backlight curve: %v", curve)

	backlightManager := backlight.New(
		cfg.SysBacklightPath,
		logger,
		curve,
		cfg.RampRate,
		cfg.LuxAlpha,
	)

	service := &Service{
		Config:                  cfg,
		Redis:                   redis,
		Logger:                  logger,
		Backlight:               backlightManager,
		lastPublishedBrightness: -1,
		lastPublishedLux:        -1,
		luxPublishMinDelta:      0.5,
		publishMinInterval:      250 * time.Millisecond,
		lastLoggedTarget:        -1,
		overrideCh:              make(chan struct{}, 1),
		manualLevels:            levels,
		backlightMode:           "auto",
		modeCh:                  make(chan struct{}, 1),
	}

	service.Logger.Printf("dbc-backlight-service %s", version)

	return service, nil
}

func (s *Service) Run(ctx context.Context) error {
	defer s.Redis.Close()

	source := "redis"
	if s.Config.SensorPath != "" {
		source = s.Config.SensorPath
	}
	s.Logger.Printf("Starting backlight service (ramp=%v/%.0f%%, sensor=%v, source=%s)",
		s.Config.PollingTime, s.Config.RampRate*100, s.Config.SensorInterval, source)
	s.Logger.Printf("Using backlight path: %s", s.Config.SysBacklightPath)

	go s.rampLoop(ctx)
	go s.sensorLoop(ctx)
	go s.subscribeOverride(ctx)

	<-ctx.Done()
	return nil
}

// rampLoop advances the brightness ramp. Every step is a cheap in-memory
// calculation plus at most one small sysfs write, so it can run at the
// configured tick rate without the sensor holding it up.
func (s *Service) rampLoop(ctx context.Context) {
	ticker := time.NewTicker(s.Config.PollingTime)
	defer ticker.Stop()

	s.checkOverride(ctx)
	s.refreshMode(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.overrideCh:
			s.checkOverride(ctx)
		case <-s.modeCh:
			s.refreshMode(ctx)
		case <-ticker.C:
			if s.backlightDisabled.Load() {
				continue
			}
			if err := s.Backlight.Tick(); err != nil {
				s.Logger.Printf("Failed to write backlight: %v", err)
			}
		}
	}
}

// sensorLoop samples ambient light at the rate the hardware can sustain. The
// OPT3001 has no data-ready interrupt wired on the DBC, so a read blocks for
// the whole integration time (~1s at the 0.8s setting the unit file selects).
// SensorInterval is therefore a floor, not a guarantee.
func (s *Service) sensorLoop(ctx context.Context) {
	for {
		start := time.Now()

		lux, err := s.readLux(ctx)
		if err != nil {
			s.Logger.Printf("Failed to read illuminance: %v", err)
		} else {
			s.Backlight.SetLux(lux)
			s.publish(ctx, lux)
		}

		// Pace to SensorInterval, minus however long the read already took.
		wait := s.Config.SensorInterval - time.Since(start)
		if wait <= 0 {
			wait = time.Millisecond
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

func (s *Service) subscribeOverride(ctx context.Context) {
	pubsub := s.Redis.Subscribe(ctx, "dashboard", "settings")
	defer pubsub.Close()

	// Signal initial checks
	s.signal(s.overrideCh)
	s.signal(s.modeCh)

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			switch msg.Payload {
			case "backlight-enabled":
				s.signal(s.overrideCh)
			case "dashboard.backlight-mode":
				s.signal(s.modeCh)
			}
		}
	}
}

func (s *Service) signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (s *Service) refreshMode(ctx context.Context) {
	mode, err := s.Redis.GetBacklightMode(ctx)
	if err != nil {
		s.Logger.Printf("Failed to read backlight mode: %v", err)
		return
	}
	if mode == s.backlightMode {
		return
	}
	s.backlightMode = mode
	s.Logger.Printf("Backlight mode: %s", mode)

	if level, manual := s.manualLevels[mode]; manual {
		if err := s.Backlight.SetManual(level); err != nil {
			s.Logger.Printf("Failed to set manual backlight: %v", err)
		}
		return
	}
	s.Backlight.SetAuto()
}

func (s *Service) readLux(ctx context.Context) (float64, error) {
	if s.Config.SensorPath != "" {
		return s.readSensor()
	}
	return s.Redis.GetIlluminanceValue(ctx)
}

func (s *Service) readSensor() (float64, error) {
	data, err := os.ReadFile(s.Config.SensorPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read sensor: %v", err)
	}
	return strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
}

func (s *Service) checkOverride(ctx context.Context) {
	enabled, err := s.Redis.GetBacklightEnabled(ctx)
	if err != nil {
		s.Logger.Printf("Failed to check backlight-enabled: %v", err)
		return
	}
	if !enabled && !s.backlightDisabled.Load() {
		s.backlightDisabled.Store(true)
		if err := s.Backlight.ForceOff(); err != nil {
			s.Logger.Printf("Failed to force backlight off: %v", err)
		} else {
			s.Logger.Printf("Backlight disabled")
		}
	} else if enabled && s.backlightDisabled.Load() {
		s.backlightDisabled.Store(false)
		s.Logger.Printf("Backlight enabled, resuming auto-adjustment")
	}
}

// publish mirrors the sample and the resulting brightness into Redis. Both are
// rate-limited by magnitude, and the pair by time: sensor noise alone clears
// the lux delta on nearly every sample, so without the interval this would put
// a write and a publish on the wire at the full sample rate. scootui-qt drives
// the auto light/dark theme off the lux field, so it keeps flowing even while
// the backlight is overridden off and even in a manual backlight mode.
func (s *Service) publish(ctx context.Context, lux float64) {
	if !s.lastPublishedAt.IsZero() && time.Since(s.lastPublishedAt) < s.publishMinInterval {
		return
	}
	s.lastPublishedAt = time.Now()

	if s.Config.Debug {
		target := s.Backlight.Target()
		delta := target - s.lastLoggedTarget
		if delta < 0 {
			delta = -delta
		}
		if delta >= 100 || s.lastLoggedTarget < 0 {
			s.Logger.Printf("lux=%.1f mode=%s -> target %d (output %d)", lux, s.backlightMode, target, s.Backlight.Output())
			s.lastLoggedTarget = target
		}
	}

	// Publish lux to Redis if reading from sensor directly
	if s.Config.SensorPath != "" {
		luxDelta := lux - s.lastPublishedLux
		if luxDelta < 0 {
			luxDelta = -luxDelta
		}
		if s.lastPublishedLux < 0 || luxDelta >= s.luxPublishMinDelta {
			if err := s.Redis.SetIlluminanceValue(ctx, lux); err != nil {
				s.Logger.Printf("Warning: Failed to publish lux to Redis: %v", err)
			}
			s.lastPublishedLux = lux
		}
	}

	// Publish backlight to Redis. The settled value always gets published, so
	// the last word is the level actually on screen rather than whatever the
	// ramp happened to be passing through.
	brightness := s.Backlight.Output()
	bDelta := brightness - s.lastPublishedBrightness
	if bDelta < 0 {
		bDelta = -bDelta
	}
	settled := brightness == s.Backlight.Target() && brightness != s.lastPublishedBrightness

	if bDelta >= 100 || settled || s.lastPublishedBrightness == -1 {
		if err := s.Redis.SetBacklightValue(ctx, brightness); err != nil {
			s.Logger.Printf("Warning: Failed to write backlight value to Redis: %v", err)
		} else {
			s.lastPublishedBrightness = brightness
		}
	}
}
