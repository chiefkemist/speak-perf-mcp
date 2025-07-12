import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  stages: [
    { duration: '2m', target: 100 }, // Normal load
    { duration: '30s', target: 500 }, // Spike to 500 users
    { duration: '30s', target: 500 }, // Stay at spike
    { duration: '2m', target: 100 }, // Scale down
    { duration: '2m', target: 0 },   // Ramp down to 0
  ],
  thresholds: {
    http_req_duration: ['p(95)<2000'], // 95% of requests under 2s even during spike
    http_req_failed: ['rate<0.2'], // Error rate under 20% even during spike
  },
};

export default function () {
  const res = http.get('http://localhost:8080/api/data');
  
  check(res, {
    'status is 200': (r) => r.status === 200,
  });
  
  sleep(Math.random() * 2 + 1); // Random sleep between 1-3 seconds
}