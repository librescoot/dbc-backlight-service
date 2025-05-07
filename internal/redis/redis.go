package redis

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"github.com/redis/go-redis/v9"
)

type Client struct {
	client *redis.Client
	logger *log.Logger
}

func New(redisURL string, logger *log.Logger) (*Client, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("invalid redis URL: %v", err)
	}

	client := redis.NewClient(opt)
	return &Client{
		client: client,
		logger: logger,
	}, nil
}

func (c *Client) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *Client) GetIlluminationValue(ctx context.Context) (int, error) {
	result, err := c.client.HGet(ctx, "dashboard", "illumination").Result()
	if err != nil {
		if err == redis.Nil {
			c.logger.Printf("Illumination value not found in Redis")
			return 0, nil
		}
		return 0, fmt.Errorf("failed to get illumination value: %v", err)
	}

	value, err := strconv.Atoi(result)
	if err != nil {
		return 0, fmt.Errorf("invalid illumination value: %v", err)
	}

	return value, nil
}

func (c *Client) SetBacklightValue(ctx context.Context, value int) error {
	pipe := c.client.Pipeline()
	pipe.HSet(ctx, "dashboard", "backlight", value)
	pipe.Publish(ctx, "dashboard", "backlight")
	_, err := pipe.Exec(ctx)
	if err != nil {
		c.logger.Printf("Unable to set backlight value in Redis: %v", err)
		return fmt.Errorf("cannot write to Redis: %v", err)
	}
	return nil
}

func (c *Client) Close() error {
	return c.client.Close()
}