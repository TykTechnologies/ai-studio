# AI Studio UI Testing Framework

This folder contains automated UI tests for the Tyk AI Studio using Playwright. The tests are designed to validate the functionality of the studio's user interface.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Starting the Test Environment](#starting-the-test-environment)
- [Running Tests](#running-tests)
- [Framework Structure](#framework-structure)
- [Writing New Tests](#writing-new-tests)

## Prerequisites

- Node.js (v18.16 or higher)
- npm (v9 or higher)
- Docker and Docker Compose (for running the test environment)

## Installation

1. Install the UI test dependencies:
   ```bash
   cd tests/ui
   npm ci
   ```

2. Install Playwright browsers:
   ```bash
   npx playwright install --with-deps chromium
   ```

## Starting the Test Environment

The test environment can be started using Docker Compose, which will set up all the necessary services including the Tyk AI Studio and postgres DB.

1. Make sure you have the required environment variables set in your `.env` file. At minimum, you need:
   ```
   TYK_AI_LICENSE=your_license_key
   ```

2. Start the environment using Docker Compose (execute from repo top level):
   ```bash
   docker compose --env-file .env -f tests/compose.yml up
   ```

3. Wait for the services to start. The UI will be available at http://localhost:3000.

## Running Tests

The framework includes several npm scripts to run different types of tests:

1. Run prerequisite tests (like registering an admin user):
   ```bash
   npm run prerequisite
   ```

2. Run all tests:
   ```bash
   npm run test
   ```

3. Run tests with the Playwright UI:
   ```bash
   npm run gui
   ```

## Framework Structure

The UI testing framework follows the Page Object Model (POM) pattern and is organized as follows:

### Directory Structure

```
tests/ui/
├── config.ts                # Configuration settings
├── package.json             # Dependencies and scripts
├── playwright.config.ts     # Playwright configuration
├── pom/                     # Page Object Models
│   ├── Login_page.ts        # Login page object
│   └── Register_page.ts     # Registration page object
├── prerequisite/            # Prerequisite tests
│   └── register-admin.spec.ts  # Admin registration test
├── tests/                   # Test files
│   └── register-user.spec.ts   # User registration test
├── tsconfig.json            # TypeScript configuration
└── utils/                   # Utility functions
    ├── fixtures.ts          # Test fixtures
    └── utils.ts             # Helper functions
```

### Key Components

1. **Page Object Models (POM)**
   - Located in the `pom/` directory
   - Each page of the application has its own class
   - Encapsulates page elements and actions

2. **Test Fixtures**
   - Located in `utils/fixtures.ts`
   - Extends Playwright's test fixtures with custom page objects
   - Makes page objects available in test functions

3. **Configuration**
   - Located in `config.ts`
   - Contains test data and environment settings

4. **Prerequisite Tests**
   - Located in the `prerequisite/` directory
   - Sets up the environment for other tests (e.g., registering an admin user)

5. **Tests**
   - Located in the `tests/` directory
   - Contains the actual test scenarios

## Writing New Tests

To write a new test, follow these steps:

1. **Identify the page(s) you need to interact with**
   - Use existing page objects or create new ones if needed

2. **Create a new test file**
   - Place it in the `tests/` directory
   - Name it descriptively, e.g., `login-validation.spec.ts`

3. **Import the necessary components**
   ```typescript
   import { test } from '@fixtures';
   import { expect } from '@playwright/test';
   import { config } from '@config';
   // Import any utility functions if needed
   import { generateRandomEmail } from '@utils';
   ```

4. **Write your test**
   ```typescript
   test('Test description', async ({ page, loginPage, registerPage }) => {
     // Test steps using page objects
     await loginPage.goto();
     await loginPage.login(config.admin_email, config.admin_password);
     
     // Assertions
     await expect(page).toHaveTitle(/Tyk AI Portal/);
   });
   ```

5. **Add new page objects if needed**
   - Create a new file in the `pom/` directory
   - Follow the existing pattern:

   ```typescript
   import { Locator, Page } from '@playwright/test';
   import { config } from '../config';

   export class NewPage {
     readonly page: Page;
     readonly someElement: Locator;

     constructor(page: Page) {
       this.page = page;
       this.someElement = this.page.getByRole('button', { name: 'Some Button' });
     }

     async someAction() {
       await this.someElement.click();
     }
   }
   ```

6. **Update fixtures if you added a new page object**
   - Add your new page object to `utils/fixtures.ts`:

   ```typescript
   import { NewPage } from '../pom/New_page';

   export const test = base.extend<{
     // Existing fixtures
     loginPage: LoginPage;
     registerPage: RegisterPage;
     // New fixture
     newPage: NewPage;
   }>({
     // Existing fixture implementations
     // ...
     // New fixture implementation
     newPage: async ({ page }, use) => {
       const newPage = new NewPage(page);
       await use(newPage);
     },
   });
   ```

7. **Run your test**
   ```bash
   npx playwright test tests/your-new-test.spec.ts
   ```

## Best Practices

1. **Keep page objects focused on a single page**
   - Each page object should represent a single page or component

2. **Use descriptive names for test files and functions**
   - Names should clearly indicate what is being tested

3. **Use the config file for test data**
   - Avoid hardcoding test data in test files

4. **Run prerequisite tests before running other tests**
   - This ensures the environment is properly set up

5. **Use utility functions for common operations**
   - For example, generating random emails or other test data