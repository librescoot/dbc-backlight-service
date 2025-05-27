# DBC Backlight Service

A Go-based service that adjusts the backlight of the Dashboard Controller based on the value of Redis key `HGET dashboard brightness`. The service writes the configured backlight brightness to `HSET dashboard backlight` in Redis.

## Features

- Dynamically adjusts backlight brightness based on ambient light readings
- Uses device tree brightness levels: 0x00, 0x800, 0x1000, 0x2000, 0x4000, 0xffff
- Configurable illuminance thresholds for brightness level transitions
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
- `--polling-time`: Polling interval for illuminance value (default: 1s)
- `--backlight-path`: Path to backlight brightness file (default: "/sys/class/backlight/backlight/brightness")
- `--base-illuminance`: Base illuminance threshold (lux) for the brightness formula (default: 15.0).
- `--base-brightness`: Brightness value set when illuminance is at or below `--base-illuminance` (default: 8192).
- `--lux-multiplier`: The factor by which illuminance must increase (beyond `--base-illuminance`) for the brightness to step up. This acts as the base of the logarithm in the brightness formula (default: 3.0).
- `--brightness-increment`: The amount of brightness added for each step defined by the `--lux-multiplier` (default: 1024).

## Brightness Adjustment Formula

The service dynamically adjusts backlight brightness based on a mathematical formula controlled by the configuration flags mentioned above. This provides a continuous and configurable brightness curve.

The formula is as follows:

1.  **If current illuminance <= `--base-illuminance`**:
    *   `Target Brightness = --base-brightness`

2.  **If current illuminance > `--base-illuminance`**:
    *   Let `I_current` be the current illuminance.
    *   Let `I_base` be `--base-illuminance`.
    *   Let `B_base` be `--base-brightness`.
    *   Let `M_lux` be `--lux-multiplier`.
    *   Let `B_inc` be `--brightness-increment`.
    *   The number of "steps" (`n`) is calculated as: `n = log_M_lux(I_current / I_base)`
    *   `Target Brightness = B_base + n * B_inc`

The calculated `Target Brightness` is rounded to the nearest integer and capped at a maximum hardware value of 65535. It is also ensured that if illuminance is above `--base-illuminance`, the target brightness will not be less than `--base-brightness`.

## Redis Keys

- **Read**: `HGET dashboard brightness` - Gets the current illuminance value
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
