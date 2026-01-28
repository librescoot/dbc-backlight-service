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
	lastUpdate              time.Time
	lastPublishedBrightness int
}

func New(cfg *config.Config, logger *log.Logger, version string) (*Service, error) {
	redis, err := redisClient.New(cfg.RedisURL, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create Redis client: %v", err)
	}

	backlightManager := backlight.New(
		cfg.SysBacklightPath,
		logger,
		cfg.VeryLowBrightness,
		cfg.LowBrightness,
		cfg.MidBrightness,
		cfg.HighBrightness,
		cfg.VeryHighBrightness,
		cfg.VeryLowToLowThreshold,
		cfg.LowToMidThreshold,
		cfg.MidToHighThreshold,
		cfg.HighToVeryHighThreshold,
		cfg.LowToVeryLowThreshold,
		cfg.MidToLowThreshold,
		cfg.HighToMidThreshold,
		cfg.VeryHighToHighThreshold,
	)

	service := &Service{
		Config:                  cfg,
		Redis:                   redis,
		Logger:                  logger,
		Backlight:               backlightManager,
		lastUpdate:              time.Now(),
		lastPublishedBrightness: -1, // Initialize to -1 to ensure first reading triggers update
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

	// Start the main monitoring loop
	go s.monitorIlluminance(ctx)

	<-ctx.Done()
	return nil
}

func (s *Service) monitorIlluminance(ctx context.Context) {
	ticker := time.NewTicker(s.Config.PollingTime)
	defer ticker.Stop()

	// Initial reading and adjustment
	initCtx, initCancel := context.WithTimeout(ctx, 5*time.Second)
	if err := s.adjustBacklightBasedOnIlluminance(initCtx); err != nil {
		s.Logger.Printf("Initial backlight adjustment failed: %v", err)
	}
	initCancel()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			adjustCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			if err := s.adjustBacklightBasedOnIlluminance(adjustCtx); err != nil {
				s.Logger.Printf("Periodic backlight adjustment failed: %v", err)
			}
			cancel()
		}
	}
}

func (s *Service) adjustBacklightBasedOnIlluminance(ctx context.Context) error {
	// Get illuminance value from Redis
	illuminance, err := s.Redis.GetIlluminanceValue(ctx)
	if err != nil {
		return fmt.Errorf("failed to get illuminance value: %v", err)
	}

	// Adjust backlight based on illuminance
	if err := s.Backlight.AdjustBacklight(illuminance); err != nil {
		return fmt.Errorf("failed to adjust backlight: %v", err)
	}

	// Get current brightness after adjustment
	brightness, err := s.Backlight.GetCurrentBrightness()
	if err != nil {
		s.Logger.Printf("Warning: Failed to read current brightness: %v", err)
		return nil
	}

	// Apply hysteresis: only update Redis if brightness changed significantly
	brightnessChange := brightness - s.lastPublishedBrightness
	if brightnessChange < 0 {
		brightnessChange = -brightnessChange
	}

	if brightnessChange >= s.Config.HysteresisThreshold || s.lastPublishedBrightness == -1 {
		// Write backlight value to Redis
		if err := s.Redis.SetBacklightValue(ctx, brightness); err != nil {
			s.Logger.Printf("Warning: Failed to write backlight value to Redis: %v", err)
		} else {
			s.Logger.Printf("Brightness changed from %d to %d (delta: %d) - updating Redis",
				s.lastPublishedBrightness, brightness, brightnessChange)
			s.lastPublishedBrightness = brightness
		}
	}

	return nil
}
