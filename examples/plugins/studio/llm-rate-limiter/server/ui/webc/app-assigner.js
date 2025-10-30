// Rate Limit App Assigner Web Component
class RateLimitAppAssigner extends HTMLElement {
  constructor() {
    super();
    this.attachShadow({ mode: 'open' });
    this.data = {
      apps: [],
      policies: [],
      loading: true
    };
  }

  connectedCallback() {
    console.log('RateLimitAppAssigner component initialized');
    this.render();
    this.setupEventListeners();
    this.waitForPluginAPI();
  }

  async waitForPluginAPI() {
    for (let i = 0; i < 50; i++) {
      if (this.pluginAPI) {
        console.log('Plugin API found, loading data...');
        this.loadData();
        return;
      }
      await new Promise(resolve => setTimeout(resolve, 100));
    }
    console.error('Plugin API injection timeout');
    this.showError('Plugin API initialization timeout - please refresh the page');
  }

  async loadData() {
    this.setLoading(true);
    try {
      console.log('Loading apps and policies...');

      // Load both apps and policies in parallel
      const [appsResult, policiesResult] = await Promise.all([
        this.pluginAPI.call('listAppsWithPolicies', {}),
        this.pluginAPI.call('listPolicies', {})
      ]);

      console.log('Apps loaded:', appsResult);
      console.log('Policies loaded:', policiesResult);

      this.data.apps = appsResult.apps || [];
      this.data.policies = policiesResult.policies || [];

      this.updateAppsTable();
      this.updateStats();

    } catch (error) {
      console.error('Failed to load data:', error);
      this.showError('Failed to load data: ' + error.message);
    } finally {
      this.setLoading(false);
    }
  }

  async assignPolicy(appId, policyName, enabled, overrides) {
    try {
      const result = await this.pluginAPI.call('assignPolicy', {
        app_id: appId,
        policy_name: policyName,
        enabled: enabled,
        overrides: overrides || {}
      });

      console.log('Policy assigned:', result);
      this.showSuccess('Rate limit policy assigned successfully');

      await this.loadData();

      return result;
    } catch (error) {
      console.error('Failed to assign policy:', error);
      throw error;
    }
  }

  async removePolicy(appId) {
    if (!confirm('Are you sure you want to remove the rate limit policy from this app?')) {
      return;
    }

    try {
      const result = await this.pluginAPI.call('removePolicy', { app_id: appId });

      console.log('Policy removed:', result);
      this.showSuccess('Rate limit policy removed successfully');

      await this.loadData();

      return result;
    } catch (error) {
      console.error('Failed to remove policy:', error);
      this.showError('Failed to remove policy: ' + error.message);
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

  updateStats() {
    const appsWithPolicy = this.data.apps.filter(app => app.rate_limit && app.rate_limit.policy_name);
    const enabledApps = this.data.apps.filter(app => app.rate_limit && app.rate_limit.enabled);

    const totalCount = this.shadowRoot.querySelector('#apps-total');
    const withPolicyCount = this.shadowRoot.querySelector('#apps-with-policy');
    const enabledCount = this.shadowRoot.querySelector('#apps-enabled');

    if (totalCount) totalCount.textContent = this.data.apps.length;
    if (withPolicyCount) withPolicyCount.textContent = appsWithPolicy.length;
    if (enabledCount) enabledCount.textContent = enabledApps.length;
  }

  updateAppsTable() {
    const tableBody = this.shadowRoot.querySelector('#apps-table tbody');
    if (!tableBody) return;

    if (this.data.apps.length === 0) {
      tableBody.innerHTML = `
        <tr>
          <td colspan="5" style="text-align: center; color: #999; padding: 40px;">
            No applications found in the system.
          </td>
        </tr>
      `;
      return;
    }

    tableBody.innerHTML = this.data.apps.map(app => {
      const hasPolicy = app.rate_limit && app.rate_limit.policy_name;
      const isEnabled = hasPolicy && app.rate_limit.enabled;

      let policyBadge = '<span style="color: #999;">No policy</span>';
      if (hasPolicy) {
        const color = isEnabled ? '#4caf50' : '#ff9800';
        const status = isEnabled ? 'Active' : 'Disabled';
        policyBadge = `
          <div>
            <strong>${app.rate_limit.policy_name}</strong>
            <span style="display: inline-block; margin-left: 8px; padding: 2px 8px; border-radius: 3px; background: ${color}; color: white; font-size: 11px;">
              ${status}
            </span>
          </div>
        `;
      }

      return `
        <tr>
          <td>
            <strong>${app.name}</strong>
            ${app.description ? `<div style="font-size: 12px; color: #666;">${app.description}</div>` : ''}
          </td>
          <td><code style="font-size: 12px;">${app.id}</code></td>
          <td>${app.owner_email || '<em>N/A</em>'}</td>
          <td>${policyBadge}</td>
          <td>
            <button class="btn-assign" data-id="${app.id}">
              ${hasPolicy ? 'Edit' : 'Assign'}
            </button>
            ${hasPolicy ? `<button class="btn-remove" data-id="${app.id}">Remove</button>` : ''}
          </td>
        </tr>
      `;
    }).join('');
  }

  setupEventListeners() {
    // Refresh button
    this.shadowRoot.querySelector('#refresh-btn')?.addEventListener('click', () => {
      this.loadData();
    });

    // Event delegation for table buttons
    this.shadowRoot.addEventListener('click', async (e) => {
      const appId = e.target.getAttribute('data-id');

      if (e.target.classList.contains('btn-assign')) {
        this.showAssignDialog(parseInt(appId, 10));
      } else if (e.target.classList.contains('btn-remove')) {
        await this.removePolicy(parseInt(appId, 10));
      }
    });
  }

  showAssignDialog(appId) {
    const app = this.data.apps.find(a => a.id === appId);
    if (!app) {
      this.showError('App not found');
      return;
    }

    const existingPolicy = app.rate_limit?.policy_name || '';
    const existingEnabled = app.rate_limit?.enabled !== false; // Default to true

    const dialog = document.createElement('div');
    dialog.className = 'dialog-overlay';
    dialog.innerHTML = `
      <div class="dialog">
        <h3>${existingPolicy ? 'Edit' : 'Assign'} Rate Limit Policy</h3>

        <div style="padding: 12px; background: #f5f5f5; border-radius: 4px; margin-bottom: 20px;">
          <strong>App:</strong> ${app.name}<br>
          <strong>ID:</strong> ${app.id}<br>
          <strong>Owner:</strong> ${app.owner_email || 'N/A'}
        </div>

        ${this.data.policies.length === 0 ? `
          <div style="padding: 16px; background: #fff3cd; border-left: 4px solid #ffc107; margin-bottom: 20px;">
            <strong>No policies available!</strong><br>
            You need to create at least one rate limit policy before assigning it to an app.
            Go to the "Rate Limit Policies" page to create a policy.
          </div>
        ` : ''}

        <div class="form-group">
          <label>Policy <span style="color: red;">*</span></label>
          <select id="policy-select" ${this.data.policies.length === 0 ? 'disabled' : ''}>
            <option value="">-- Select a policy --</option>
            ${this.data.policies.map(policy => `
              <option value="${policy.name}" ${policy.name === existingPolicy ? 'selected' : ''}>
                ${policy.name}${policy.description ? ` - ${policy.description}` : ''}
              </option>
            `).join('')}
          </select>
        </div>

        <div class="form-group">
          <label style="display: flex; align-items: center; cursor: pointer;">
            <input type="checkbox" id="enabled-checkbox" ${existingEnabled ? 'checked' : ''} style="margin-right: 8px;" />
            <span>Enable rate limiting</span>
          </label>
          <small>When disabled, the policy is assigned but not enforced</small>
        </div>

        <div id="policy-details" style="display: none; padding: 16px; background: #f9f9f9; border-radius: 4px; margin-top: 16px;">
          <h4 style="margin: 0 0 12px 0; color: #555;">Policy Limits</h4>
          <div id="policy-limits-display"></div>
        </div>

        <div class="dialog-actions">
          <button class="btn-cancel">Cancel</button>
          <button class="btn-primary" id="submit-assign" ${this.data.policies.length === 0 ? 'disabled' : ''}>
            ${existingPolicy ? 'Update' : 'Assign'} Policy
          </button>
        </div>
      </div>
    `;

    this.shadowRoot.appendChild(dialog);

    // Show policy details when selected
    const policySelect = dialog.querySelector('#policy-select');
    const policyDetails = dialog.querySelector('#policy-details');
    const policyLimitsDisplay = dialog.querySelector('#policy-limits-display');

    const updatePolicyDisplay = () => {
      const selectedPolicyName = policySelect.value;
      if (!selectedPolicyName) {
        policyDetails.style.display = 'none';
        return;
      }

      const policy = this.data.policies.find(p => p.name === selectedPolicyName);
      if (!policy) {
        policyDetails.style.display = 'none';
        return;
      }

      const modelEntries = Object.entries(policy.models || {});
      policyLimitsDisplay.innerHTML = `
        <table style="width: 100%; font-size: 13px;">
          <thead>
            <tr style="border-bottom: 1px solid #ddd;">
              <th style="text-align: left; padding: 6px;">Model</th>
              <th style="text-align: right; padding: 6px;">TPM</th>
              <th style="text-align: right; padding: 6px;">RPM</th>
              <th style="text-align: right; padding: 6px;">Concurrent</th>
            </tr>
          </thead>
          <tbody>
            ${modelEntries.map(([model, limits]) => `
              <tr>
                <td style="padding: 6px;"><code style="font-size: 12px;">${model === '*' ? '* (default)' : model}</code></td>
                <td style="text-align: right; padding: 6px;">${limits.tpm.toLocaleString()}</td>
                <td style="text-align: right; padding: 6px;">${limits.rpm}</td>
                <td style="text-align: right; padding: 6px;">${limits.concurrent}</td>
              </tr>
            `).join('')}
          </tbody>
        </table>
      `;
      policyDetails.style.display = 'block';
    };

    policySelect.addEventListener('change', updatePolicyDisplay);

    // Show policy details if one is already selected
    if (existingPolicy) {
      updatePolicyDisplay();
    }

    // Handle cancel
    dialog.querySelector('.btn-cancel').addEventListener('click', () => {
      dialog.remove();
    });

    // Handle submit
    dialog.querySelector('#submit-assign').addEventListener('click', async () => {
      const policyName = policySelect.value;
      const enabled = dialog.querySelector('#enabled-checkbox').checked;

      if (!policyName) {
        this.showError('Please select a policy');
        return;
      }

      const btn = dialog.querySelector('#submit-assign');
      btn.disabled = true;
      btn.textContent = existingPolicy ? 'Updating...' : 'Assigning...';

      try {
        await this.assignPolicy(appId, policyName, enabled, {});
        dialog.remove();
      } catch (error) {
        this.showError('Failed to assign policy: ' + error.message);
        btn.disabled = false;
        btn.textContent = existingPolicy ? 'Update' : 'Assign' + ' Policy';
      }
    });
  }

  showError(message) {
    this.showMessage(message, 'error');
  }

  showSuccess(message) {
    this.showMessage(message, 'success');
  }

  showMessage(message, type) {
    const messageDiv = this.shadowRoot.querySelector('#message');
    if (!messageDiv) return;

    messageDiv.style.display = 'block';
    messageDiv.textContent = message;

    if (type === 'success') {
      messageDiv.style.background = '#e8f5e9';
      messageDiv.style.color = '#2e7d32';
      messageDiv.style.borderLeft = '4px solid #4caf50';
    } else {
      messageDiv.style.background = '#ffebee';
      messageDiv.style.color = '#c62828';
      messageDiv.style.borderLeft = '4px solid #f44336';
    }

    setTimeout(() => {
      messageDiv.style.display = 'none';
    }, 5000);
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

        h2 {
          margin: 0;
          color: #333;
        }

        .header-actions {
          display: flex;
          gap: 12px;
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
          padding: 20px;
          text-align: center;
        }

        .stat-value {
          font-size: 32px;
          font-weight: bold;
          color: #1976d2;
          margin-bottom: 8px;
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
          padding: 20px;
          border-bottom: 1px solid #e0e0e0;
          font-weight: bold;
          font-size: 18px;
          color: #333;
        }

        table {
          width: 100%;
          border-collapse: collapse;
        }

        th, td {
          padding: 14px;
          text-align: left;
          border-bottom: 1px solid #e0e0e0;
        }

        th {
          background: #f5f5f5;
          font-weight: bold;
          color: #555;
          font-size: 13px;
          text-transform: uppercase;
        }

        tbody tr:hover {
          background: #fafafa;
        }

        code {
          background: #f5f5f5;
          padding: 2px 6px;
          border-radius: 3px;
          font-family: 'Monaco', 'Courier New', monospace;
          font-size: 13px;
        }

        button {
          padding: 8px 16px;
          border: none;
          border-radius: 4px;
          cursor: pointer;
          font-size: 14px;
          font-weight: 500;
          transition: all 0.2s;
        }

        button:hover {
          transform: translateY(-1px);
          box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }

        button:disabled {
          opacity: 0.5;
          cursor: not-allowed;
          transform: none;
        }

        .btn-primary {
          background: #1976d2;
          color: white;
        }

        .btn-primary:hover:not(:disabled) {
          background: #1565c0;
        }

        .btn-assign {
          background: #4caf50;
          color: white;
          padding: 6px 12px;
          font-size: 13px;
        }

        .btn-assign:hover {
          background: #45a049;
        }

        .btn-remove {
          background: #f44336;
          color: white;
          padding: 6px 12px;
          font-size: 13px;
        }

        .btn-remove:hover {
          background: #d32f2f;
        }

        .btn-cancel {
          background: #e0e0e0;
          color: #333;
        }

        .btn-cancel:hover {
          background: #d0d0d0;
        }

        #loading-spinner {
          text-align: center;
          padding: 60px;
          color: #666;
          font-size: 16px;
        }

        #message {
          padding: 14px;
          border-radius: 4px;
          margin-bottom: 20px;
          display: none;
        }

        .dialog-overlay {
          position: fixed;
          top: 0;
          left: 0;
          right: 0;
          bottom: 0;
          background: rgba(0, 0, 0, 0.5);
          display: flex;
          align-items: center;
          justify-content: center;
          z-index: 1000;
        }

        .dialog {
          background: white;
          border-radius: 8px;
          padding: 24px;
          width: 600px;
          max-width: 90vw;
          max-height: 90vh;
          overflow-y: auto;
        }

        .dialog h3 {
          margin: 0 0 16px 0;
          color: #333;
        }

        .form-group {
          margin-bottom: 16px;
        }

        .form-group label {
          display: block;
          margin-bottom: 6px;
          font-weight: 500;
          color: #555;
          font-size: 14px;
        }

        .form-group select {
          width: 100%;
          padding: 10px;
          border: 1px solid #ddd;
          border-radius: 4px;
          font-size: 14px;
          font-family: inherit;
          box-sizing: border-box;
        }

        .form-group select:focus {
          outline: none;
          border-color: #1976d2;
        }

        .form-group small {
          display: block;
          margin-top: 4px;
          color: #666;
          font-size: 12px;
        }

        .dialog-actions {
          display: flex;
          gap: 12px;
          justify-content: flex-end;
          margin-top: 24px;
        }

        h4 {
          color: #555;
          font-size: 15px;
          margin: 16px 0 8px 0;
        }
      </style>

      <div id="loading-spinner">Loading Applications...</div>
      <div id="message"></div>

      <div id="content" style="display: none;">
        <div class="header">
          <h2>App Rate Limit Assignments</h2>
          <div class="header-actions">
            <button id="refresh-btn" class="btn-primary">Refresh</button>
          </div>
        </div>

        <div class="stats-grid">
          <div class="stat-card">
            <div class="stat-value" id="apps-total">0</div>
            <div class="stat-label">Total Apps</div>
          </div>
          <div class="stat-card">
            <div class="stat-value" id="apps-with-policy">0</div>
            <div class="stat-label">With Policy</div>
          </div>
          <div class="stat-card">
            <div class="stat-value" id="apps-enabled">0</div>
            <div class="stat-label">Enforcement Active</div>
          </div>
        </div>

        <div class="section">
          <div class="section-header">Applications</div>
          <table id="apps-table">
            <thead>
              <tr>
                <th>App Name</th>
                <th>ID</th>
                <th>Owner</th>
                <th>Rate Limit Policy</th>
                <th>Actions</th>
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
customElements.define('rate-limit-app-assigner', RateLimitAppAssigner);
