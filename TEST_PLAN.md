# Test Plan: dbc-backlight-service

## Service Analysis

The service reads ambient light sensor data from Redis and controls hardware backlight brightness using a discrete 5-state state machine with hysteresis to prevent oscillation.

### Testability Assessment

**Minor testability challenges:**

1. **Direct filesystem I/O** - `backlight.Manager` reads/writes to sysfs without abstraction
2. **No interfaces** - Redis client and other dependencies are concrete types
3. **Time-based logic** - Polling loop makes timing-dependent tests harder
4. **Global state** - Service maintains internal state (lastPublishedBrightness, currentLevel)

**Excellent testability aspects:**

1. **Discrete state machine** - Simple, deterministic state transitions
2. **No complex math** - No floating-point calculations or logarithms
3. **Well-structured** - Clear separation of concerns
4. **Configuration via struct** - Easy to inject test values
5. **Predictable behavior** - Each state transition is explicit and testable

---

## Test Plan Structure

### 1. Unit Tests: Backlight State Machine Logic
**File:** `internal/backlight/backlight_test.go`
**Priority:** HIGH - This is the core business logic

#### Test Suite: `AdjustBacklight()` State Transitions

**Table-driven tests for state transitions:**

| Test Case | Current State | Illuminance | Expected New State | Brightness Set? |
|-----------|---------------|-------------|-------------------|----------------|
| **Upward transitions** |||||
| VERY_LOW stays | VERY_LOW | 7 | VERY_LOW | No |
| VERY_LOW → LOW | VERY_LOW | 9 | LOW | Yes (9500) |
| LOW → MID | LOW | 20 | MID | Yes (9700) |
| MID → HIGH | MID | 45 | HIGH | Yes (9950) |
| HIGH → VERY_HIGH | HIGH | 85 | VERY_HIGH | Yes (10240) |
| **Downward transitions** |||||
| LOW → VERY_LOW | LOW | 4 | VERY_LOW | Yes (9350) |
| MID → LOW | MID | 14 | LOW | Yes (9500) |
| HIGH → MID | HIGH | 30 | MID | Yes (9700) |
| VERY_HIGH → HIGH | VERY_HIGH | 65 | HIGH | Yes (9950) |
| **Hysteresis gaps** |||||
| VERY_LOW stable in gap | VERY_LOW | 6 | VERY_LOW | No |
| LOW stable in gap | LOW | 10 | LOW | No |
| MID stable in gap | MID | 20 | MID | No |
| HIGH stable in gap | HIGH | 50 | HIGH | No |
| VERY_HIGH stable | VERY_HIGH | 75 | VERY_HIGH | No |
| **Edge cases** |||||
| Zero illuminance | MID | 0 | VERY_LOW | Yes (9350) |
| Negative (invalid) | MID | -5 | VERY_LOW | Yes (9350) |
| Very high illuminance | MID | 1000 | VERY_HIGH | Yes (10240) |
| Exact threshold up | LOW | 18 | LOW | No |
| Just above threshold | LOW | 19 | MID | Yes (9700) |
| Exact threshold down | LOW | 5 | LOW | No |
| Just below threshold | LOW | 4 | VERY_LOW | Yes (9350) |

**Multi-step transition tests:**
- VERY_LOW → MID (multiple calls with increasing lux)
- VERY_HIGH → VERY_LOW (multiple calls with decreasing lux)
- Oscillating near threshold (verify hysteresis prevents rapid changes)

**Test approach:**
- Use temp files for brightness I/O
- Test state transitions with controlled illuminance values
- Verify only changed states trigger brightness writes
- Verify correct brightness values are written

#### Test Suite: `SetBrightness()` and `GetCurrentBrightness()`

**Tests:**
- Write and read brightness values
- Parse valid integer strings
- Handle malformed data in brightness file
- Handle missing/non-existent files
- Handle permission errors
- Verify file permissions (0644)
- Whitespace trimming

**Test approach:**
- Use temporary files (`os.CreateTemp`)
- Test error conditions with missing/invalid files

---

### 2. Unit Tests: Redis Client
**File:** `internal/redis/redis_test.go`
**Priority:** HIGH - Critical integration point

#### Test Suite: `GetIlluminanceValue()`

**Tests:**
- Parse valid float values (e.g., "42.5", "100.0")
- Parse integer values stored as strings
- Handle `redis.Nil` (missing key) → return 0
- Handle invalid float strings → error
- Handle empty string → error
- Float to int conversion (truncation behavior)
- Large float values
- Negative values

#### Test Suite: `SetBacklightValue()`

**Tests:**
- Pipeline execution (HSET + PUBLISH)
- Verify correct Redis key ("dashboard")
- Verify correct field name ("backlight")
- Verify publish to "dashboard" channel
- Handle Redis connection errors
- Handle pipeline execution errors
- Verify atomic execution

**Test approach:**
- Use `miniredis` library (in-memory Redis mock)
- Alternative: Mock `redis.Client` using interface extraction
- Verify command sequences

---

### 3. Unit Tests: Service Orchestration
**File:** `internal/service/service_test.go`
**Priority:** HIGH - Hysteresis and orchestration logic

#### Test Suite: `adjustBacklightBasedOnIlluminance()`

**Hysteresis logic tests:**

| Scenario | Last Published | Current | Threshold | Expected Behavior |
|----------|---------------|---------|-----------|-------------------|
| First run | -1 | 9500 | 512 | Update Redis |
| Below threshold | 9500 | 9600 | 512 | Skip update |
| At threshold | 9500 | 10012 | 512 | Update Redis |
| Above threshold | 9500 | 10100 | 512 | Update Redis |
| Decreasing below | 10000 | 9600 | 512 | Skip update |
| Decreasing above | 10000 | 9400 | 512 | Update Redis |

**Error handling:**
- Redis read failure → error returned
- Backlight adjustment failure → error returned
- Brightness read failure → warning logged, no error
- Redis write failure → warning logged, no error

**State management:**
- Verify `lastPublishedBrightness` updated only on Redis write
- Verify `lastPublishedBrightness` unchanged on skipped updates

#### Test Suite: `monitorIlluminance()`

**Tests:**
- Initial reading performed immediately
- Ticker fires at configured interval
- Context cancellation stops loop
- Errors logged but don't stop loop

**Test approach:**
- Mock ticker or use short intervals
- Mock context with timeout
- Inject mocked dependencies
- Verify log output

---

### 4. Integration Tests
**File:** `internal/service/integration_test.go`
**Priority:** MEDIUM - End-to-end validation

#### Test Suite: Full Service Lifecycle

**Tests:**
- Service creation with valid config
- Redis connection check on startup
- Complete adjustment cycle:
  1. Read illuminance from Redis
  2. Calculate brightness
  3. Write to backlight file
  4. Publish to Redis
- Multiple cycles with changing illuminance
- Verify hysteresis prevents excessive updates
- Graceful shutdown on context cancellation

**Test approach:**
- Use `miniredis` for Redis
- Use temp directory for backlight sysfs
- Use short polling intervals (10ms)
- Inject test configuration

---

### 5. Table-Driven Tests: Configuration
**File:** `internal/config/config_test.go`
**Priority:** LOW - Straightforward logic

**Tests:**
- Default values
- Flag parsing
- Override defaults with flags
- Invalid flag types
- Duration parsing
- URL parsing

---

### 6. Benchmark Tests
**File:** `internal/backlight/benchmark_test.go`
**Priority:** LOW - Performance validation

**Benchmarks:**
- `AdjustBacklight()` calculation speed
- `GetIlluminanceValue()` with miniredis
- Full adjustment cycle

---

## Testing Strategy Recommendations

### Refactoring for Testability (Optional)

To improve testability, consider extracting interfaces:

#### 1. Filesystem Abstraction

```go
type BacklightWriter interface {
    Write(path string, value int) error
    Read(path string) (int, error)
}

type FileBacklightWriter struct{}

func (f *FileBacklightWriter) Write(path string, value int) error {
    return os.WriteFile(path, []byte(strconv.Itoa(value)), 0644)
}

func (f *FileBacklightWriter) Read(path string) (int, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return 0, err
    }
    return strconv.Atoi(strings.TrimSpace(string(data)))
}
```

#### 2. Redis Client Interface

```go
type RedisClient interface {
    GetIlluminanceValue(ctx context.Context) (int, error)
    SetBacklightValue(ctx context.Context, value int) error
    Ping(ctx context.Context) error
}
```

#### 3. Time Abstraction

```go
type TickerFactory func(d time.Duration) *time.Ticker
```

**Note:** These refactorings are optional. Tests can be written without them using temporary files and miniredis.

---

## Testing Tools & Dependencies

### Required Dependencies

```go
// go.mod additions for testing
require (
    github.com/stretchr/testify v1.8.4
    github.com/alicebob/miniredis/v2 v2.31.0
)
```

### Test Utilities

- **Assertions:** `github.com/stretchr/testify/assert`, `github.com/stretchr/testify/require`
- **Mocking:** `github.com/stretchr/testify/mock` (if interface-based approach used)
- **Redis Mock:** `github.com/alicebob/miniredis/v2`
- **Filesystem:** `os.CreateTemp`, `t.TempDir()`
- **Standard Library:** `testing`, `testing/iotest`

---

## Test File Organization

```
dbc-backlight-service/
├── internal/
│   ├── backlight/
│   │   ├── backlight.go
│   │   ├── backlight_test.go
│   │   └── benchmark_test.go
│   ├── redis/
│   │   ├── redis.go
│   │   └── redis_test.go
│   ├── service/
│   │   ├── service.go
│   │   ├── service_test.go
│   │   └── integration_test.go
│   └── config/
│       ├── config.go
│       └── config_test.go
└── TEST_PLAN.md (this file)
```

---

## Test Coverage Goals

- **Unit tests:** >80% coverage
- **Core calculation logic:** 100% coverage
- **Error paths:** 100% coverage
- **Integration tests:** Happy path + major error scenarios

### Running Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run specific package
go test ./internal/backlight

# Run with verbose output
go test -v ./...

# Run benchmarks
go test -bench=. ./...
```

---

## Priority Execution Order

### Phase 1: Critical (Core Business Logic)
- `internal/backlight/backlight_test.go`
  - Brightness calculation tests
  - File I/O tests (read/write brightness)
  - Edge cases and error handling

### Phase 2: High Priority (External Dependencies)
- `internal/redis/redis_test.go`
  - Redis operations with miniredis
  - Error handling and edge cases

### Phase 3: High Priority (Orchestration)
- `internal/service/service_test.go`
  - Hysteresis logic
  - State management
  - Error handling

### Phase 4: Medium Priority (End-to-End)
- `internal/service/integration_test.go`
  - Full service lifecycle
  - Multi-component interaction

### Phase 5: Low Priority (Simple Logic)
- `internal/config/config_test.go`
  - Configuration parsing
  - Default values

### Phase 6: Optional (Performance)
- `internal/backlight/benchmark_test.go`
  - Performance benchmarks

---

## Example Test Cases

### State Transition Test

```go
func TestAdjustBacklight_StateTransitions(t *testing.T) {
    tests := []struct {
        name              string
        initialState      backlight.BrightnessLevel
        illuminance       int
        wantState         backlight.BrightnessLevel
        wantBrightness    int
        wantBrightnessSet bool
    }{
        // Upward transitions
        {"VERY_LOW_to_LOW", backlight.LevelVeryLow, 9, backlight.LevelLow, 9500, true},
        {"LOW_to_MID", backlight.LevelLow, 20, backlight.LevelMid, 9700, true},
        {"MID_to_HIGH", backlight.LevelMid, 45, backlight.LevelHigh, 9950, true},
        {"HIGH_to_VERY_HIGH", backlight.LevelHigh, 85, backlight.LevelVeryHigh, 10240, true},

        // Downward transitions
        {"LOW_to_VERY_LOW", backlight.LevelLow, 4, backlight.LevelVeryLow, 9350, true},
        {"MID_to_LOW", backlight.LevelMid, 14, backlight.LevelLow, 9500, true},
        {"HIGH_to_MID", backlight.LevelHigh, 30, backlight.LevelMid, 9700, true},
        {"VERY_HIGH_to_HIGH", backlight.LevelVeryHigh, 65, backlight.LevelHigh, 9950, true},

        // Hysteresis - stay in current state
        {"LOW_stable_in_gap", backlight.LevelLow, 10, backlight.LevelLow, 9500, false},
        {"MID_stable_in_gap", backlight.LevelMid, 20, backlight.LevelMid, 9700, false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            tempFile := createTempBrightnessFile(t)
            defer os.Remove(tempFile)

            // Create manager with default thresholds
            m := backlight.New(
                tempFile,
                log.New(io.Discard, "", 0),
                9350, 9500, 9700, 9950, 10240, // brightness levels
                8, 18, 40, 80,                  // upward thresholds
                5, 15, 35, 70,                  // downward thresholds
            )

            // Set initial state (using reflection or exported setter)
            m.SetCurrentLevel(tt.initialState)

            err := m.AdjustBacklight(tt.illuminance)
            require.NoError(t, err)

            assert.Equal(t, tt.wantState, m.GetCurrentLevel())

            if tt.wantBrightnessSet {
                brightness, err := m.GetCurrentBrightness()
                require.NoError(t, err)
                assert.Equal(t, tt.wantBrightness, brightness)
            }
        })
    }
}
```

### Hysteresis Test

```go
func TestService_Hysteresis(t *testing.T) {
    // Setup: miniredis, temp file, service with threshold=512

    // Scenario 1: First update (lastPublished=-1) should always write
    // Scenario 2: Small change (delta < 512) should skip update
    // Scenario 3: Large change (delta >= 512) should update

    // Verify Redis write count matches expectations
}
```

### Redis Client Test

```go
func TestRedisClient_GetIlluminanceValue(t *testing.T) {
    tests := []struct {
        name      string
        setValue  string
        want      int
        wantError bool
    }{
        {"valid_float", "42.7", 42, false},
        {"valid_int", "100", 100, false},
        {"missing_key", "", 0, false}, // redis.Nil case
        {"invalid_format", "abc", 0, true},
        {"negative", "-10.5", -10, false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            s := miniredis.RunT(t)
            defer s.Close()

            if tt.setValue != "" {
                s.HSet("dashboard", "brightness", tt.setValue)
            }

            client, _ := redis.New("redis://"+s.Addr(), log.New(os.Stdout, "", 0))
            got, err := client.GetIlluminanceValue(context.Background())

            if tt.wantError {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.Equal(t, tt.want, got)
            }
        })
    }
}
```

---

## Success Criteria

Tests are considered complete when:

1. All priority phases 1-3 tests are implemented
2. Coverage exceeds 80% overall
3. Core calculation logic has 100% coverage
4. All tests pass consistently
5. Tests run in <5 seconds (excluding benchmarks)
6. CI/CD integration possible (no external dependencies required)

---

## Future Enhancements

- Property-based testing for mathematical formulas (using `gopter`)
- Fuzz testing for input validation
- Load/stress testing for production scenarios
- Mock generation automation
- Test data fixtures for complex scenarios
