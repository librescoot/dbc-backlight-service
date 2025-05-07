# DBC Backlight Service

A Go-based service that adjusts the backlight of the Dashboard Controller based on the value of Redis key `HGET dashboard illumination`. The service writes the configured backlight brightness to `HSET dashboard backlight` in Redis.

## Features

- Dynamically adjusts backlight brightness based on ambient light readings
- Uses device tree brightness levels: 0x00, 0x800, 0x1000, 0x2000, 0x4000, 0xffff
- Configurable illumination thresholds for brightness level transitions
- Multiple brightness levels with intelligent transitions
- Persists backlight state to Redis
- Systemd integration

## Building

```bash
# Build for development
make build

# Build for ARM target
make arm

# Build for distribution (optimized)
make dist

# Clean build artifacts
make clean
```

## Configuration

The service supports the following configuration flags:

- `--redis-url`: Redis URL (default: "redis://192.168.7.1:6379")
- `--polling-time`: Polling interval for illumination value (default: 1s)
- `--backlight-path`: Path to backlight brightness file (default: "/sys/class/backlight/backlight/brightness")

## Brightness Levels

The service uses the device tree's brightness levels that are defined as:

| Level Name | Brightness Value | Illumination Range |
|------------|------------------|-------------------|
| OFF        | 0x00             | 0-5               |
| VERY_LOW   | 0x800            | 5-15              |
| LOW        | 0x1000           | 15-30             |
| MEDIUM     | 0x2000           | 30-45             |
| HIGH       | 0x4000           | 45-60             |
| MAX        | 0xffff           | 60+               |

The service dynamically adjusts the backlight level based on the current illumination value from Redis. It transitions between levels when the illumination crosses the defined thresholds.

## Redis Keys

- **Read**: `HGET dashboard illumination` - Gets the current illumination value
- **Write**: `HSET dashboard backlight <value>` - Sets the current backlight value

## Installation

1. Build the service for ARM target:
   ```bash
   make dist
   ```

2. Copy the binary to the target system:
   ```bash
   scp dbc-backlight-arm-dist user@target:/tmp/
   ```

3. Install the binary and service file:
   ```bash
   # On target system
   sudo mv /tmp/dbc-backlight-arm-dist /usr/bin/dbc-backlight
   sudo chmod +x /usr/bin/dbc-backlight
   sudo cp dbc-backlight.service /etc/systemd/system/
   sudo systemctl daemon-reload
   sudo systemctl enable dbc-backlight.service
   sudo systemctl start dbc-backlight.service
   ```

## License

[CC-BY-NC-SA-4.0](LICENSE)