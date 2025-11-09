class HookTestRunner extends HTMLElement {
  constructor() {
    super();
    this.attachShadow({ mode: 'open' });

    // State
    this.data = {
      testResults: [],
      isRunning: false,
      currentFilter: 'all',
      startTime: null
    };

    // Test configuration
    this.objectTypes = ['llm', 'datasource', 'tool', 'user'];
    this.hookTypes = ['before_create', 'after_create', 'before_update', 'after_update', 'before_delete', 'after_delete'];
  }

  connectedCallback() {
    console.log('HookTestRunner component initialized:', {
      hasPluginAPI: !!this.pluginAPI
    });

    this.render();
    this.setupEventListeners();

    // Wait for plugin API to be injected before enabling UI
    this.waitForPluginAPI();
  }

  async waitForPluginAPI() {
    // Poll for plugin API availability (max 5 seconds)
    for (let i = 0; i < 50; i++) {
      if (this.pluginAPI) {
        console.log('Plugin API found, ready to run tests');
        this.updateStatus('Ready');
        return;
      }
      await new Promise(resolve => setTimeout(resolve, 100));
    }

    // Timeout - show error
    console.error('Plugin API injection timeout');
    this.showError('Plugin API initialization timeout - please refresh the page');
  }

  render() {
    this.shadowRoot.innerHTML = `
      <style>
        * {
          box-sizing: border-box;
        }

        .container {
          padding: 20px;
          font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
          max-width: 1400px;
          margin: 0 auto;
        }

        h1 {
          font-size: 24px;
          font-weight: 600;
          margin-bottom: 8px;
          color: #1a1a1a;
        }

        .subtitle {
          color: #666;
          margin-bottom: 24px;
        }

        .controls {
          display: flex;
          gap: 12px;
          margin-bottom: 24px;
        }

        button {
          padding: 10px 20px;
          border: none;
          border-radius: 6px;
          font-size: 14px;
          font-weight: 500;
          cursor: pointer;
          transition: all 0.2s;
        }

        button:disabled {
          opacity: 0.5;
          cursor: not-allowed;
        }

        .primary-btn {
          background: #2563eb;
          color: white;
        }

        .primary-btn:hover:not(:disabled) {
          background: #1d4ed8;
        }

        .secondary-btn {
          background: #f3f4f6;
          color: #374151;
        }

        .secondary-btn:hover:not(:disabled) {
          background: #e5e7eb;
        }

        .status-bar {
          background: #f8fafc;
          border: 1px solid #e2e8f0;
          border-radius: 8px;
          padding: 16px;
          margin-bottom: 24px;
        }

        .status-text {
          font-weight: 500;
          color: #334155;
          margin-bottom: 12px;
        }

        .progress-bar {
          height: 8px;
          background: #e2e8f0;
          border-radius: 4px;
          overflow: hidden;
        }

        .progress-fill {
          height: 100%;
          background: #2563eb;
          transition: width 0.3s ease;
          width: 0%;
        }

        .summary-cards {
          display: grid;
          grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
          gap: 16px;
          margin-bottom: 24px;
        }

        .summary-card {
          background: white;
          border: 1px solid #e2e8f0;
          border-radius: 8px;
          padding: 20px;
        }

        .summary-label {
          color: #64748b;
          font-size: 14px;
          margin-bottom: 8px;
        }

        .summary-value {
          font-size: 32px;
          font-weight: 700;
          color: #0f172a;
        }

        .summary-value.passed {
          color: #16a34a;
        }

        .summary-value.failed {
          color: #dc2626;
        }

        .coverage-matrix {
          background: white;
          border: 1px solid #e2e8f0;
          border-radius: 8px;
          padding: 20px;
          margin-bottom: 24px;
          overflow-x: auto;
        }

        .coverage-matrix h3 {
          font-size: 16px;
          font-weight: 600;
          margin-bottom: 16px;
          color: #0f172a;
        }

        table {
          width: 100%;
          border-collapse: collapse;
        }

        th, td {
          padding: 12px;
          text-align: center;
          border: 1px solid #e2e8f0;
        }

        th {
          background: #f8fafc;
          font-weight: 600;
          color: #475569;
          font-size: 13px;
        }

        td.status-cell {
          font-weight: 600;
          font-size: 16px;
        }

        td.pending {
          color: #94a3b8;
          background: #f8fafc;
        }

        td.passed {
          color: #16a34a;
          background: #f0fdf4;
        }

        td.failed {
          color: #dc2626;
          background: #fef2f2;
        }

        .results-section {
          background: white;
          border: 1px solid #e2e8f0;
          border-radius: 8px;
          padding: 20px;
        }

        .results-header {
          display: flex;
          justify-content: space-between;
          align-items: center;
          margin-bottom: 16px;
        }

        .results-header h3 {
          font-size: 16px;
          font-weight: 600;
          color: #0f172a;
        }

        .filter-buttons {
          display: flex;
          gap: 8px;
        }

        .filter-btn {
          padding: 6px 12px;
          font-size: 13px;
        }

        .filter-btn.active {
          background: #2563eb;
          color: white;
        }

        .results-container {
          max-height: 600px;
          overflow-y: auto;
        }

        .test-result {
          border: 1px solid #e2e8f0;
          border-radius: 6px;
          padding: 16px;
          margin-bottom: 12px;
        }

        .test-result.passed {
          border-left: 4px solid #16a34a;
        }

        .test-result.failed {
          border-left: 4px solid #dc2626;
        }

        .test-result-header {
          display: flex;
          justify-content: space-between;
          align-items: center;
          margin-bottom: 8px;
        }

        .test-name {
          font-weight: 600;
          color: #0f172a;
        }

        .test-status {
          font-size: 13px;
          font-weight: 600;
          padding: 4px 8px;
          border-radius: 4px;
        }

        .test-status.passed {
          color: #16a34a;
          background: #f0fdf4;
        }

        .test-status.failed {
          color: #dc2626;
          background: #fef2f2;
        }

        .test-details {
          font-size: 14px;
          color: #64748b;
        }

        .test-error {
          color: #dc2626;
          margin-top: 8px;
        }

        .empty-state {
          text-align: center;
          padding: 40px;
          color: #94a3b8;
        }

        .error-message {
          background: #fef2f2;
          border: 1px solid #fecaca;
          border-radius: 6px;
          padding: 12px;
          color: #dc2626;
          margin-bottom: 16px;
        }
      </style>

      <div class="container">
        <h1>Object Hooks Test Runner</h1>
        <p class="subtitle">Automated testing for all object hook types (LLM, Datasource, Tool, User)</p>

        <div id="errorContainer"></div>

        <div class="controls">
          <button id="runTests" class="primary-btn">Run All Tests</button>
          <button id="clearResults" class="secondary-btn">Clear Results</button>
        </div>

        <div class="status-bar">
          <div id="statusText" class="status-text">Ready</div>
          <div class="progress-bar">
            <div id="progressFill" class="progress-fill"></div>
          </div>
        </div>

        <div class="summary-cards">
          <div class="summary-card">
            <div class="summary-label">Total Tests</div>
            <div id="totalTests" class="summary-value">24</div>
          </div>
          <div class="summary-card">
            <div class="summary-label">Passed</div>
            <div id="passedTests" class="summary-value passed">0</div>
          </div>
          <div class="summary-card">
            <div class="summary-label">Failed</div>
            <div id="failedTests" class="summary-value failed">0</div>
          </div>
          <div class="summary-card">
            <div class="summary-label">Duration</div>
            <div id="duration" class="summary-value">-</div>
          </div>
        </div>

        <div class="coverage-matrix">
          <h3>Coverage Matrix</h3>
          <table>
            <thead>
              <tr>
                <th>Object Type</th>
                <th>before_create</th>
                <th>after_create</th>
                <th>before_update</th>
                <th>after_update</th>
                <th>before_delete</th>
                <th>after_delete</th>
              </tr>
            </thead>
            <tbody>
              <tr data-object="llm">
                <th>LLM</th>
                <td class="status-cell pending" data-hook="before_create">-</td>
                <td class="status-cell pending" data-hook="after_create">-</td>
                <td class="status-cell pending" data-hook="before_update">-</td>
                <td class="status-cell pending" data-hook="after_update">-</td>
                <td class="status-cell pending" data-hook="before_delete">-</td>
                <td class="status-cell pending" data-hook="after_delete">-</td>
              </tr>
              <tr data-object="datasource">
                <th>Datasource</th>
                <td class="status-cell pending" data-hook="before_create">-</td>
                <td class="status-cell pending" data-hook="after_create">-</td>
                <td class="status-cell pending" data-hook="before_update">-</td>
                <td class="status-cell pending" data-hook="after_update">-</td>
                <td class="status-cell pending" data-hook="before_delete">-</td>
                <td class="status-cell pending" data-hook="after_delete">-</td>
              </tr>
              <tr data-object="tool">
                <th>Tool</th>
                <td class="status-cell pending" data-hook="before_create">-</td>
                <td class="status-cell pending" data-hook="after_create">-</td>
                <td class="status-cell pending" data-hook="before_update">-</td>
                <td class="status-cell pending" data-hook="after_update">-</td>
                <td class="status-cell pending" data-hook="before_delete">-</td>
                <td class="status-cell pending" data-hook="after_delete">-</td>
              </tr>
              <tr data-object="user">
                <th>User</th>
                <td class="status-cell pending" data-hook="before_create">-</td>
                <td class="status-cell pending" data-hook="after_create">-</td>
                <td class="status-cell pending" data-hook="before_update">-</td>
                <td class="status-cell pending" data-hook="after_update">-</td>
                <td class="status-cell pending" data-hook="before_delete">-</td>
                <td class="status-cell pending" data-hook="after_delete">-</td>
              </tr>
            </tbody>
          </table>
        </div>

        <div class="results-section">
          <div class="results-header">
            <h3>Test Results</h3>
            <div class="filter-buttons">
              <button class="filter-btn secondary-btn active" data-filter="all">All</button>
              <button class="filter-btn secondary-btn" data-filter="passed">Passed</button>
              <button class="filter-btn secondary-btn" data-filter="failed">Failed</button>
            </div>
          </div>
          <div id="resultsContainer" class="results-container">
            <div class="empty-state">
              <p>No tests run yet. Click "Run All Tests" to start.</p>
            </div>
          </div>
        </div>
      </div>
    `;
  }

  setupEventListeners() {
    const runTestsBtn = this.shadowRoot.getElementById('runTests');
    const clearResultsBtn = this.shadowRoot.getElementById('clearResults');
    const filterBtns = this.shadowRoot.querySelectorAll('.filter-btn');

    runTestsBtn.addEventListener('click', () => this.runAllTests());
    clearResultsBtn.addEventListener('click', () => this.clearResults());

    filterBtns.forEach(btn => {
      btn.addEventListener('click', (e) => {
        filterBtns.forEach(b => b.classList.remove('active'));
        e.target.classList.add('active');
        this.data.currentFilter = e.target.dataset.filter;
        this.renderResults();
      });
    });
  }

  async runAllTests() {
    if (this.data.isRunning) return;

    this.data.isRunning = true;
    this.data.testResults = [];
    this.data.startTime = Date.now();

    const runTestsBtn = this.shadowRoot.getElementById('runTests');
    runTestsBtn.disabled = true;

    const totalTests = this.objectTypes.length * this.hookTypes.length;
    let completedTests = 0;

    this.updateStatus('Running tests...');
    this.updateProgress(0);
    this.clearMatrix();
    this.clearResultsContainer();

    try {
      // Run tests for each object type and hook type combination
      for (const objectType of this.objectTypes) {
        for (const hookType of this.hookTypes) {
          this.updateStatus(`Testing ${objectType}.${hookType}...`);

          const result = await this.runTest(objectType, hookType);
          this.data.testResults.push(result);

          completedTests++;
          const progress = (completedTests / totalTests) * 100;
          this.updateProgress(progress);

          this.updateMatrix(objectType, hookType, result.passed);
          this.updateSummary();
          this.renderResults();

          // Small delay between tests
          await this.sleep(100);
        }
      }

      const duration = ((Date.now() - this.data.startTime) / 1000).toFixed(2);
      this.shadowRoot.getElementById('duration').textContent = `${duration}s`;
      this.updateStatus('Tests complete!');

    } catch (error) {
      this.updateStatus(`Error: ${error.message}`);
      console.error('Test run error:', error);
    } finally {
      this.data.isRunning = false;
      runTestsBtn.disabled = false;
    }
  }

  async runTest(objectType, hookType) {
    const testName = `${objectType}.${hookType}`;

    try {
      // Call the plugin's RPC method
      const data = await this.pluginAPI.call('run_single_test', {
        object_type: objectType,
        hook_type: hookType
      });

      return {
        name: testName,
        objectType,
        hookType,
        passed: data.success === true,
        error: data.error || null,
        message: data.message || '',
        duration: data.duration || 0
      };

    } catch (error) {
      return {
        name: testName,
        objectType,
        hookType,
        passed: false,
        error: error.message,
        message: '',
        duration: 0
      };
    }
  }

  updateStatus(text) {
    const statusText = this.shadowRoot.getElementById('statusText');
    if (statusText) {
      statusText.textContent = text;
    }
  }

  updateProgress(percent) {
    const progressFill = this.shadowRoot.getElementById('progressFill');
    if (progressFill) {
      progressFill.style.width = `${percent}%`;
    }
  }

  updateSummary() {
    const passed = this.data.testResults.filter(r => r.passed).length;
    const failed = this.data.testResults.filter(r => !r.passed).length;

    this.shadowRoot.getElementById('passedTests').textContent = passed;
    this.shadowRoot.getElementById('failedTests').textContent = failed;
  }

  updateMatrix(objectType, hookType, passed) {
    const row = this.shadowRoot.querySelector(`tr[data-object="${objectType}"]`);
    if (!row) return;

    const cell = row.querySelector(`td[data-hook="${hookType}"]`);
    if (!cell) return;

    cell.classList.remove('pending', 'passed', 'failed');
    cell.classList.add(passed ? 'passed' : 'failed');
    cell.textContent = passed ? '✓' : '✗';
  }

  clearMatrix() {
    const cells = this.shadowRoot.querySelectorAll('.status-cell');
    cells.forEach(cell => {
      cell.classList.remove('passed', 'failed');
      cell.classList.add('pending');
      cell.textContent = '-';
    });
  }

  renderResults() {
    const resultsContainer = this.shadowRoot.getElementById('resultsContainer');

    const filteredResults = this.data.testResults.filter(result => {
      if (this.data.currentFilter === 'all') return true;
      if (this.data.currentFilter === 'passed') return result.passed;
      if (this.data.currentFilter === 'failed') return !result.passed;
      return true;
    });

    if (filteredResults.length === 0) {
      resultsContainer.innerHTML = '<div class="empty-state"><p>No results to display.</p></div>';
      return;
    }

    resultsContainer.innerHTML = filteredResults.map(result => `
      <div class="test-result ${result.passed ? 'passed' : 'failed'}">
        <div class="test-result-header">
          <span class="test-name">${result.name}</span>
          <span class="test-status ${result.passed ? 'passed' : 'failed'}">
            ${result.passed ? '✓ PASSED' : '✗ FAILED'}
          </span>
        </div>
        <div class="test-details">
          ${result.message ? `<div>${result.message}</div>` : ''}
          ${result.error ? `<div class="test-error">Error: ${result.error}</div>` : ''}
        </div>
      </div>
    `).join('');
  }

  clearResultsContainer() {
    const resultsContainer = this.shadowRoot.getElementById('resultsContainer');
    resultsContainer.innerHTML = '<div class="empty-state"><p>No tests run yet. Click "Run All Tests" to start.</p></div>';
  }

  clearResults() {
    this.data.testResults = [];
    this.clearResultsContainer();
    this.clearMatrix();

    this.shadowRoot.getElementById('passedTests').textContent = '0';
    this.shadowRoot.getElementById('failedTests').textContent = '0';
    this.shadowRoot.getElementById('duration').textContent = '-';
    this.updateStatus('Ready');
    this.updateProgress(0);
  }

  showError(message) {
    const errorContainer = this.shadowRoot.getElementById('errorContainer');
    errorContainer.innerHTML = `<div class="error-message">${message}</div>`;
  }

  sleep(ms) {
    return new Promise(resolve => setTimeout(resolve, ms));
  }
}

// Register the custom element
customElements.define('hook-test-runner', HookTestRunner);
