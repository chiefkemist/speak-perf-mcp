# K6 Test Scripts

This directory contains k6 performance test scripts for load testing web applications.

## Scripts

### 1. basic-test.js
**Purpose**: Simple health check and baseline performance test

**What it does**:
- Sends GET requests to `/api/health` endpoint
- Validates 200 status responses
- Checks response times under 200ms
- Runs with 10 VUs for 30 seconds

**Usage**:
```bash
k6 run basic-test.js
```

**Thresholds**:
- 95% of requests must complete below 500ms
- HTTP error rate must be less than 10%

### 2. api-test.js
**Purpose**: Comprehensive API endpoint testing with CRUD operations

**What it does**:
- Tests multiple endpoints (`/api/users`)
- Performs GET requests to list users
- Creates new users with POST requests
- Tracks custom error metrics
- Uses staged load (ramp up, sustain, ramp down)

**Usage**:
```bash
# With default localhost:8080
k6 run api-test.js

# With custom base URL
k6 run -e BASE_URL=https://api.example.com api-test.js
```

**Stages**:
1. 2 minutes: Ramp up to 50 users
2. 5 minutes: Maintain 50 users
3. 2 minutes: Ramp down to 0 users

**Thresholds**:
- 95% of requests under 1 second
- Error rate under 5%

### 3. spike-test.js
**Purpose**: Test system behavior under sudden load spikes

**What it does**:
- Simulates traffic spikes (5x normal load)
- Tests system recovery after spike
- Monitors performance degradation
- Uses random sleep intervals (1-3 seconds)

**Usage**:
```bash
k6 run spike-test.js
```

**Stages**:
1. 2 minutes: Normal load (100 VUs)
2. 30 seconds: Spike to 500 VUs
3. 30 seconds: Maintain spike
4. 2 minutes: Return to normal (100 VUs)
5. 2 minutes: Ramp down to 0

**Thresholds**:
- 95% of requests under 2 seconds (even during spike)
- Error rate under 20% (even during spike)

### 4. browser-test.js
**Purpose**: End-to-end browser testing with Web Vitals

**What it does**:
- Launches headless Chrome browser
- Navigates to homepage
- Fills and submits login form
- Takes screenshots
- Measures Core Web Vitals

**Usage**:
```bash
k6 run browser-test.js
```

**Requirements**:
- k6 with browser support
- Chrome/Chromium installed

**Thresholds**:
- LCP (Largest Contentful Paint) < 2.5s
- FID (First Input Delay) < 100ms
- CLS (Cumulative Layout Shift) < 0.1

## Customizing Scripts

### Environment Variables
- `BASE_URL`: Override the default target URL
- `__VU`: Built-in k6 variable for virtual user ID
- `__ITER`: Built-in k6 variable for iteration number

### Common Patterns

**Adding authentication**:
```javascript
const params = {
  headers: {
    'Authorization': `Bearer ${__ENV.API_TOKEN}`,
    'Content-Type': 'application/json',
  },
};
```

**Custom metrics**:
```javascript
import { Rate, Trend } from 'k6/metrics';

const myErrorRate = new Rate('custom_errors');
const myLatency = new Trend('custom_latency');
```

**Grouping requests**:
```javascript
import { group } from 'k6';

group('User API', () => {
  // Related requests here
});
```

## Output Formats

Scripts can output results in various formats:

```bash
# Console output (default)
k6 run script.js

# JSON output for parsing
k6 run --out json=results.json script.js

# Multiple outputs
k6 run --out json=results.json --out csv=results.csv script.js
```

## Best Practices

1. **Use checks liberally** - Validate response content, not just status codes
2. **Set realistic thresholds** - Based on your SLAs
3. **Use stages for gradual load** - Avoid shocking the system
4. **Add think time** - `sleep()` between requests mimics real users
5. **Tag your requests** - For better result analysis
6. **Handle errors gracefully** - Check responses before parsing

## Troubleshooting

**High error rates**:
- Check if the target server is running (default: localhost:8080)
- Verify endpoints exist and are responding
- Look for rate limiting or connection limits

**Browser test failures**:
- Ensure k6 browser module is installed
- Check Chrome/Chromium is available
- Verify the page structure matches selectors

## Integration with CI/CD

These scripts are designed to be CI/CD friendly:

```yaml
# Example GitHub Actions
- name: Run Performance Tests
  run: |
    k6 run k6-scripts/api-test.js --out json=results.json
    k6 run k6-scripts/spike-test.js --quiet
```

## Contributing New Scripts

When adding new test scripts:
1. Follow the existing naming convention
2. Include clear comments explaining the test scenario
3. Define appropriate thresholds
4. Document any special requirements
5. Update this README with script details