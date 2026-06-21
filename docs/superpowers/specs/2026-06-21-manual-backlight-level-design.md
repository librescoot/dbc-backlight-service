# Manual backlight level setting

Bean: librescoot-g363

## Goal

Let the DBC user pin the display backlight to a fixed level (Low / Medium / High)
or keep the existing automatic ambient-light adjustment (Auto). Exposed in the
scootui-qt settings menu, persisted like any other setting.

The ambient-light-driven dark/light *theme* switch stays independent: it keeps
running off the lux sensor regardless of backlight mode, still controlled by the
separate `dashboard.theme` setting.

## Setting

One new enum in the `settings` hash, modeled exactly on `dashboard.theme`:

- Key: `dashboard.backlight-mode`
- Values: `auto`, `low`, `medium`, `high`
- Default: `auto`
- user-visible: true

`off` is intentionally excluded. The system already blanks the screen via the
existing `dashboard.backlight-enabled=false` override (power management / idle),
so a user-facing "off" would be redundant and easy to get stuck in.

Level -> raw sysfs brightness (max ~10240), tunable on-target, no rebuild:

| Level  | Brightness |
|--------|-----------|
| low    | 1300      |
| medium | 4000      |
| high   | 10240     |

## Changes per repo

### settings-service

Add one entry to `settings.schema.json`, mirroring `dashboard.theme`
(lines ~797-817):

```json
"dashboard.backlight-mode": {
  "type": "enum",
  "description": "Display backlight",
  "label": "Backlight",
  "user-visible": true,
  "service": "dbc-backlight",
  "default": "auto",
  "values": [
    {"value": "auto",   "label": "Auto"},
    {"value": "low",    "label": "Low"},
    {"value": "medium", "label": "Medium"},
    {"value": "high",   "label": "High"}
  ]
}
```

No Go code change: the generic watch/persist/default path (`WatchSettings`,
`SaveSettingsToTOML`, `schema.Defaults()`) handles it.

### dbc-backlight-service

The brightness *target* becomes mode-aware. The lux read/publish loop is
untouched, so theme auto-switch and other `dashboard.brightness` consumers keep
working.

- **config** (`internal/config/config.go`): add a tunable level map flag,
  mirroring `-curve`:
  `-manual-levels "low:1300 medium:4000 high:10240"` (default as shown).
  Parse into an ordered `map[string]int` (reuse the `lux:brightness` split shape).

- **redis** (`internal/redis/redis.go`): add
  `GetBacklightMode(ctx) (string, error)` -> `HGET settings dashboard.backlight-mode`,
  returning `"auto"` when the key is missing (`redis.Nil`).

- **subscribe** (`internal/service/service.go`): in addition to the `dashboard`
  channel, subscribe to the `settings` channel. On a payload of
  `dashboard.backlight-mode`, signal a re-read of the mode (store it on the
  Service, guarded by the existing single-goroutine `monitorIlluminance` flow via
  the `overrideCh` pattern, or a dedicated channel). Read the mode once at
  startup.

- **output decision** (`adjustBacklight`):
  - Always read lux and publish it (unchanged), and keep the
    `backlight-enabled=false` early-return blanking ahead of everything.
  - If mode == `auto`: existing lux -> curve interpolation -> ramp.
  - If mode is a fixed level: set the manager target to the mapped brightness and
    ramp toward it (smooth, no snap/flash). Add `Manager.SetManualTarget(level int)`
    (or pass the fixed target into `AdjustBacklight`) that drives the existing ramp
    machinery instead of the curve.

- The manager already ramps from `output` to `target`; a manual level just feeds a
  constant target, so transitions between levels (and between auto and manual) are
  smooth for free.

### scootui-qt

Mirror the theme wiring:

- `src/stores/SettingsStore.{h,cpp}`: add `Q_PROPERTY(QString backlightMode ...)`,
  sync field `{name: "backlightMode", variable: "dashboard.backlight-mode"}`,
  `applyFieldUpdate` handler emitting `backlightModeChanged()`.
- Settings menu QML (the screen that renders the Theme entry): add a sibling entry
  cycling `Auto / Low / Medium / High`, writing via the existing
  `RedisMdbRepository::set("settings", "dashboard.backlight-mode", v)` path
  (HSET + PUBLISH settings).
- Simulator (`qml/simulator/SimulatorWindow.qml` + `SimulatorService`): add matching
  buttons next to the theme buttons for on-desk testing.

## Data flow

UI selection
-> `set("settings","dashboard.backlight-mode", v)` (HSET + PUBLISH settings)
-> settings-service persists to `/data/settings.toml`, republishes
-> dbc-backlight-service re-reads mode, picks fixed target or auto
-> ramps sysfs brightness.

Lux read/publish loop unchanged -> theme auto-switch unaffected.

## Out of scope

- No change to the dark/light theme logic.
- No change to `backlight-enabled` semantics (system blank stays as-is).
- No new mobile-app surface (settings-hash write is enough if added later).

## Testing

- dbc-backlight-service: unit-test the level-map parse and the auto-vs-manual
  target selection. On deep-blue DBC: set each mode via
  `redis-cli -h 192.168.7.1 HSET settings dashboard.backlight-mode high` +
  `PUBLISH settings dashboard.backlight-mode`, confirm sysfs brightness ramps to
  the mapped value and that `auto` resumes ambient tracking.
- Confirm theme still auto-switches while a manual backlight level is active.
- scootui-qt: settings entry shows current value, cycling writes the hash; verify
  in the simulator.
