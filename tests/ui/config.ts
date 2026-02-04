export const config = {
  admin_email: 'auto_test@tyk.io',
  password: 'Test#2025',
  admin_name: 'Test Admin',
  dev_user_email: 'dev@tyk.io',
  dev1_user_email: 'dev1@tyk.io',
  dev2_user_email: 'dev2@tyk.io',

  // Use environment variable or default to dev ports
  // Dev: frontend on 3000 (proxies API to 8080), CI sets TEST_BASE_URL to 8081 (Go embedded)
  base_url: process.env.TEST_BASE_URL || 'http://localhost:3000',
  register_url: process.env.TEST_BASE_URL ? `${process.env.TEST_BASE_URL}/register` : 'http://localhost:3000/register',
  api_url: process.env.TEST_API_URL || process.env.TEST_BASE_URL || 'http://localhost:3000',

  // Bootstrap admin for API-based setup (when DB has existing users)
  // Set via environment variables or use defaults
  bootstrap_admin_email: process.env.BOOTSTRAP_ADMIN_EMAIL || 'admin@tyk.io',
  bootstrap_admin_password: process.env.BOOTSTRAP_ADMIN_PASSWORD || 'Admin#2025',
};