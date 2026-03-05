package service

import (
	"context"
	"fmt"
	"log"
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
		cfg.MinDelta,
	)

	service := &Service{
		Config:                  cfg,
		Redis:                   redis,
		Logger:                  logger,
		Backlight:               backlightManager,
		lastPublishedBrightness: -1,
	}

	service.Logger.Printf("dbc-backlight-service v%s", version)

	return service, nil
}

func (s *Service) Run(ctx context.Context) error {
	defer s.Redis.Close()

	if err := s.Redis.Ping(ctx); err != nil {
		return fmt.Errorf("redis connection failed: %v", err)
	}

	s.Logger.Printf("Starting backlight service with polling interval %v", s.Config.PollingTime)
	s.Logger.Printf("Using backlight path: %s", s.Config.SysBacklightPath)

	go s.monitorIlluminance(ctx)

	<-ctx.Done()
	return nil
}

func (s *Service) monitorIlluminance(ctx context.Context) {
	ticker := time.NewTicker(s.Config.PollingTime)
	defer ticker.Stop()

	initCtx, initCancel := context.WithTimeout(ctx, 5*time.Second)
	if err := s.adjustBacklight(initCtx); err != nil {
		s.Logger.Printf("Initial backlight adjustment failed: %v", err)
	}
	initCancel()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			adjustCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			if err := s.adjustBacklight(adjustCtx); err != nil {
				s.Logger.Printf("Backlight adjustment failed: %v", err)
			}
			cancel()
		}
	}
}

func (s *Service) adjustBacklight(ctx context.Context) error {
	lux, err := s.Redis.GetIlluminanceValue(ctx)
	if err != nil {
		return fmt.Errorf("failed to get illuminance value: %v", err)
	}

	if err := s.Backlight.AdjustBacklight(lux); err != nil {
		return fmt.Errorf("failed to adjust backlight: %v", err)
	}

	brightness, err := s.Backlight.GetCurrentBrightness()
	if err != nil {
		s.Logger.Printf("Warning: Failed to read current brightness: %v", err)
		return nil
	}

	delta := brightness - s.lastPublishedBrightness
	if delta < 0 {
		delta = -delta
	}

	if delta >= s.Config.MinDelta || s.lastPublishedBrightness == -1 {
		if err := s.Redis.SetBacklightValue(ctx, brightness); err != nil {
			s.Logger.Printf("Warning: Failed to write backlight value to Redis: %v", err)
		} else {
			s.lastPublishedBrightness = brightness
		}
	}

	return nil
}
