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
    console.log('Dashboard component initialized:', {
      hasPluginAPI: !!this.pluginAPI
    });

    this.render();
    this.setupEventListeners();

    // Wait for plugin API to be injected before loading data
    this.waitForPluginAPI();
  }

  async waitForPluginAPI() {
    // Poll for plugin API availability (max 5 seconds)
    for (let i = 0; i < 50; i++) {
      if (this.pluginAPI) {
        console.log('Plugin API found, loading data...');
        this.loadData();
        return;
      }
      await new Promise(resolve => setTimeout(resolve, 100));
    }

    // Timeout - show error
    console.error('Plugin API injection timeout');
    this.showError('Plugin API initialization timeout - please refresh the page');
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
    if (!this.pluginAPI) {
      throw new Error('Plugin API not available - component not properly initialized');
    }

    try {
      console.log('Dashboard: Calling get_statistics...');
      this.data.statistics = await this.pluginAPI.call('get_statistics', {});
      console.log('Dashboard: get_statistics response:', this.data.statistics);
      this.updateStatistics();
      console.log('Dashboard: Statistics updated successfully');
    } catch (error) {
      console.error('Failed to load statistics:', error);
      throw error;
    }
  }

  async loadRateLimits() {
    if (!this.pluginAPI) {
      throw new Error('Plugin API not available - component not properly initialized');
    }

    try {
      console.log('Dashboard: Calling get_rate_limits...');
      this.data.rateLimits = await this.pluginAPI.call('get_rate_limits', {});
      console.log('Dashboard: get_rate_limits response:', this.data.rateLimits);
      this.updateRateLimits();
      console.log('Dashboard: Rate limits updated successfully');
    } catch (error) {
      console.error('Failed to load rate limits:', error);
      throw error;
    }
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

    // Display service API demo data
    this.shadowRoot.querySelector('#total-plugins').textContent =
      stats.total_plugins?.toLocaleString() || '0';
    this.shadowRoot.querySelector('#total-llms').textContent =
      stats.total_llms?.toLocaleString() || '0';
    this.shadowRoot.querySelector('#service-status').textContent =
      stats.service_status || 'unknown';

    // Show service message
    const messageDiv = this.shadowRoot.querySelector('#service-message');
    if (messageDiv) {
      messageDiv.textContent = stats.message || '';
    }

    // Update plugins table with real data
    const pluginsTableBody = this.shadowRoot.querySelector('#plugins-table tbody');
    if (pluginsTableBody && stats.plugin_list) {
      pluginsTableBody.innerHTML = stats.plugin_list.map(plugin => `
        <tr>
          <td>${plugin.name}</td>
          <td>${plugin.plugin_type}</td>
          <td><span class="status ${plugin.is_active ? 'active' : 'inactive'}">${plugin.is_active ? 'Active' : 'Inactive'}</span></td>
          <td>${plugin.hook_type}</td>
        </tr>
      `).join('');
    }

    // Update LLMs table with real data
    const llmsTableBody = this.shadowRoot.querySelector('#llms-table tbody');
    if (llmsTableBody && stats.llm_list) {
      llmsTableBody.innerHTML = stats.llm_list.map(llm => `
        <tr>
          <td>${llm.name}</td>
          <td>${llm.vendor}</td>
          <td><span class="status ${llm.active ? 'active' : 'inactive'}">${llm.active ? 'Active' : 'Inactive'}</span></td>
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

        .status.active {
          color: #4caf50;
          font-weight: bold;
        }

        .status.inactive {
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

      <div id="loading-spinner">Loading Service API Demo Dashboard...</div>
      <div id="error-message"></div>

      <div id="content" style="display: none;">
        <div class="header">
          <h2>Service API Integration Demo</h2>
          <button id="refresh-btn" class="btn-refresh">Refresh</button>
        </div>

        <div id="service-message" style="background: #e3f2fd; padding: 12px; border-radius: 4px; margin-bottom: 16px; color: #1565c0;"></div>

        <div class="stats-grid">
          <div class="stat-card">
            <div class="stat-value" id="total-plugins">0</div>
            <div class="stat-label">Total Plugins</div>
          </div>
          <div class="stat-card">
            <div class="stat-value" id="total-llms">0</div>
            <div class="stat-label">Total LLMs</div>
          </div>
          <div class="stat-card">
            <div class="stat-value" id="service-status">unknown</div>
            <div class="stat-label">Service Status</div>
          </div>
        </div>

        <div class="section">
          <div class="section-header">Live Plugins (via Service API)</div>
          <table id="plugins-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Type</th>
                <th>Status</th>
                <th>Hook Type</th>
              </tr>
            </thead>
            <tbody>
            </tbody>
          </table>
        </div>

        <div class="section">
          <div class="section-header">Live LLMs (via Service API)</div>
          <table id="llms-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Vendor</th>
                <th>Status</th>
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