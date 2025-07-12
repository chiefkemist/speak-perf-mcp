import http from 'k6/http';
import { check, group, sleep } from 'k6';
import { Rate } from 'k6/metrics';

// Custom metrics
const errorRate = new Rate('errors');

export const options = {
  stages: [
    { duration: '2m', target: 50 }, // Ramp up to 50 users
    { duration: '5m', target: 50 }, // Stay at 50 users
    { duration: '2m', target: 0 },  // Ramp down to 0 users
  ],
  thresholds: {
    http_req_duration: ['p(95)<1000'], // 95% of requests under 1s
    errors: ['rate<0.05'], // Error rate under 5%
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

export default function () {
  group('API endpoints', () => {
    // Test GET request
    group('GET /api/users', () => {
      const res = http.get(`${BASE_URL}/api/users`);
      const success = check(res, {
        'GET users status 200': (r) => r.status === 200,
        'GET users has results': (r) => JSON.parse(r.body).length > 0,
      });
      errorRate.add(!success);
    });

    sleep(1);

    // Test POST request
    group('POST /api/users', () => {
      const payload = JSON.stringify({
        name: `User ${__VU}`,
        email: `user${__VU}@example.com`,
      });
      
      const params = {
        headers: { 'Content-Type': 'application/json' },
      };
      
      const res = http.post(`${BASE_URL}/api/users`, payload, params);
      const success = check(res, {
        'POST user status 201': (r) => r.status === 201,
        'POST user has id': (r) => JSON.parse(r.body).id !== undefined,
      });
      errorRate.add(!success);
    });

    sleep(1);
  });
}