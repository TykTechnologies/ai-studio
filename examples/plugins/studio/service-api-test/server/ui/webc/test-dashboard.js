// Service API E2E Test Dashboard Web Component
class ServiceAPITestDashboard extends HTMLElement {
  constructor() {
    super();
    this.attachShadow({ mode: 'open' });
    this.testResults = null;
    this.isRunning = false;
  }

  connectedCallback() {
    console.log('Service API Test Dashboard initialized');
    this.render();
    this.setupEventListeners();
    this.waitForPluginAPI();
  }

  async waitForPluginAPI() {
    for (let i = 0; i < 50; i++) {
      if (this.pluginAPI) {
        console.log('Plugin API ready');
        return;
      }
      await new Promise(resolve => setTimeout(resolve, 100));
    }
    console.error('Plugin API timeout');
  }

  async runTests() {
    if (!this.pluginAPI || this.isRunning) return;

    this.isRunning = true;
    this.updateStatus('Running E2E tests...');
    const btn = this.shadowRoot.querySelector('#run-tests-btn');
    if (btn) {
      btn.disabled = true;
      btn.textContent = 'Running Tests...';
    }

    try {
      const result = await this.pluginAPI.call('run_e2e_tests', {});
      console.log('Test results:', result);
      this.testResults = result;
      this.displayResults();
    } catch (error) {
      console.error('Test execution failed:', error);
      this.updateStatus('Test execution failed: ' + error.message);
    } finally {
      this.isRunning = false;
      if (btn) {
        btn.disabled = false;
        btn.textContent = 'Run E2E Tests';
      }
    }
  }

  displayResults() {
    const container = this.shadowRoot.querySelector('#results-container');
    if (!container || !this.testResults) return;

    const report = this.testResults;

    container.innerHTML = `
      <div class="summary">
        <h3>Test Summary</h3>
        <div class="summary-stats">
          <div class="stat passed">✅ Passed: ${report.passed_tests}</div>
          <div class="stat failed">❌ Failed: ${report.failed_tests}</div>
          <div class="stat total">📊 Total: ${report.total_tests}</div>
          <div class="stat duration">⏱️ Duration: ${(report.total_duration_ms / 1000000).toFixed(2)}s</div>
        </div>
      </div>

      ${this.renderTestSuite('LLM Tests', report.llm_tests)}
      ${this.renderTestSuite('Tag Tests', report.tag_tests)}
      ${this.renderTestSuite('App Tests', report.app_tests)}
      ${this.renderTestSuite('Tool Tests', report.tool_tests)}
      ${this.renderTestSuite('Datasource Tests', report.datasource_tests)}
      ${this.renderTestSuite('Filter Tests', report.filter_tests)}
      ${this.renderTestSuite('Model Price Tests', report.model_price_tests)}
      ${this.renderTestSuite('Data Catalogue Tests', report.data_catalogue_tests)}
      ${this.renderTestSuite('KV Storage Tests', report.kv_tests)}
      ${this.renderTestSuite('Cleanup Operations', report.cleanup_results)}
    `;
  }

  renderTestSuite(title, tests) {
    if (!tests || tests.length === 0) return '';

    const passed = tests.filter(t => t.success).length;
    const failed = tests.filter(t => !t.success).length;
    const statusIcon = failed > 0 ? '⚠️' : '✅';

    return `
      <details class="test-suite" open>
        <summary>
          <strong>${title}</strong>
          <span class="suite-status">${statusIcon} ${passed}/${tests.length} passed</span>
        </summary>
        <div class="test-list">
          ${tests.map(test => this.renderTestResult(test)).join('')}
        </div>
      </details>
    `;
  }

  renderTestResult(test) {
    const icon = test.success ? '✅' : '❌';
    const statusClass = test.success ? 'success' : 'failure';
    const duration = (test.duration_ms / 1000000).toFixed(1);

    return `
      <div class="test-result ${statusClass}">
        <span class="test-icon">${icon}</span>
        <span class="test-operation">${test.operation}</span>
        <span class="test-duration">${duration}ms</span>
        <div class="test-message">${test.message}</div>
      </div>
    `;
  }

  updateStatus(message) {
    const status = this.shadowRoot.querySelector('#status-message');
    if (status) status.textContent = message;
  }

  setupEventListeners() {
    this.shadowRoot.querySelector('#run-tests-btn')?.addEventListener('click', () => {
      this.runTests();
    });
  }

  render() {
    this.shadowRoot.innerHTML = `
      <style>
        :host {
          display: block;
          padding: 24px;
          font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
        }
        .header {
          display: flex;
          justify-content: space-between;
          align-items: center;
          margin-bottom: 24px;
        }
        .btn-primary {
          background: #1976d2;
          color: white;
          border: none;
          padding: 12px 24px;
          border-radius: 4px;
          cursor: pointer;
          font-size: 14px;
        }
        .btn-primary:hover:not(:disabled) {
          background: #1565c0;
        }
        .btn-primary:disabled {
          opacity: 0.6;
          cursor: not-allowed;
        }
        #status-message {
          padding: 12px;
          background: #e3f2fd;
          border-radius: 4px;
          margin-bottom: 16px;
        }
        .summary {
          background: white;
          border: 1px solid #e0e0e0;
          border-radius: 8px;
          padding: 16px;
          margin-bottom: 16px;
        }
        .summary-stats {
          display: grid;
          grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
          gap: 12px;
          margin-top: 12px;
        }
        .stat {
          padding: 8px;
          border-radius: 4px;
          text-align: center;
        }
        .stat.passed {
          background: #e8f5e9;
          color: #2e7d32;
        }
        .stat.failed {
          background: #ffebee;
          color: #c62828;
        }
        .stat.total {
          background: #e3f2fd;
          color: #1976d2;
        }
        .stat.duration {
          background: #f3e5f5;
          color: #7b1fa2;
        }
        .test-suite {
          background: white;
          border: 1px solid #e0e0e0;
          border-radius: 8px;
          padding: 12px;
          margin-bottom: 12px;
        }
        .test-suite summary {
          cursor: pointer;
          display: flex;
          justify-content: space-between;
          padding: 8px;
          font-size: 16px;
        }
        .suite-status {
          color: #666;
          font-size: 14px;
        }
        .test-list {
          margin-top: 12px;
        }
        .test-result {
          display: grid;
          grid-template-columns: 30px 1fr 80px;
          gap: 8px;
          padding: 8px;
          border-bottom: 1px solid #f0f0f0;
        }
        .test-result:last-child {
          border-bottom: none;
        }
        .test-result.success {
          background: #f1f8f4;
        }
        .test-result.failure {
          background: #fef5f5;
        }
        .test-operation {
          font-weight: 500;
        }
        .test-duration {
          text-align: right;
          color: #999;
          font-size: 12px;
        }
        .test-message {
          grid-column: 2 / 4;
          color: #666;
          font-size: 13px;
        }
      </style>

      <div class="header">
        <h2>🧪 Service API E2E Test Runner</h2>
        <button id="run-tests-btn" class="btn-primary">Run E2E Tests</button>
      </div>

      <div id="status-message">
        Ready to run tests. Click "Run E2E Tests" to validate all service API endpoints.
      </div>

      <div id="results-container"></div>
    `;
  }
}

customElements.define('service-api-test-dashboard', ServiceAPITestDashboard);
