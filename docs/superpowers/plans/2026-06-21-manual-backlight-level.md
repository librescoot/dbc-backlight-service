# Manual Backlight Level Setting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the DBC user pin the display backlight to a fixed level (Low/Medium/High) or keep automatic ambient adjustment (Auto), via a new `dashboard.backlight-mode` setting.

**Architecture:** One enum setting in the `settings` hash, modeled on `dashboard.theme`. settings-service gets a schema entry only. dbc-backlight-service makes the brightness *target* mode-aware (fixed level vs lux curve) while leaving the lux read/publish loop untouched. scootui-qt mirrors the Theme menu entry. The dark/light theme auto-switch and the system `backlight-enabled` blank are both unaffected.

**Tech Stack:** Go 1.25.7 (dbc-backlight-service), JSON schema (settings-service), Qt6/C++/QML (scootui-qt), Redis hashes + pub/sub.

## Global Constraints

- Spec: `dbc-backlight-service/docs/superpowers/specs/2026-06-21-manual-backlight-level-design.md`. Bean: `librescoot-g363`.
- Setting key: `dashboard.backlight-mode`. Values: `auto`, `low`, `medium`, `high`. Default `auto`. No `off` value.
- Level mapping (raw sysfs brightness, max ~10240): `low=1300`, `medium=4000`, `high=10240`. Tunable via flag, no rebuild.
- Go builds: `GOTOOLCHAIN=go1.25.7`. ARM build via `make build`; host build via `go build ./...`.
- Conventional commit messages. No mention of Claude / Claude Code. No em/en dashes anywhere. Do not commit bean files or CLAUDE.md. Pass explicit file paths to `git commit` (never `git add .`).
- Each repo is a separate git repo. `cd` into the correct repo before any git/build command; verify with `git -C <repo> rev-parse --show-toplevel` if unsure.
- Theme auto-switch stays independent: never gate the lux read/publish on backlight mode.

---

## Repo 1: settings-service (schema only)

### Task 1: Add `dashboard.backlight-mode` to the settings schema

**Files:**
- Modify: `/home/teal/src/librescoot/settings-service/settings.schema.json` (insert a new block immediately after the `dashboard.theme` block, which ends at the line `},` following its `"example": "auto"` around line 819)

**Interfaces:**
- Produces: the `settings` hash field `dashboard.backlight-mode`, defaulting to `auto`, consumed by Repo 2 (dbc-backlight-service) and Repo 3 (scootui-qt).

- [ ] **Step 1: Insert the schema block**

Insert after the closing `},` of the `"dashboard.theme"` entry:

```json
  "dashboard.backlight-mode": {
    "type": "enum",
    "description": "Display backlight level (auto = ambient light sensor)",
    "label": "Backlight",
    "user-visible": true,
    "service": "dbc-backlight",
    "default": "auto",
    "values": [
      {
        "value": "auto",
        "label": "Auto"
      },
      {
        "value": "low",
        "label": "Low"
      },
      {
        "value": "medium",
        "label": "Medium"
      },
      {
        "value": "high",
        "label": "High"
      }
    ],
    "example": "auto"
  },
```

- [ ] **Step 2: Verify the JSON is still valid**

Run: `python3 -m json.tool /home/teal/src/librescoot/settings-service/settings.schema.json > /dev/null && echo OK`
Expected: prints `OK` (non-zero exit + parse error if a comma/brace is wrong).

- [ ] **Step 3: Build settings-service to confirm the schema loads**

Run: `cd /home/teal/src/librescoot/settings-service && GOTOOLCHAIN=go1.25.7 go build ./...`
Expected: builds with no error. (No Go code change; the generic loader reads the new entry. If the repo has a schema test, run `GOTOOLCHAIN=go1.25.7 go test ./...` and expect PASS.)

- [ ] **Step 4: Commit**

```bash
cd /home/teal/src/librescoot/settings-service
git commit settings.schema.json -m "feat: add dashboard.backlight-mode setting"
```

---

## Repo 2: dbc-backlight-service (mode-aware brightness)

### Task 2: Parse the manual level map

**Files:**
- Modify: `/home/teal/src/librescoot/dbc-backlight-service/internal/backlight/backlight.go` (add `ParseLevels` next to `ParseCurve`)
- Test: `/home/teal/src/librescoot/dbc-backlight-service/internal/backlight/backlight_test.go`

**Interfaces:**
- Produces: `func ParseLevels(s string) (map[string]int, error)` — parses `"low:1300 medium:4000 high:10240"` into `{"low":1300,"medium":4000,"high":10240}`. Used by Task 5 (config/service wiring).

- [ ] **Step 1: Write the failing test**

Append to `backlight_test.go`:

```go
func TestParseLevels(t *testing.T) {
	levels, err := ParseLevels("low:1300 medium:4000 high:10240")
	if err != nil {
		t.Fatal(err)
	}
	if len(levels) != 3 {
		t.Fatalf("expected 3 levels, got %d", len(levels))
	}
	if levels["low"] != 1300 || levels["medium"] != 4000 || levels["high"] != 10240 {
		t.Errorf("unexpected levels: %v", levels)
	}
}

func TestParseLevelsErrors(t *testing.T) {
	tests := []string{
		"",
		"low",
		"low:bad",
		"nocolon 2:200",
	}
	for _, s := range tests {
		if _, err := ParseLevels(s); err == nil {
			t.Errorf("expected error for %q", s)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/teal/src/librescoot/dbc-backlight-service && GOTOOLCHAIN=go1.25.7 go test ./internal/backlight/ -run TestParseLevels -v`
Expected: FAIL / does not compile (`undefined: ParseLevels`).

- [ ] **Step 3: Implement `ParseLevels`**

Add to `backlight.go` (after `ParseCurve`):

```go
// ParseLevels parses a manual level map of "name:brightness" pairs.
// Example: "low:1300 medium:4000 high:10240"
func ParseLevels(s string) (map[string]int, error) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil, fmt.Errorf("levels must have at least one entry")
	}

	levels := make(map[string]int, len(fields))
	for _, f := range fields {
		parts := strings.SplitN(f, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid level %q (expected name:brightness)", f)
		}
		brightness, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid brightness value %q: %v", parts[1], err)
		}
		levels[parts[0]] = brightness
	}

	return levels, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/teal/src/librescoot/dbc-backlight-service && GOTOOLCHAIN=go1.25.7 go test ./internal/backlight/ -run TestParseLevels -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/teal/src/librescoot/dbc-backlight-service
git commit internal/backlight/backlight.go internal/backlight/backlight_test.go -m "feat: parse manual backlight level map"
```

### Task 3: Add a manual-target ramp to the backlight Manager

**Files:**
- Modify: `/home/teal/src/librescoot/dbc-backlight-service/internal/backlight/backlight.go` (extract `rampToTarget`, add `ApplyManual`)
- Test: `/home/teal/src/librescoot/dbc-backlight-service/internal/backlight/backlight_test.go`

**Interfaces:**
- Consumes: existing `Manager` fields `output`, `target`, `rampRate`, and `writeBrightness`.
- Produces: `func (m *Manager) ApplyManual(target int) error` — pins the target to a fixed brightness and ramps the output one step toward it (smooth, no snap). Used by Task 5.

- [ ] **Step 1: Write the failing test**

Append to `backlight_test.go`:

```go
func TestApplyManualRampsToTarget(t *testing.T) {
	m := newTestManager(t) // hardware brightness seeded at 5000
	m.AdjustBacklight(1.0)  // initialize internal state

	// One manual step toward 10240 should move up but not jump there.
	m.ApplyManual(10240)
	if m.Output() >= 10240 {
		t.Errorf("expected gradual ramp, got instant jump to %d", m.Output())
	}
	if m.Output() <= 5000 {
		t.Errorf("expected upward movement from 5000, got %d", m.Output())
	}

	// Many steps converge exactly on the target.
	for i := 0; i < 200; i++ {
		m.ApplyManual(10240)
	}
	if m.Output() != 10240 {
		t.Errorf("expected convergence to 10240, got %d", m.Output())
	}
}

func TestApplyManualRampsDown(t *testing.T) {
	m := newTestManager(t)
	m.AdjustBacklight(1.0)
	for i := 0; i < 200; i++ {
		m.ApplyManual(1300)
	}
	if m.Output() != 1300 {
		t.Errorf("expected convergence to 1300, got %d", m.Output())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/teal/src/librescoot/dbc-backlight-service && GOTOOLCHAIN=go1.25.7 go test ./internal/backlight/ -run TestApplyManual -v`
Expected: FAIL / does not compile (`m.ApplyManual undefined`).

- [ ] **Step 3: Extract `rampToTarget` and add `ApplyManual`**

In `backlight.go`, replace the ramp tail of `AdjustBacklight` (the block from `if m.target == m.output {` through the final `return m.writeBrightness(m.output)`) with a call to the new helper:

```go
	if delta > m.targetDeadband {
		m.target = newTarget
	}

	return m.rampToTarget()
}

// rampToTarget moves output one ramp-step toward target, snapping when close.
func (m *Manager) rampToTarget() error {
	if m.target == m.output {
		return nil
	}

	diff := float64(m.target - m.output)
	step := int(math.Round(diff * m.rampRate))

	if step == 0 {
		m.output = m.target
	} else {
		m.output += step
	}

	return m.writeBrightness(m.output)
}

// ApplyManual pins the target to a fixed brightness (manual mode) and ramps
// output toward it, reusing the same smoothing as auto mode.
func (m *Manager) ApplyManual(target int) error {
	if m.output < 0 {
		m.output = target
		m.target = target
		return m.writeBrightness(m.output)
	}
	m.target = target
	return m.rampToTarget()
}
```

(The lines being removed from `AdjustBacklight` are the original:
`if m.target == m.output { return nil }`, the `diff`/`step` computation, the snap `if step == 0 { ... } else { ... }`, and `return m.writeBrightness(m.output)`. They now live verbatim in `rampToTarget`.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/teal/src/librescoot/dbc-backlight-service && GOTOOLCHAIN=go1.25.7 go test ./internal/backlight/ -v`
Expected: PASS (the new `TestApplyManual*` tests and all existing ramp tests, since `AdjustBacklight` behavior is unchanged).

- [ ] **Step 5: Commit**

```bash
cd /home/teal/src/librescoot/dbc-backlight-service
git commit internal/backlight/backlight.go internal/backlight/backlight_test.go -m "feat: add manual-target ramp to backlight manager"
```

### Task 4: Read the backlight mode from Redis and subscribe to settings

**Files:**
- Modify: `/home/teal/src/librescoot/dbc-backlight-service/internal/redis/redis.go` (add `GetBacklightMode`, make `Subscribe` variadic)

**Interfaces:**
- Consumes: existing `Client.client`.
- Produces:
  - `func (c *Client) GetBacklightMode(ctx context.Context) (string, error)` — `HGET settings dashboard.backlight-mode`, returns `"auto"` when missing.
  - `func (c *Client) Subscribe(ctx context.Context, channels ...string) *redis.PubSub` — now variadic so the service can listen on both `dashboard` and `settings`.

- [ ] **Step 1: Add the Redis methods**

In `redis.go`, add after `GetBacklightEnabled`:

```go
func (c *Client) GetBacklightMode(ctx context.Context) (string, error) {
	result, err := c.client.HGet(ctx, "settings", "dashboard.backlight-mode").Result()
	if err != nil {
		if err == redis.Nil {
			return "auto", nil // default to auto when key doesn't exist
		}
		return "auto", err
	}
	return result, nil
}
```

Change `Subscribe` to be variadic:

```go
func (c *Client) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	return c.client.Subscribe(ctx, channels...)
}
```

- [ ] **Step 2: Build to verify it compiles**

Run: `cd /home/teal/src/librescoot/dbc-backlight-service && GOTOOLCHAIN=go1.25.7 go build ./...`
Expected: builds with no error (the single existing `Subscribe(ctx, "dashboard")` call in service.go still compiles, since one arg satisfies the variadic).

- [ ] **Step 3: Commit**

```bash
cd /home/teal/src/librescoot/dbc-backlight-service
git commit internal/redis/redis.go -m "feat: read backlight mode and allow multi-channel subscribe"
```

### Task 5: Wire mode handling into the service

**Files:**
- Modify: `/home/teal/src/librescoot/dbc-backlight-service/internal/config/config.go` (add `ManualLevels` flag)
- Modify: `/home/teal/src/librescoot/dbc-backlight-service/internal/service/service.go` (parse levels, track mode, subscribe to `settings`, branch in `adjustBacklight`)

**Interfaces:**
- Consumes: `backlight.ParseLevels` (Task 2), `Manager.ApplyManual` (Task 3), `Client.GetBacklightMode` + variadic `Subscribe` (Task 4).
- Produces: runtime behavior — when `dashboard.backlight-mode` is `low/medium/high`, output ramps to the mapped level; when `auto` (or unknown), the lux curve drives output. Lux is always read and published.

- [ ] **Step 1: Add the config flag**

In `config.go`, add the field to `Config`:

```go
	ManualLevels     string
```

and the flag in `New()` (after the `Curve` flag):

```go
	flag.StringVar(&cfg.ManualLevels, "manual-levels", "low:1300 medium:4000 high:10240", "Manual backlight levels as name:brightness pairs")
```

- [ ] **Step 2: Add mode state to the Service struct and constructor**

In `service.go`, add fields to `Service`:

```go
	manualLevels      map[string]int
	backlightMode     string
	modeCh            chan struct{}
```

In `New()`, after the curve is parsed, parse the levels and initialize the fields:

```go
	levels, err := backlight.ParseLevels(cfg.ManualLevels)
	if err != nil {
		return nil, fmt.Errorf("invalid manual-levels: %v", err)
	}
```

and in the `service := &Service{...}` literal add:

```go
		manualLevels:  levels,
		backlightMode: "auto",
		modeCh:        make(chan struct{}, 1),
```

- [ ] **Step 3: Subscribe to the settings channel and refresh mode**

In `service.go`, change `subscribeOverride` to listen on both channels and route messages:

```go
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
```

Add the mode refresh method:

```go
func (s *Service) refreshMode(ctx context.Context) {
	mode, err := s.Redis.GetBacklightMode(ctx)
	if err != nil {
		s.Logger.Printf("Failed to read backlight mode: %v", err)
		return
	}
	if mode != s.backlightMode {
		s.backlightMode = mode
		s.Logger.Printf("Backlight mode: %s", mode)
	}
}
```

- [ ] **Step 4: Handle the mode channel and initial read in the loop**

In `monitorIlluminance`, add the initial mode read and the new select case:

```go
	s.checkOverride(ctx)
	s.refreshMode(ctx)
	s.adjustBacklight(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.overrideCh:
			s.checkOverride(ctx)
		case <-s.modeCh:
			s.refreshMode(ctx)
		case <-ticker.C:
			s.adjustBacklight(ctx)
		}
	}
```

- [ ] **Step 5: Branch the output decision in `adjustBacklight`**

In `adjustBacklight`, after the lux read + lux publish block but before the "Publish backlight to Redis" block, replace the single `s.Backlight.AdjustBacklight(lux)` call. The lux read stays first (so the publish below still happens). Concretely, the body becomes:

```go
func (s *Service) adjustBacklight(ctx context.Context) {
	if s.backlightDisabled {
		return
	}

	lux, err := s.readLux(ctx)
	if err != nil {
		s.Logger.Printf("Failed to read illuminance: %v", err)
		return
	}

	if level, manual := s.manualLevels[s.backlightMode]; manual {
		if err := s.Backlight.ApplyManual(level); err != nil {
			s.Logger.Printf("Failed to set manual backlight: %v", err)
			return
		}
	} else {
		if err := s.Backlight.AdjustBacklight(lux); err != nil {
			s.Logger.Printf("Failed to adjust backlight: %v", err)
			return
		}
	}

	if s.Config.Debug {
		target := s.Backlight.Target()
		delta := target - s.lastLoggedTarget
		if delta < 0 {
			delta = -delta
		}
		if delta >= 100 || s.lastLoggedTarget < 0 {
			s.Logger.Printf("lux=%.1f mode=%s → target %d (output %d)", lux, s.backlightMode, target, s.Backlight.Output())
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
```

- [ ] **Step 6: Build and run the full test suite**

Run: `cd /home/teal/src/librescoot/dbc-backlight-service && GOTOOLCHAIN=go1.25.7 go build ./... && GOTOOLCHAIN=go1.25.7 go test ./...`
Expected: build OK, all tests PASS.

- [ ] **Step 7: Cross-build the ARM binary**

Run: `cd /home/teal/src/librescoot/dbc-backlight-service && make build`
Expected: produces `bin/dbc-backlight` with no error.

- [ ] **Step 8: Commit**

```bash
cd /home/teal/src/librescoot/dbc-backlight-service
git commit internal/config/config.go internal/service/service.go -m "feat: apply manual backlight level from dashboard.backlight-mode"
```

### Task 6: On-target integration test (deep-blue DBC)

**Files:** none (manual verification).

**Interfaces:** Consumes the built ARM binary from Task 5.

- [ ] **Step 1: Deploy the test binary to the DBC**

Power the DBC on for testing (never unlock): `ssh deep-blue "lsc dbc on-wait"`. Then copy the binary via tar pipe (scp through the jump host corrupts binaries):

```bash
cd /home/teal/src/librescoot/dbc-backlight-service
tar -C bin -cf - dbc-backlight | ssh deep-blue "ssh root@192.168.7.2 'cat > /data/dbc-backlight-test'"
ssh -J deep-blue root@192.168.7.2 "chmod +x /data/dbc-backlight-test && cp /usr/bin/dbc-backlight /data/dbc-backlight.bak && systemctl stop dbc-backlight && cp /data/dbc-backlight-test /usr/bin/dbc-backlight && systemctl start dbc-backlight"
```

- [ ] **Step 2: Exercise each mode and confirm sysfs brightness**

```bash
# Set high, confirm it ramps to 10240
ssh deep-blue "redis-cli -h 192.168.7.1 HSET settings dashboard.backlight-mode high && redis-cli -h 192.168.7.1 PUBLISH settings dashboard.backlight-mode"
sleep 3; ssh -J deep-blue root@192.168.7.2 "cat /sys/class/backlight/backlight/brightness"   # expect ~10240
# Low -> ~1300, Medium -> ~4000, Auto -> tracks ambient again
ssh deep-blue "redis-cli -h 192.168.7.1 HSET settings dashboard.backlight-mode low && redis-cli -h 192.168.7.1 PUBLISH settings dashboard.backlight-mode"
sleep 3; ssh -J deep-blue root@192.168.7.2 "cat /sys/class/backlight/backlight/brightness"   # expect ~1300
ssh deep-blue "redis-cli -h 192.168.7.1 HSET settings dashboard.backlight-mode auto && redis-cli -h 192.168.7.1 PUBLISH settings dashboard.backlight-mode"
```

Expected: brightness file ramps to the mapped value within a few seconds for each level; `auto` resumes ambient tracking. Confirm `journalctl -u dbc-backlight` logs `Backlight mode: <value>`.

- [ ] **Step 3: Confirm the theme auto-switch still works in manual mode**

With `dashboard.backlight-mode=high` and `dashboard.theme=auto`, cover/uncover the OPT3001 and confirm `dashboard.brightness` (lux) still updates and the theme still flips (the lux loop is independent). Restore: `redis-cli -h 192.168.7.1 HSET settings dashboard.backlight-mode auto`.

- [ ] **Step 4: Restore the original binary and power down**

```bash
ssh -J deep-blue root@192.168.7.2 "systemctl stop dbc-backlight && cp /data/dbc-backlight.bak /usr/bin/dbc-backlight && systemctl start dbc-backlight && sync"
ssh deep-blue "lsc dbc off"
```

(The production binary is restored here; real deployment ships via the normal RPM/OTA build, not this hand-copied test binary.)

---

## Repo 3: scootui-qt (settings UI)

### Task 7: Add `backlightMode` to SettingsStore

**Files:**
- Modify: `/home/teal/src/librescoot/scootui-qt/src/stores/SettingsStore.h`
- Modify: `/home/teal/src/librescoot/scootui-qt/src/stores/SettingsStore.cpp`

**Interfaces:**
- Produces: `QString SettingsStore::backlightMode() const` (default `"auto"`) + `backlightModeChanged()` signal + sync mapping `backlightMode <-> dashboard.backlight-mode`. Consumed by Task 9 (menu).

- [ ] **Step 1: Add the property, getter, signal, and member (SettingsStore.h)**

Add the `Q_PROPERTY` near the other theme/mode properties (after line 10, the `mode` property):

```cpp
    Q_PROPERTY(QString backlightMode READ backlightMode NOTIFY backlightModeChanged)
```

Add the getter near `mode()` (after line 41):

```cpp
    QString backlightMode() const { return m_backlightMode; }
```

Add the signal near `modeChanged()` (after line 74):

```cpp
    void backlightModeChanged();
```

Add the member near `m_mode` (after line 109):

```cpp
    // @schema dashboard.backlight-mode
    QString m_backlightMode = QStringLiteral("auto");
```

- [ ] **Step 2: Add the sync field and update handler (SettingsStore.cpp)**

Add the sync field mapping after the `mode` line (line 14):

```cpp
            {QStringLiteral("backlightMode"), QStringLiteral("dashboard.backlight-mode")},
```

Add the handler branch after the `dashboard.mode` branch (after line 50, before the `show-raw-speed` branch):

```cpp
    } else if (variable == QLatin1String("dashboard.backlight-mode")) {
        if (value != m_backlightMode) { m_backlightMode = value; emit backlightModeChanged(); }
```

- [ ] **Step 3: Build to verify it compiles**

Run: `cd /home/teal/src/librescoot/scootui-qt && make build`
Expected: builds `build/bin/scootui` with no error.

- [ ] **Step 4: Commit**

```bash
cd /home/teal/src/librescoot/scootui-qt
git commit src/stores/SettingsStore.h src/stores/SettingsStore.cpp -m "feat: sync dashboard.backlight-mode in SettingsStore"
```

### Task 8: Add the writer method and translation strings

**Files:**
- Modify: `/home/teal/src/librescoot/scootui-qt/src/services/SettingsService.h`
- Modify: `/home/teal/src/librescoot/scootui-qt/src/services/SettingsService.cpp`
- Modify: `/home/teal/src/librescoot/scootui-qt/src/l10n/Translations.h`
- Modify: `/home/teal/src/librescoot/scootui-qt/src/l10n/Translations.cpp`

**Interfaces:**
- Produces:
  - `void SettingsService::updateBacklightMode(const QString &mode)` (Q_INVOKABLE) — writes `dashboard.backlight-mode`.
  - Translation getters `menuBacklight()`, `menuBacklightAuto()`, `menuBacklightLow()`, `menuBacklightMedium()`, `menuBacklightHigh()`.
  - Both consumed by Task 9.

- [ ] **Step 1: Add the SettingsService writer (header + impl)**

In `SettingsService.h`, add after `updateTheme`/`updateAutoTheme` (after line 17):

```cpp
    Q_INVOKABLE void updateBacklightMode(const QString &mode);
```

In `SettingsService.cpp`, add after `updateTheme`'s definition (after line ~27):

```cpp
void SettingsService::updateBacklightMode(const QString &mode)
{
    writeSetting(QStringLiteral("dashboard.backlight-mode"), mode);
}
```

- [ ] **Step 2: Add the translation properties and getters (Translations.h)**

After the `menuThemeLight` property (line 16) add:

```cpp
    Q_PROPERTY(QString menuBacklight READ menuBacklight NOTIFY languageChanged)
    Q_PROPERTY(QString menuBacklightAuto READ menuBacklightAuto NOTIFY languageChanged)
    Q_PROPERTY(QString menuBacklightLow READ menuBacklightLow NOTIFY languageChanged)
    Q_PROPERTY(QString menuBacklightMedium READ menuBacklightMedium NOTIFY languageChanged)
    Q_PROPERTY(QString menuBacklightHigh READ menuBacklightHigh NOTIFY languageChanged)
```

After the `menuThemeLight()` getter (line 431) add:

```cpp
    QString menuBacklight() const { return lookup("menuBacklight"); }
    QString menuBacklightAuto() const { return lookup("menuBacklightAuto"); }
    QString menuBacklightLow() const { return lookup("menuBacklightLow"); }
    QString menuBacklightMedium() const { return lookup("menuBacklightMedium"); }
    QString menuBacklightHigh() const { return lookup("menuBacklightHigh"); }
```

- [ ] **Step 3: Add the en/de strings (Translations.cpp)**

After the `menuThemeLight` entries (after line 59) add:

```cpp
    en[QStringLiteral("menuBacklight")] = QStringLiteral("Backlight");
    de[QStringLiteral("menuBacklight")] = QStringLiteral("Beleuchtung");

    en[QStringLiteral("menuBacklightAuto")] = QStringLiteral("Automatic");
    de[QStringLiteral("menuBacklightAuto")] = QStringLiteral("Automatisch");

    en[QStringLiteral("menuBacklightLow")] = QStringLiteral("Low");
    de[QStringLiteral("menuBacklightLow")] = QStringLiteral("Niedrig");

    en[QStringLiteral("menuBacklightMedium")] = QStringLiteral("Medium");
    de[QStringLiteral("menuBacklightMedium")] = QStringLiteral("Mittel");

    en[QStringLiteral("menuBacklightHigh")] = QStringLiteral("High");
    de[QStringLiteral("menuBacklightHigh")] = QStringLiteral("Hoch");
```

- [ ] **Step 4: Build to verify it compiles**

Run: `cd /home/teal/src/librescoot/scootui-qt && make build`
Expected: builds with no error.

- [ ] **Step 5: Commit**

```bash
cd /home/teal/src/librescoot/scootui-qt
git commit src/services/SettingsService.h src/services/SettingsService.cpp src/l10n/Translations.h src/l10n/Translations.cpp -m "feat: add backlight mode writer and translations"
```

### Task 9: Add the Backlight cycle entry to the settings menu

**Files:**
- Modify: `/home/teal/src/librescoot/scootui-qt/src/stores/MenuStore.cpp` (insert after the Theme cycle, around line 385)

**Interfaces:**
- Consumes: `SettingsStore::backlightMode()` (Task 7), `SettingsService::updateBacklightMode` (Task 8), translation getters (Task 8), existing `svc`, `settings`, `tr`, and `MenuNode::cycleSetting`.

- [ ] **Step 1: Insert the Backlight cycle entry**

In `MenuStore.cpp`, immediately after the Theme block's closing `}` (line 385), add:

```cpp
    // Backlight (inline cycle: Auto -> Low -> Medium -> High). Auto = ambient
    // light sensor; the fixed levels override brightness only, not the theme.
    {
        const QString blMode = settings->backlightMode();
        int blIdx = 0; // auto
        if (blMode == QLatin1String("low")) blIdx = 1;
        else if (blMode == QLatin1String("medium")) blIdx = 2;
        else if (blMode == QLatin1String("high")) blIdx = 3;
        settingsNode->addChild(MenuNode::cycleSetting(QStringLiteral("settings_backlight"),
            tr->menuBacklight(), {
                {tr->menuBacklightAuto(),   [svc]() { svc->updateBacklightMode(QStringLiteral("auto")); }},
                {tr->menuBacklightLow(),    [svc]() { svc->updateBacklightMode(QStringLiteral("low")); }},
                {tr->menuBacklightMedium(), [svc]() { svc->updateBacklightMode(QStringLiteral("medium")); }},
                {tr->menuBacklightHigh(),   [svc]() { svc->updateBacklightMode(QStringLiteral("high")); }},
            }, blIdx));
    }
```

- [ ] **Step 2: Build to verify it compiles**

Run: `cd /home/teal/src/librescoot/scootui-qt && make build`
Expected: builds with no error.

- [ ] **Step 3: Commit**

```bash
cd /home/teal/src/librescoot/scootui-qt
git commit src/stores/MenuStore.cpp -m "feat: add backlight level entry to settings menu"
```

### Task 10: Add simulator buttons and verify end to end on desktop

**Files:**
- Modify: `/home/teal/src/librescoot/scootui-qt/qml/simulator/SimulatorWindow.qml` (add buttons next to the theme buttons, around lines 58-74)

**Interfaces:**
- Consumes: existing `simulator.setSetting(key, value)` (no C++ change needed).

- [ ] **Step 1: Add the simulator buttons**

In `SimulatorWindow.qml`, after the theme button group (the `SimButton` with `onClicked: simulator.setTheme("auto")` near line 74), add a backlight row:

```qml
            SimButton {
                text: "BL Auto"
                onClicked: simulator.setSetting("dashboard.backlight-mode", "auto")
            }
            SimButton {
                text: "BL Low"
                onClicked: simulator.setSetting("dashboard.backlight-mode", "low")
            }
            SimButton {
                text: "BL Med"
                onClicked: simulator.setSetting("dashboard.backlight-mode", "medium")
            }
            SimButton {
                text: "BL High"
                onClicked: simulator.setSetting("dashboard.backlight-mode", "high")
            }
```

- [ ] **Step 2: Build and run the simulator**

Run: `cd /home/teal/src/librescoot/scootui-qt && make build && ./build/bin/scootui` (or `./run-desktop.sh`)
Expected: app launches. Open the menu (Settings) and confirm a "Backlight" entry appears under Theme, cycling Automatic/Low/Medium/High and showing the current value. Clicking the simulator "BL *" buttons updates the entry's shown value. The Theme entry still works independently.

- [ ] **Step 3: Commit**

```bash
cd /home/teal/src/librescoot/scootui-qt
git commit qml/simulator/SimulatorWindow.qml -m "feat: add backlight mode buttons to simulator"
```

---

## Self-Review

**Spec coverage:**
- Setting `dashboard.backlight-mode` {auto,low,medium,high}, default auto, no off -> Task 1 (schema), Task 7 (store), Task 9 (menu).
- Levels 1300/4000/10240, flag-tunable -> Task 2 (parse) + Task 5 (flag default).
- Mode-aware target, lux loop untouched, theme independent -> Task 3 (manual ramp) + Task 5 (branch, lux always read/published) + Task 6 Step 3 (verify theme).
- `backlight-enabled` system blank still wins -> preserved by the unchanged `if s.backlightDisabled { return }` guard at the top of `adjustBacklight` (Task 5 Step 5).
- settings-service schema-only -> Task 1 (no Go change).
- scootui-qt store + menu + simulator -> Tasks 7-10.
- Testing on deep-blue -> Task 6.

**Placeholder scan:** No TBD/TODO; every code step shows full code. The only "approximate" values are the `~10240`/`~1300` brightness readings in Task 6, which are runtime ramp targets, not code.

**Type consistency:** `ParseLevels` returns `map[string]int` (Task 2) consumed as `s.manualLevels map[string]int` (Task 5). `ApplyManual(int)` (Task 3) called with `level int` from the map lookup (Task 5). `GetBacklightMode` returns `(string, error)` -> `s.backlightMode string` (Tasks 4-5). `backlightMode()` getter (Task 7) used by `settings->backlightMode()` (Task 9). `updateBacklightMode(const QString&)` (Task 8) called in the menu lambdas (Task 9). Translation getter names match between Tasks 8 and 9. Consistent.
