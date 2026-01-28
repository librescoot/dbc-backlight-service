# DBC Backlight Service

A Go-based service that adjusts the backlight of the Dashboard Controller based on the value of Redis key `HGET dashboard brightness`. The service writes the configured backlight brightness to `HSET dashboard backlight` in Redis.

## Features

- Dynamically adjusts backlight brightness based on ambient light readings
- 5-level discrete state machine with hysteresis to prevent oscillation
- Fully configurable brightness values and transition thresholds
- Reads illuminance from Redis and publishes backlight state back to Redis
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

### Basic Configuration
- `--redis-url`: Redis URL (default: "redis://192.168.7.1:6379")
- `--polling-time`: Polling interval for illuminance value (default: 1s)
- `--backlight-path`: Path to backlight brightness file (default: "/sys/class/backlight/backlight/brightness")
- `--hysteresis-threshold`: Minimum brightness change to trigger Redis update (default: 512)

### Brightness Levels
- `--very-low-brightness`: Brightness for VERY_LOW state (default: 9350)
- `--low-brightness`: Brightness for LOW state (default: 9500)
- `--mid-brightness`: Brightness for MID state (default: 9700)
- `--high-brightness`: Brightness for HIGH state (default: 9950)
- `--very-high-brightness`: Brightness for VERY_HIGH state (default: 10240)

### Upward Transition Thresholds (lux)
- `--very-low-to-low-threshold`: Transition VERY_LOW → LOW (default: 8)
- `--low-to-mid-threshold`: Transition LOW → MID (default: 18)
- `--mid-to-high-threshold`: Transition MID → HIGH (default: 40)
- `--high-to-very-high-threshold`: Transition HIGH → VERY_HIGH (default: 80)

### Downward Transition Thresholds (lux)
- `--low-to-very-low-threshold`: Transition LOW → VERY_LOW (default: 5)
- `--mid-to-low-threshold`: Transition MID → LOW (default: 15)
- `--high-to-mid-threshold`: Transition HIGH → MID (default: 35)
- `--very-high-to-high-threshold`: Transition VERY_HIGH → HIGH (default: 70)

## Brightness State Machine

The service uses a discrete 5-state state machine with hysteresis to adjust backlight brightness smoothly while preventing rapid oscillation.

### States and Brightness Levels

| State | Default Brightness | Use Case |
|-------|-------------------|----------|
| VERY_LOW | 9350 | Dark room, night |
| LOW | 9500 | Dim indoor, evening |
| MID | 9700 | Normal indoor, cloudy day |
| HIGH | 9950 | Outdoor shade, indirect sun |
| VERY_HIGH | 10240 | Direct sunlight |

### State Transitions

The state machine includes hysteresis gaps between upward and downward thresholds to prevent rapid state changes:

```
VERY_LOW (9350)
  ↑ when lux > 8

LOW (9500)
  ↑ when lux > 18
  ↓ when lux < 5

MID (9700)
  ↑ when lux > 40
  ↓ when lux < 15

HIGH (9950)
  ↑ when lux > 80
  ↓ when lux < 35

VERY_HIGH (10240)
  ↓ when lux < 70
```

**Hysteresis gaps** (prevents oscillation):
- VERY_LOW ↔ LOW: 5-8 lux (3 lux gap)
- LOW ↔ MID: 15-18 lux (3 lux gap)
- MID ↔ HIGH: 35-40 lux (5 lux gap)
- HIGH ↔ VERY_HIGH: 70-80 lux (10 lux gap)

The service reads the current hardware brightness at startup to determine its
initial state. If the backlight file cannot be read, it defaults to MID.

## Redis Keys

- **Read**: `HGET dashboard brightness` - Ambient light sensor reading (lux) from dbc-illumination-service
- **Write**: `HSET dashboard backlight <value>` - Current backlight brightness value set by this service

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
