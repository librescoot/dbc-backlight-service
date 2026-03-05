package service

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
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
	lastLoggedTarget        int
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

	logger.Printf("Backlight curve: %v", curve)

	backlightManager := backlight.New(
		cfg.SysBacklightPath,
		logger,
		curve,
		cfg.RampRate,
	)

	service := &Service{
		Config:                  cfg,
		Redis:                   redis,
		Logger:                  logger,
		Backlight:               backlightManager,
		lastPublishedBrightness: -1,
		lastPublishedLux:        -1,
		luxPublishMinDelta:      0.5,
		lastLoggedTarget:        -1,
	}

	service.Logger.Printf("dbc-backlight-service %s", version)

	return service, nil
}

func (s *Service) Run(ctx context.Context) error {
	defer s.Redis.Close()

	if err := s.Redis.Ping(ctx); err != nil {
		return fmt.Errorf("redis connection failed: %v", err)
	}

	mode := "redis"
	if s.Config.SensorPath != "" {
		mode = s.Config.SensorPath
	}
	s.Logger.Printf("Starting backlight service (poll=%v, ramp=%.0f%%, source=%s)",
		s.Config.PollingTime, s.Config.RampRate*100, mode)
	s.Logger.Printf("Using backlight path: %s", s.Config.SysBacklightPath)

	go s.monitorIlluminance(ctx)

	<-ctx.Done()
	return nil
}

func (s *Service) monitorIlluminance(ctx context.Context) {
	ticker := time.NewTicker(s.Config.PollingTime)
	defer ticker.Stop()

	s.adjustBacklight(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.adjustBacklight(ctx)
		}
	}
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

func (s *Service) adjustBacklight(ctx context.Context) {
	lux, err := s.readLux(ctx)
	if err != nil {
		s.Logger.Printf("Failed to read illuminance: %v", err)
		return
	}

	if err := s.Backlight.AdjustBacklight(lux); err != nil {
		s.Logger.Printf("Failed to adjust backlight: %v", err)
		return
	}

	if s.Config.Debug {
		target := s.Backlight.Target()
		delta := target - s.lastLoggedTarget
		if delta < 0 {
			delta = -delta
		}
		if delta >= 100 || s.lastLoggedTarget < 0 {
			s.Logger.Printf("lux=%.1f → target %d (output %d)", lux, target, s.Backlight.Output())
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

	// Publish backlight to Redis
	brightness := s.Backlight.Output()
	bDelta := brightness - s.lastPublishedBrightness
	if bDelta < 0 {
		bDelta = -bDelta
	}

	if bDelta >= 100 || s.lastPublishedBrightness == -1 {
		if err := s.Redis.SetBacklightValue(ctx, brightness); err != nil {
			s.Logger.Printf("Warning: Failed to write backlight value to Redis: %v", err)
		} else {
			s.lastPublishedBrightness = brightness
		}
	}
}
