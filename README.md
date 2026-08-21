# DBC Backlight Service

Adjusts the Dashboard Controller's display backlight from the ambient light
sensor, and mirrors both the sensor reading and the resulting backlight level
into Redis.

Part of the [Librescoot](https://librescoot.org/) open-source platform.

## How it works

The DBC carries a TI OPT3001 ambient light sensor on I2C, exposed through IIO at
`/sys/bus/iio/devices/iio:device0/in_illuminance_input`. There is no data-ready
interrupt wired, so the driver sleeps through the whole conversion and a read
blocks for the integration time: about a second at the 0.8s setting the unit
file selects.

The service therefore runs two loops.

The sensor loop samples at whatever rate the hardware sustains (roughly 1 Hz),
smooths the reading with an EMA, maps it through a lux-to-brightness curve, and
moves the ramp target if the result shifted by more than the deadband. It also
publishes to Redis.

The ramp loop runs far faster (50ms by default) and does nothing but step the
output a fraction of the remaining distance toward the target and write it to
sysfs. Keeping it off the sensor loop is what makes the fade smooth: at the
default 5% per step a full-range change lands in about 7 seconds instead of the
two minutes it would take at the sensor's own rate.

### Curve

Brightness runs 0 to 10240, which is the interpolated step count the
`pwm-backlight` node exposes (`brightness-levels` with `num-interpolated-steps`
set to 2048), not a raw duty cycle. The default curve:

| lux | 0 | 0.5 | 1 | 2 | 5 | 10 | 20 | 35 | 50 | 80 |
|---|---|---|---|---|---|---|---|---|---|---|
| brightness | 400 | 1300 | 2200 | 2900 | 4000 | 5200 | 7000 | 8600 | 9600 | 10240 |

Values in between are linearly interpolated; outside the ends they clamp. The
whole dynamic range sits below 80 lux, so in any daylight the backlight is
pinned at maximum and the curve only does real work at night, in tunnels and
indoors.

### Modes and overrides

`settings[dashboard.backlight-mode]` picks the mode. `auto` runs the curve;
`low`, `medium` and `high` pin a fixed level and snap to it, since a manual pick
is a deliberate choice and should not fade. Ambient sampling continues in the
fixed modes, both to keep the EMA warm for the switch back and because
scootui-qt drives its automatic light/dark theme off the published lux.

`dashboard[backlight-enabled]` is a hard override. Setting it false writes 0 and
holds there; setting it true snaps back to the ambient level rather than ramping
up from black. vehicle-service asserts it on entering parked and ready-to-drive,
scootui-qt clears it for the hop-on lock overlay and the OTA and maintenance
screens, and `lsc backlight on|off` drives it by hand.

## Building

```bash
make build        # ARM (default)
make build-host   # current platform
make dist         # stripped ARM binary
make test
```

## Configuration

| Flag | Default | Meaning |
|---|---|---|
| `--redis-url` | `redis://192.168.7.1:6379` | Redis URL |
| `--sensor-path` | (empty) | IIO illuminance input. Empty reads lux from Redis instead. |
| `--sensor-interval` | `1s` | Floor on the gap between samples. The blocking read usually exceeds it. |
| `--polling-time` | `50ms` | Interval between ramp steps |
| `--ramp-rate` | `0.05` | Fraction of the remaining distance per ramp step |
| `--lux-alpha` | `0.1` | EMA weight applied per lux sample; lower is slower and less flickery |
| `--backlight-path` | `/sys/class/backlight/backlight/brightness` | sysfs brightness file |
| `--curve` | see above | `lux:brightness` pairs, whitespace separated |
| `--manual-levels` | `low:1300 medium:4000 high:10240` | `name:brightness` pairs for the fixed modes |
| `--debug` | false | Log target changes |

## Redis keys

Read:

- `dashboard[backlight-enabled]`, override; absent means enabled
- `settings[dashboard.backlight-mode]`, one of auto/low/medium/high; absent means auto
- `dashboard[brightness]`, only when `--sensor-path` is not set

Written, each followed by a `PUBLISH` on the owning channel:

- `dashboard[brightness]`, the raw lux sample, when it moves by 0.5 or more
- `dashboard[backlight]`, the current output, when it moves by 100 or more and once it settles

The service subscribes to `dashboard` and `settings` and reacts to the
`backlight-enabled` and `dashboard.backlight-mode` payloads.

## Installation

The unit is `librescoot-backlight.service`, packaged by meta-librescoot. To
replace the binary on a running DBC:

```bash
tar -C bin -cf - dbc-backlight | ssh deep-blue "ssh root@192.168.7.2 'tar -C /data -xf -'"
ssh -J deep-blue root@192.168.7.2 \
  "systemctl stop librescoot-backlight && cp /data/dbc-backlight /usr/bin/backlight-service && systemctl start librescoot-backlight && sync"
```

## License

This project is dual-licensed. The source code is available under the
[Creative Commons Attribution-NonCommercial-ShareAlike 4.0 International License][cc-by-nc-sa].
The maintainers reserve the right to grant separate licenses for commercial distribution; please contact the maintainers to discuss commercial licensing.

[![CC BY-NC-SA 4.0][cc-by-nc-sa-image]][cc-by-nc-sa]

[cc-by-nc-sa]: http://creativecommons.org/licenses/by-nc-sa/4.0/
[cc-by-nc-sa-image]: https://licensebuttons.net/l/by-nc-sa/4.0/88x31.png
