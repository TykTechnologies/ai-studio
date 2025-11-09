// Rate Limit Policy Manager Web Component
class RateLimitPolicyManager extends HTMLElement {
  constructor() {
    super();
    this.attachShadow({ mode: 'open' });
    this.data = {
      policies: [],
      loading: true,
      editingPolicy: null,
      showAddForm: false
    };
  }

  connectedCallback() {
    console.log('RateLimitPolicyManager component initialized');
    this.render();
    this.setupEventListeners();
    this.waitForPluginAPI();
  }

  async waitForPluginAPI() {
    for (let i = 0; i < 50; i++) {
      if (this.pluginAPI) {
        console.log('Plugin API found, loading policies...');
        this.loadPolicies();
        return;
      }
      await new Promise(resolve => setTimeout(resolve, 100));
    }
    console.error('Plugin API injection timeout');
    this.showError('Plugin API initialization timeout - please refresh the page');
  }

  async loadPolicies() {
    this.setLoading(true);
    try {
      console.log('Loading policies via RPC...');
      const result = await this.pluginAPI.call('listPolicies', {});
      console.log('Policies loaded:', result);

      this.data.policies = result.policies || [];
      this.updatePoliciesTable();

      const statsDiv = this.shadowRoot.querySelector('#policy-count');
      if (statsDiv) {
        statsDiv.textContent = this.data.policies.length;
      }
    } catch (error) {
      console.error('Failed to load policies:', error);
      this.showError('Failed to load policies: ' + error.message);
    } finally {
      this.setLoading(false);
    }
  }

  async createPolicy(name, description, models) {
    try {
      const result = await this.pluginAPI.call('createPolicy', {
        name: name,
        description: description,
        models: models
      });

      console.log('Policy created:', result);
      this.showSuccess('Policy created successfully');

      await this.loadPolicies();

      this.data.showAddForm = false;
      this.updateFormVisibility();

      return result;
    } catch (error) {
      console.error('Failed to create policy:', error);
      throw error;
    }
  }

  async updatePolicy(name, description, models) {
    try {
      const result = await this.pluginAPI.call('updatePolicy', {
        name: name,
        description: description,
        models: models
      });

      console.log('Policy updated:', result);
      this.showSuccess('Policy updated successfully');

      await this.loadPolicies();

      this.data.editingPolicy = null;
      return result;
    } catch (error) {
      console.error('Failed to update policy:', error);
      throw error;
    }
  }

  async deletePolicy(name) {
    if (!confirm(`Are you sure you want to delete the policy "${name}"? This action cannot be undone.`)) {
      return;
    }

    try {
      const result = await this.pluginAPI.call('deletePolicy', { name: name });

      console.log('Policy deleted:', result);
      this.showSuccess('Policy deleted successfully');

      await this.loadPolicies();

      return result;
    } catch (error) {
      console.error('Failed to delete policy:', error);
      this.showError('Failed to delete policy: ' + error.message);
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

  updatePoliciesTable() {
    const tableBody = this.shadowRoot.querySelector('#policies-table tbody');
    if (!tableBody) return;

    if (this.data.policies.length === 0) {
      tableBody.innerHTML = `
        <tr>
          <td colspan="5" style="text-align: center; color: #999; padding: 40px;">
            No policies configured. Click "Add Policy" to create your first rate limit policy.
          </td>
        </tr>
      `;
      return;
    }

    tableBody.innerHTML = this.data.policies.map(policy => {
      const modelCount = Object.keys(policy.models || {}).length;
      const hasWildcard = policy.models && policy.models['*'];

      return `
        <tr>
          <td><strong>${policy.name}</strong></td>
          <td>${policy.description || '<em>No description</em>'}</td>
          <td>
            ${modelCount} model${modelCount !== 1 ? 's' : ''}
            ${hasWildcard ? '<span style="color: #1976d2;">(+ default)</span>' : ''}
          </td>
          <td style="font-size: 12px; color: #666;">
            ${new Date(policy.updated_at).toLocaleDateString()}
          </td>
          <td>
            <button class="btn-view" data-name="${policy.name}">View</button>
            <button class="btn-edit" data-name="${policy.name}">Edit</button>
            <button class="btn-delete" data-name="${policy.name}">Delete</button>
          </td>
        </tr>
      `;
    }).join('');
  }

  updateFormVisibility() {
    const addForm = this.shadowRoot.querySelector('#add-policy-form');
    const addButton = this.shadowRoot.querySelector('#add-policy-btn');

    if (addForm && addButton) {
      addForm.style.display = this.data.showAddForm ? 'block' : 'none';
      addButton.style.display = this.data.showAddForm ? 'none' : 'inline-block';
    }
  }

  setupEventListeners() {
    // Add policy button
    this.shadowRoot.querySelector('#add-policy-btn')?.addEventListener('click', () => {
      this.showAddPolicyForm();
    });

    // Refresh button
    this.shadowRoot.querySelector('#refresh-btn')?.addEventListener('click', () => {
      this.loadPolicies();
    });

    // Event delegation for table buttons
    this.shadowRoot.addEventListener('click', async (e) => {
      const policyName = e.target.getAttribute('data-name');

      if (e.target.classList.contains('btn-view')) {
        this.showViewDialog(policyName);
      } else if (e.target.classList.contains('btn-edit')) {
        this.showEditDialog(policyName);
      } else if (e.target.classList.contains('btn-delete')) {
        await this.deletePolicy(policyName);
      }
    });
  }

  showAddPolicyForm() {
    const dialog = document.createElement('div');
    dialog.className = 'dialog-overlay';
    dialog.innerHTML = `
      <div class="dialog">
        <h3>Create Rate Limit Policy</h3>

        <div class="form-group">
          <label>Policy Name <span style="color: red;">*</span></label>
          <input type="text" id="policy-name" placeholder="e.g., bronze, silver, gold" required />
          <small>Use lowercase with no spaces (e.g., "bronze", "premium_tier")</small>
        </div>

        <div class="form-group">
          <label>Description</label>
          <textarea id="policy-description" rows="2" placeholder="Brief description of this rate limit tier"></textarea>
        </div>

        <h4 style="margin: 24px 0 12px 0;">Model Limits</h4>
        <div id="model-limits-container">
          <div class="model-limit-row">
            <input type="text" placeholder="Model name or * for default" class="model-name" value="*" />
            <input type="number" placeholder="TPM" class="tpm" value="10000" min="0" />
            <input type="number" placeholder="RPM" class="rpm" value="10" min="0" />
            <input type="number" placeholder="Concurrent" class="concurrent" value="2" min="1" />
            <button class="btn-remove-model" style="display: none;">×</button>
          </div>
        </div>
        <button id="add-model-btn" class="btn-secondary" style="margin-top: 8px;">+ Add Model Limit</button>

        <small style="display: block; margin-top: 12px; color: #666;">
          <strong>*</strong> = Default limits for all models<br>
          Specific model names (e.g., "gpt-4", "claude-3") override the default
        </small>

        <div class="dialog-actions">
          <button class="btn-cancel">Cancel</button>
          <button class="btn-primary" id="submit-create-policy">Create Policy</button>
        </div>
      </div>
    `;

    this.shadowRoot.appendChild(dialog);

    // Add model limit row
    dialog.querySelector('#add-model-btn').addEventListener('click', () => {
      const container = dialog.querySelector('#model-limits-container');
      const row = document.createElement('div');
      row.className = 'model-limit-row';
      row.innerHTML = `
        <input type="text" placeholder="Model name" class="model-name" />
        <input type="number" placeholder="TPM" class="tpm" value="10000" min="0" />
        <input type="number" placeholder="RPM" class="rpm" value="10" min="0" />
        <input type="number" placeholder="Concurrent" class="concurrent" value="2" min="1" />
        <button class="btn-remove-model">×</button>
      `;
      container.appendChild(row);

      row.querySelector('.btn-remove-model').addEventListener('click', () => {
        row.remove();
      });
    });

    // Handle cancel
    dialog.querySelector('.btn-cancel').addEventListener('click', () => {
      dialog.remove();
    });

    // Handle submit
    dialog.querySelector('#submit-create-policy').addEventListener('click', async () => {
      const name = dialog.querySelector('#policy-name').value.trim();
      const description = dialog.querySelector('#policy-description').value.trim();

      if (!name) {
        this.showError('Policy name is required');
        return;
      }

      // Collect model limits
      const models = {};
      const rows = dialog.querySelectorAll('.model-limit-row');
      for (const row of rows) {
        const modelName = row.querySelector('.model-name').value.trim();
        const tpm = parseInt(row.querySelector('.tpm').value, 10);
        const rpm = parseInt(row.querySelector('.rpm').value, 10);
        const concurrent = parseInt(row.querySelector('.concurrent').value, 10);

        if (modelName && !isNaN(tpm) && !isNaN(rpm) && !isNaN(concurrent)) {
          models[modelName] = { tpm, rpm, concurrent };
        }
      }

      if (Object.keys(models).length === 0) {
        this.showError('At least one model limit configuration is required');
        return;
      }

      const btn = dialog.querySelector('#submit-create-policy');
      btn.disabled = true;
      btn.textContent = 'Creating...';

      try {
        await this.createPolicy(name, description, models);
        dialog.remove();
      } catch (error) {
        this.showError('Failed to create policy: ' + error.message);
        btn.disabled = false;
        btn.textContent = 'Create Policy';
      }
    });
  }

  showViewDialog(policyName) {
    const policy = this.data.policies.find(p => p.name === policyName);
    if (!policy) {
      this.showError('Policy not found');
      return;
    }

    const modelEntries = Object.entries(policy.models || {});

    const dialog = document.createElement('div');
    dialog.className = 'dialog-overlay';
    dialog.innerHTML = `
      <div class="dialog" style="max-width: 700px;">
        <h3>Policy: ${policy.name}</h3>
        <p style="color: #666; margin-bottom: 20px;">${policy.description || 'No description'}</p>

        <h4>Model Limits</h4>
        <table class="limits-table">
          <thead>
            <tr>
              <th>Model</th>
              <th>TPM</th>
              <th>RPM</th>
              <th>Concurrent</th>
            </tr>
          </thead>
          <tbody>
            ${modelEntries.map(([model, limits]) => `
              <tr>
                <td><code>${model === '*' ? '* (default)' : model}</code></td>
                <td>${limits.tpm.toLocaleString()}</td>
                <td>${limits.rpm}</td>
                <td>${limits.concurrent}</td>
              </tr>
            `).join('')}
          </tbody>
        </table>

        <div style="margin-top: 20px; padding: 12px; background: #f5f5f5; border-radius: 4px; font-size: 13px; color: #666;">
          <div><strong>Created:</strong> ${new Date(policy.created_at).toLocaleString()}</div>
          <div><strong>Updated:</strong> ${new Date(policy.updated_at).toLocaleString()}</div>
        </div>

        <div class="dialog-actions">
          <button class="btn-cancel">Close</button>
          <button class="btn-primary" data-edit="${policy.name}">Edit Policy</button>
        </div>
      </div>
    `;

    this.shadowRoot.appendChild(dialog);

    dialog.querySelector('.btn-cancel').addEventListener('click', () => {
      dialog.remove();
    });

    dialog.querySelector('[data-edit]').addEventListener('click', (e) => {
      dialog.remove();
      this.showEditDialog(e.target.getAttribute('data-edit'));
    });
  }

  showEditDialog(policyName) {
    const policy = this.data.policies.find(p => p.name === policyName);
    if (!policy) {
      this.showError('Policy not found');
      return;
    }

    const modelEntries = Object.entries(policy.models || {});

    const dialog = document.createElement('div');
    dialog.className = 'dialog-overlay';
    dialog.innerHTML = `
      <div class="dialog">
        <h3>Edit Policy: ${policy.name}</h3>
        <p style="color: #666; margin-bottom: 16px;">Policy name cannot be changed after creation.</p>

        <div class="form-group">
          <label>Description</label>
          <textarea id="policy-description" rows="2">${policy.description || ''}</textarea>
        </div>

        <h4 style="margin: 24px 0 12px 0;">Model Limits</h4>
        <div id="model-limits-container">
          ${modelEntries.map(([model, limits]) => `
            <div class="model-limit-row">
              <input type="text" placeholder="Model name" class="model-name" value="${model}" ${model === '*' ? 'readonly style="background: #f5f5f5;"' : ''} />
              <input type="number" placeholder="TPM" class="tpm" value="${limits.tpm}" min="0" />
              <input type="number" placeholder="RPM" class="rpm" value="${limits.rpm}" min="0" />
              <input type="number" placeholder="Concurrent" class="concurrent" value="${limits.concurrent}" min="1" />
              <button class="btn-remove-model" ${model === '*' ? 'style="display: none;"' : ''}>×</button>
            </div>
          `).join('')}
        </div>
        <button id="add-model-btn" class="btn-secondary" style="margin-top: 8px;">+ Add Model Limit</button>

        <div class="dialog-actions">
          <button class="btn-cancel">Cancel</button>
          <button class="btn-primary" id="submit-update-policy">Save Changes</button>
        </div>
      </div>
    `;

    this.shadowRoot.appendChild(dialog);

    // Setup remove buttons
    dialog.querySelectorAll('.btn-remove-model').forEach(btn => {
      if (btn.style.display !== 'none') {
        btn.addEventListener('click', (e) => {
          e.target.closest('.model-limit-row').remove();
        });
      }
    });

    // Add model limit row
    dialog.querySelector('#add-model-btn').addEventListener('click', () => {
      const container = dialog.querySelector('#model-limits-container');
      const row = document.createElement('div');
      row.className = 'model-limit-row';
      row.innerHTML = `
        <input type="text" placeholder="Model name" class="model-name" />
        <input type="number" placeholder="TPM" class="tpm" value="10000" min="0" />
        <input type="number" placeholder="RPM" class="rpm" value="10" min="0" />
        <input type="number" placeholder="Concurrent" class="concurrent" value="2" min="1" />
        <button class="btn-remove-model">×</button>
      `;
      container.appendChild(row);

      row.querySelector('.btn-remove-model').addEventListener('click', () => {
        row.remove();
      });
    });

    // Handle cancel
    dialog.querySelector('.btn-cancel').addEventListener('click', () => {
      dialog.remove();
    });

    // Handle submit
    dialog.querySelector('#submit-update-policy').addEventListener('click', async () => {
      const description = dialog.querySelector('#policy-description').value.trim();

      // Collect model limits
      const models = {};
      const rows = dialog.querySelectorAll('.model-limit-row');
      for (const row of rows) {
        const modelName = row.querySelector('.model-name').value.trim();
        const tpm = parseInt(row.querySelector('.tpm').value, 10);
        const rpm = parseInt(row.querySelector('.rpm').value, 10);
        const concurrent = parseInt(row.querySelector('.concurrent').value, 10);

        if (modelName && !isNaN(tpm) && !isNaN(rpm) && !isNaN(concurrent)) {
          models[modelName] = { tpm, rpm, concurrent };
        }
      }

      if (Object.keys(models).length === 0) {
        this.showError('At least one model limit configuration is required');
        return;
      }

      const btn = dialog.querySelector('#submit-update-policy');
      btn.disabled = true;
      btn.textContent = 'Saving...';

      try {
        await this.updatePolicy(policyName, description, models);
        dialog.remove();
      } catch (error) {
        this.showError('Failed to update policy: ' + error.message);
        btn.disabled = false;
        btn.textContent = 'Save Changes';
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

        .limits-table {
          font-size: 14px;
        }

        .limits-table td {
          padding: 10px;
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

        .btn-secondary {
          background: #f5f5f5;
          color: #333;
          border: 1px solid #ddd;
        }

        .btn-secondary:hover {
          background: #e0e0e0;
        }

        .btn-success {
          background: #4caf50;
          color: white;
        }

        .btn-success:hover {
          background: #45a049;
        }

        .btn-view {
          background: #1976d2;
          color: white;
          padding: 6px 12px;
          font-size: 13px;
        }

        .btn-view:hover {
          background: #1565c0;
        }

        .btn-edit {
          background: #ff9800;
          color: white;
          padding: 6px 12px;
          font-size: 13px;
        }

        .btn-edit:hover {
          background: #f57c00;
        }

        .btn-delete {
          background: #f44336;
          color: white;
          padding: 6px 12px;
          font-size: 13px;
        }

        .btn-delete:hover {
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

        .form-group input,
        .form-group textarea {
          width: 100%;
          padding: 10px;
          border: 1px solid #ddd;
          border-radius: 4px;
          font-size: 14px;
          font-family: inherit;
          box-sizing: border-box;
        }

        .form-group input:focus,
        .form-group textarea:focus {
          outline: none;
          border-color: #1976d2;
        }

        .form-group small {
          display: block;
          margin-top: 4px;
          color: #666;
          font-size: 12px;
        }

        .model-limit-row {
          display: grid;
          grid-template-columns: 2fr 1fr 1fr 1fr auto;
          gap: 8px;
          margin-bottom: 8px;
          align-items: center;
        }

        .model-limit-row input {
          padding: 8px;
          border: 1px solid #ddd;
          border-radius: 4px;
          font-size: 13px;
        }

        .btn-remove-model {
          padding: 4px 10px;
          background: #f44336;
          color: white;
          border: none;
          border-radius: 4px;
          cursor: pointer;
          font-size: 18px;
          line-height: 1;
        }

        .btn-remove-model:hover {
          background: #d32f2f;
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

      <div id="loading-spinner">Loading Rate Limit Policies...</div>
      <div id="message"></div>

      <div id="content" style="display: none;">
        <div class="header">
          <h2>Rate Limit Policies</h2>
          <div class="header-actions">
            <button id="refresh-btn" class="btn-primary">Refresh</button>
            <button id="add-policy-btn" class="btn-success">Add Policy</button>
          </div>
        </div>

        <div class="stats-grid">
          <div class="stat-card">
            <div class="stat-value" id="policy-count">0</div>
            <div class="stat-label">Total Policies</div>
          </div>
        </div>

        <div class="section">
          <div class="section-header">Configured Policies</div>
          <table id="policies-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Description</th>
                <th>Model Limits</th>
                <th>Last Updated</th>
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
customElements.define('rate-limit-policy-manager', RateLimitPolicyManager);
