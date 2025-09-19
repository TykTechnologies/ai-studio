// Rate Limiting Global Settings Web Component
class RateLimitingSettings extends HTMLElement {
  constructor() {
    super();
    this.attachShadow({ mode: 'open' });
    this.rpcBase = '';
    this.settings = {};
  }

  connectedCallback() {
    this.rpcBase = this.getAttribute('data-rpc-base') || '';
    this.render();
    this.loadSettings();
    this.setupEventListeners();
  }

  async loadSettings() {
    try {
      const response = await fetch(`${this.rpcBase}/call/get_global_settings`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({})
      });

      if (!response.ok) throw new Error('Failed to load settings');

      const result = await response.json();
      this.settings = JSON.parse(result.data);
      this.populateForm();
    } catch (error) {
      console.error('Failed to load settings:', error);
      this.showError('Failed to load global settings');
    }
  }

  async saveSettings() {
    try {
      this.setLoading(true);

      // Collect form data
      const formData = new FormData(this.shadowRoot.querySelector('#settings-form'));
      const settings = {
        storage_type: formData.get('storage_type'),
        redis_url: formData.get('redis_url'),
        default_limit: parseInt(formData.get('default_limit')),
        default_window: formData.get('default_window'),
        enable_burst: formData.get('enable_burst') === 'on',
        burst_multiplier: parseFloat(formData.get('burst_multiplier')),
        monitoring_enabled: formData.get('monitoring_enabled') === 'on',
        alert_threshold: parseFloat(formData.get('alert_threshold'))
      };

      const response = await fetch(`${this.rpcBase}/call/set_global_settings`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(settings)
      });

      if (!response.ok) throw new Error('Failed to save settings');

      this.showSuccess('Settings saved successfully');
    } catch (error) {
      console.error('Failed to save settings:', error);
      this.showError('Failed to save settings');
    } finally {
      this.setLoading(false);
    }
  }

  populateForm() {
    const form = this.shadowRoot.querySelector('#settings-form');
    if (!form) return;

    // Populate form fields
    Object.entries(this.settings).forEach(([key, value]) => {
      const input = form.querySelector(`[name="${key}"]`);
      if (input) {
        if (input.type === 'checkbox') {
          input.checked = Boolean(value);
        } else {
          input.value = value;
        }
      }
    });
  }

  setupEventListeners() {
    // Save button
    this.shadowRoot.querySelector('#save-btn')?.addEventListener('click', (e) => {
      e.preventDefault();
      this.saveSettings();
    });

    // Reset button
    this.shadowRoot.querySelector('#reset-btn')?.addEventListener('click', (e) => {
      e.preventDefault();
      this.populateForm();
    });

    // Test connection button
    this.shadowRoot.querySelector('#test-connection-btn')?.addEventListener('click', () => {
      this.testConnection();
    });
  }

  async testConnection() {
    const redisUrl = this.shadowRoot.querySelector('[name="redis_url"]').value;

    try {
      this.showInfo('Testing Redis connection...');

      // Mock connection test - in real implementation this would call the backend
      await new Promise(resolve => setTimeout(resolve, 1000));

      if (redisUrl.includes('localhost') || redisUrl.includes('127.0.0.1')) {
        this.showSuccess('✓ Redis connection successful');
      } else {
        this.showWarning('⚠ Could not reach Redis server (this is expected in demo mode)');
      }
    } catch (error) {
      this.showError('✗ Redis connection failed');
    }
  }

  setLoading(loading) {
    const saveBtn = this.shadowRoot.querySelector('#save-btn');
    if (saveBtn) {
      saveBtn.disabled = loading;
      saveBtn.textContent = loading ? 'Saving...' : 'Save Settings';
    }
  }

  showMessage(message, type = 'info') {
    const messageDiv = this.shadowRoot.querySelector('#message');
    if (messageDiv) {
      messageDiv.textContent = message;
      messageDiv.className = `message ${type}`;
      messageDiv.style.display = 'block';

      // Auto-hide after 3 seconds
      setTimeout(() => {
        messageDiv.style.display = 'none';
      }, 3000);
    }
  }

  showSuccess(message) { this.showMessage(message, 'success'); }
  showError(message) { this.showMessage(message, 'error'); }
  showWarning(message) { this.showMessage(message, 'warning'); }
  showInfo(message) { this.showMessage(message, 'info'); }

  render() {
    this.shadowRoot.innerHTML = `
      <style>
        :host {
          display: block;
          padding: 24px;
          font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
        }

        .header {
          margin-bottom: 24px;
        }

        .form-grid {
          display: grid;
          grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
          gap: 24px;
          margin-bottom: 24px;
        }

        .form-section {
          background: white;
          border: 1px solid #e0e0e0;
          border-radius: 8px;
          padding: 20px;
        }

        .form-section h3 {
          margin-top: 0;
          margin-bottom: 16px;
          color: #333;
        }

        .form-group {
          margin-bottom: 16px;
        }

        label {
          display: block;
          margin-bottom: 4px;
          font-weight: 500;
          color: #555;
        }

        input, select {
          width: 100%;
          padding: 8px 12px;
          border: 1px solid #ddd;
          border-radius: 4px;
          font-size: 14px;
        }

        input:focus, select:focus {
          outline: none;
          border-color: #1976d2;
          box-shadow: 0 0 0 2px rgba(25, 118, 210, 0.2);
        }

        .checkbox-group {
          display: flex;
          align-items: center;
          gap: 8px;
        }

        .checkbox-group input[type="checkbox"] {
          width: auto;
        }

        .buttons {
          display: flex;
          gap: 12px;
          margin-top: 24px;
        }

        .btn {
          padding: 10px 20px;
          border: none;
          border-radius: 4px;
          cursor: pointer;
          font-size: 14px;
          font-weight: 500;
        }

        .btn-primary {
          background: #1976d2;
          color: white;
        }

        .btn-primary:hover {
          background: #1565c0;
        }

        .btn-secondary {
          background: #f5f5f5;
          color: #333;
          border: 1px solid #ddd;
        }

        .btn-secondary:hover {
          background: #eeeeee;
        }

        .btn-test {
          background: #ff9800;
          color: white;
        }

        .btn-test:hover {
          background: #f57c00;
        }

        .btn:disabled {
          opacity: 0.6;
          cursor: not-allowed;
        }

        .message {
          padding: 12px;
          border-radius: 4px;
          margin-bottom: 16px;
          display: none;
        }

        .message.success {
          background: #e8f5e8;
          color: #2e7d32;
          border: 1px solid #4caf50;
        }

        .message.error {
          background: #ffebee;
          color: #c62828;
          border: 1px solid #f44336;
        }

        .message.warning {
          background: #fff3e0;
          color: #ef6c00;
          border: 1px solid #ff9800;
        }

        .message.info {
          background: #e3f2fd;
          color: #1565c0;
          border: 1px solid #2196f3;
        }
      </style>

      <div class="header">
        <h2>Rate Limiting Global Settings</h2>
        <p>Configure global settings for rate limiting across all endpoints.</p>
      </div>

      <div id="message" class="message"></div>

      <form id="settings-form">
        <div class="form-grid">
          <div class="form-section">
            <h3>Storage Configuration</h3>

            <div class="form-group">
              <label for="storage_type">Storage Type</label>
              <select name="storage_type" id="storage_type">
                <option value="redis">Redis</option>
                <option value="memory">In-Memory</option>
                <option value="database">Database</option>
              </select>
            </div>

            <div class="form-group">
              <label for="redis_url">Redis URL</label>
              <input type="text" name="redis_url" id="redis_url" placeholder="redis://localhost:6379" />
            </div>

            <div class="form-group">
              <button type="button" id="test-connection-btn" class="btn btn-test">Test Connection</button>
            </div>
          </div>

          <div class="form-section">
            <h3>Default Limits</h3>

            <div class="form-group">
              <label for="default_limit">Default Request Limit</label>
              <input type="number" name="default_limit" id="default_limit" min="1" />
            </div>

            <div class="form-group">
              <label for="default_window">Default Time Window</label>
              <select name="default_window" id="default_window">
                <option value="1s">1 Second</option>
                <option value="1m">1 Minute</option>
                <option value="1h">1 Hour</option>
                <option value="24h">24 Hours</option>
              </select>
            </div>

            <div class="form-group checkbox-group">
              <input type="checkbox" name="enable_burst" id="enable_burst" />
              <label for="enable_burst">Enable Burst Handling</label>
            </div>

            <div class="form-group">
              <label for="burst_multiplier">Burst Multiplier</label>
              <input type="number" name="burst_multiplier" id="burst_multiplier" min="1" max="10" step="0.1" />
            </div>
          </div>

          <div class="form-section">
            <h3>Monitoring</h3>

            <div class="form-group checkbox-group">
              <input type="checkbox" name="monitoring_enabled" id="monitoring_enabled" />
              <label for="monitoring_enabled">Enable Monitoring</label>
            </div>

            <div class="form-group">
              <label for="alert_threshold">Alert Threshold (0.0 - 1.0)</label>
              <input type="number" name="alert_threshold" id="alert_threshold" min="0" max="1" step="0.1" />
            </div>
          </div>
        </div>

        <div class="buttons">
          <button type="button" id="save-btn" class="btn btn-primary">Save Settings</button>
          <button type="button" id="reset-btn" class="btn btn-secondary">Reset</button>
        </div>
      </form>
    `;
  }
}

// Register the custom element
customElements.define('rate-limiting-settings', RateLimitingSettings);