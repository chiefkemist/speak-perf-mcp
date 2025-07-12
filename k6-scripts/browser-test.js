import { browser } from 'k6/experimental/browser';
import { check } from 'k6';

export const options = {
  scenarios: {
    browser: {
      executor: 'shared-iterations',
      vus: 1,
      iterations: 5,
      options: {
        browser: {
          type: 'chromium',
        },
      },
    },
  },
  thresholds: {
    browser_web_vital_lcp: ['p(90) < 2500'], // Largest Contentful Paint
    browser_web_vital_fid: ['p(90) < 100'],  // First Input Delay
    browser_web_vital_cls: ['p(90) < 0.1'],  // Cumulative Layout Shift
  },
};

export default async function () {
  const page = browser.newPage();

  try {
    // Navigate to the page
    await page.goto('http://localhost:8080', { waitUntil: 'networkidle' });
    
    // Take a screenshot
    page.screenshot({ path: 'screenshots/homepage.png' });
    
    // Test form interaction
    await page.locator('input[name="email"]').type('test@example.com');
    await page.locator('input[name="password"]').type('password123');
    await page.locator('button[type="submit"]').click();
    
    // Wait for navigation
    await page.waitForNavigation();
    
    // Check if we're logged in
    const welcomeText = await page.locator('h1').textContent();
    check(welcomeText, {
      'logged in successfully': (text) => text.includes('Welcome'),
    });
    
  } finally {
    page.close();
  }
}