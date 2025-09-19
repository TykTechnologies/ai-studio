// Rate Limiting Dashboard Web Component
class RateLimitingDashboard extends HTMLElement {
  constructor() {
    super();
    this.attachShadow({ mode: 'open' });
    this.rpcBase = '';
    this.data = {
      statistics: {},
      rateLimits: {},
      loading: true
    };
  }

  connectedCallback() {
    this.rpcBase = this.getAttribute('data-rpc-base') || '';
    this.render();
    this.loadData();
    this.setupEventListeners();
  }

  async loadData() {
    this.setLoading(true);
    try {
      await Promise.all([
        this.loadStatistics(),
        this.loadRateLimits()
      ]);
    } catch (error) {
      console.error('Failed to load dashboard data:', error);
      this.showError('Failed to load dashboard data');
    } finally {
      this.setLoading(false);
    }
  }

  async loadStatistics() {
    const response = await fetch(`${this.rpcBase}/call/get_statistics`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({})
    });

    if (!response.ok) throw new Error('Failed to load statistics');

    const result = await response.json();
    this.data.statistics = JSON.parse(result.data);
    this.updateStatistics();
  }

  async loadRateLimits() {
    const response = await fetch(`${this.rpcBase}/call/get_rate_limits`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({})
    });

    if (!response.ok) throw new Error('Failed to load rate limits');

    const result = await response.json();
    this.data.rateLimits = JSON.parse(result.data);
    this.updateRateLimits();
  }

  setLoading(loading) {
    this.data.loading = loading;
    const spinner = this.shadowRoot.querySelector('#loading-spinner');
    const content = this.shadowRoot.querySelector('#content');

    if (spinner && content) {
      spinner.style.display = loading ? 'block' : 'none';
      content.style.display = loading ? 'none' : 'block';
    }
  }

  updateStatistics() {
    const stats = this.data.statistics;

    this.shadowRoot.querySelector('#total-requests').textContent =
      stats.total_requests?.toLocaleString() || '0';
    this.shadowRoot.querySelector('#blocked-requests').textContent =
      stats.blocked_requests?.toLocaleString() || '0';
    this.shadowRoot.querySelector('#success-rate').textContent =
      `${((stats.success_rate || 0) * 100).toFixed(1)}%`;

    // Update top endpoints table
    const tableBody = this.shadowRoot.querySelector('#top-endpoints tbody');
    if (tableBody && stats.top_endpoints) {
      tableBody.innerHTML = stats.top_endpoints.map(endpoint => `
        <tr>
          <td>${endpoint.path}</td>
          <td>${endpoint.requests.toLocaleString()}</td>
          <td>${endpoint.blocked.toLocaleString()}</td>
          <td>${(((endpoint.requests - endpoint.blocked) / endpoint.requests) * 100).toFixed(1)}%</td>
        </tr>
      `).join('');
    }
  }

  updateRateLimits() {
    const rateLimits = this.data.rateLimits;

    // Update endpoints table
    const tableBody = this.shadowRoot.querySelector('#rate-limits tbody');
    if (tableBody && rateLimits.endpoints) {
      tableBody.innerHTML = rateLimits.endpoints.map(endpoint => `
        <tr>
          <td>${endpoint.path}</td>
          <td>${endpoint.method}</td>
          <td>${endpoint.limit}/${endpoint.window}</td>
          <td>
            <span class="status ${endpoint.enabled ? 'enabled' : 'disabled'}">
              ${endpoint.enabled ? 'Enabled' : 'Disabled'}
            </span>
          </td>
          <td>
            <button class="btn-edit" data-id="${endpoint.id}">Edit</button>
          </td>
        </tr>
      `).join('');
    }
  }

  setupEventListeners() {
    // Refresh button
    this.shadowRoot.querySelector('#refresh-btn')?.addEventListener('click', () => {
      this.loadData();
    });

    // Edit rate limit buttons (event delegation)
    this.shadowRoot.addEventListener('click', (e) => {
      if (e.target.classList.contains('btn-edit')) {
        const endpointId = e.target.getAttribute('data-id');
        this.editRateLimit(endpointId);
      }
    });
  }

  editRateLimit(endpointId) {
    // Navigate to rate limit configuration (replace current plugin config UI)
    const event = new CustomEvent('plugin-navigate', {
      detail: { path: `/admin/plugins/rate-limit-config/${endpointId}` },
      bubbles: true
    });
    this.dispatchEvent(event);
  }

  showError(message) {
    const errorDiv = this.shadowRoot.querySelector('#error-message');
    if (errorDiv) {
      errorDiv.textContent = message;
      errorDiv.style.display = 'block';
    }
  }

  render() {
    this.shadowRoot.innerHTML = `
      <style>
        :host {
          display: block;
          padding: 24px;
          font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
        }

        .header {
          display: flex;
          justify-content: space-between;
          align-items: center;
          margin-bottom: 24px;
        }

        .stats-grid {
          display: grid;
          grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
          gap: 16px;
          margin-bottom: 32px;
        }

        .stat-card {
          background: white;
          border: 1px solid #e0e0e0;
          border-radius: 8px;
          padding: 16px;
          text-align: center;
        }

        .stat-value {
          font-size: 24px;
          font-weight: bold;
          color: #1976d2;
          margin-bottom: 4px;
        }

        .stat-label {
          font-size: 14px;
          color: #666;
        }

        .section {
          background: white;
          border: 1px solid #e0e0e0;
          border-radius: 8px;
          margin-bottom: 24px;
        }

        .section-header {
          padding: 16px;
          border-bottom: 1px solid #e0e0e0;
          font-weight: bold;
        }

        table {
          width: 100%;
          border-collapse: collapse;
        }

        th, td {
          padding: 12px;
          text-align: left;
          border-bottom: 1px solid #e0e0e0;
        }

        th {
          background: #f5f5f5;
          font-weight: bold;
        }

        .status.enabled {
          color: #4caf50;
          font-weight: bold;
        }

        .status.disabled {
          color: #f44336;
          font-weight: bold;
        }

        .btn-edit {
          background: #1976d2;
          color: white;
          border: none;
          padding: 6px 12px;
          border-radius: 4px;
          cursor: pointer;
        }

        .btn-edit:hover {
          background: #1565c0;
        }

        .btn-refresh {
          background: #4caf50;
          color: white;
          border: none;
          padding: 8px 16px;
          border-radius: 4px;
          cursor: pointer;
        }

        .btn-refresh:hover {
          background: #45a049;
        }

        #loading-spinner {
          text-align: center;
          padding: 40px;
        }

        #error-message {
          background: #ffebee;
          color: #c62828;
          padding: 12px;
          border-radius: 4px;
          margin-bottom: 16px;
          display: none;
        }
      </style>

      <div id="loading-spinner">Loading rate limiting dashboard...</div>
      <div id="error-message"></div>

      <div id="content" style="display: none;">
        <div class="header">
          <h2>Rate Limiting Dashboard</h2>
          <button id="refresh-btn" class="btn-refresh">Refresh</button>
        </div>

        <div class="stats-grid">
          <div class="stat-card">
            <div class="stat-value" id="total-requests">0</div>
            <div class="stat-label">Total Requests</div>
          </div>
          <div class="stat-card">
            <div class="stat-value" id="blocked-requests">0</div>
            <div class="stat-label">Blocked Requests</div>
          </div>
          <div class="stat-card">
            <div class="stat-value" id="success-rate">0%</div>
            <div class="stat-label">Success Rate</div>
          </div>
        </div>

        <div class="section">
          <div class="section-header">Rate Limit Configurations</div>
          <table id="rate-limits">
            <thead>
              <tr>
                <th>Endpoint</th>
                <th>Method</th>
                <th>Limit</th>
                <th>Status</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
            </tbody>
          </table>
        </div>

        <div class="section">
          <div class="section-header">Top Endpoints by Traffic</div>
          <table id="top-endpoints">
            <thead>
              <tr>
                <th>Endpoint</th>
                <th>Requests</th>
                <th>Blocked</th>
                <th>Success Rate</th>
              </tr>
            </thead>
            <tbody>
            </tbody>
          </table>
        </div>
      </div>
    `;
  }
}

// Register the custom element
customElements.define('rate-limiting-dashboard', RateLimitingDashboard);