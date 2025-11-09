// Custom Auth Token Management Web Component
class CustomAuthManager extends HTMLElement {
  constructor() {
    super();
    this.attachShadow({ mode: 'open' });
    this.data = {
      tokens: [],
      loading: true,
      editingToken: null,
      showAddForm: false
    };
  }

  connectedCallback() {
    console.log('CustomAuthManager component initialized:', {
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
        console.log('Plugin API found, loading tokens...');
        this.loadTokens();
        return;
      }
      await new Promise(resolve => setTimeout(resolve, 100));
    }

    // Timeout - show error
    console.error('Plugin API injection timeout');
    this.showError('Plugin API initialization timeout - please refresh the page');
  }

  async loadTokens() {
    this.setLoading(true);
    try {
      console.log('Loading tokens via RPC...');
      const result = await this.pluginAPI.call('listTokens', {});
      console.log('Tokens loaded:', result);

      this.data.tokens = result.tokens || [];
      this.updateTokensTable();

      // Update stats
      const statsDiv = this.shadowRoot.querySelector('#token-count');
      if (statsDiv) {
        statsDiv.textContent = this.data.tokens.length;
      }
    } catch (error) {
      console.error('Failed to load tokens:', error);
      this.showError('Failed to load tokens: ' + error.message);
    } finally {
      this.setLoading(false);
    }
  }

  async addToken(token, appId, userId, description) {
    try {
      const result = await this.pluginAPI.call('addToken', {
        token: token,
        app_id: parseInt(appId, 10),
        user_id: userId || '',
        description: description || ''
      });

      console.log('Token added:', result);
      this.showSuccess('Token added successfully');

      // Reload tokens
      await this.loadTokens();

      // Hide form
      this.data.showAddForm = false;
      this.updateFormVisibility();

      return result;
    } catch (error) {
      console.error('Failed to add token:', error);
      throw error;
    }
  }

  async updateToken(id, appId, userId, description) {
    try {
      const result = await this.pluginAPI.call('updateToken', {
        id: id,
        app_id: parseInt(appId, 10),
        user_id: userId || '',
        description: description || ''
      });

      console.log('Token updated:', result);
      this.showSuccess('Token updated successfully');

      // Reload tokens
      await this.loadTokens();

      // Clear editing state
      this.data.editingToken = null;
      this.updateTokensTable();

      return result;
    } catch (error) {
      console.error('Failed to update token:', error);
      throw error;
    }
  }

  async deleteToken(id) {
    if (!confirm('Are you sure you want to delete this token? This action cannot be undone.')) {
      return;
    }

    try {
      const result = await this.pluginAPI.call('deleteToken', { id: id });

      console.log('Token deleted:', result);
      this.showSuccess('Token deleted successfully');

      // Reload tokens
      await this.loadTokens();

      return result;
    } catch (error) {
      console.error('Failed to delete token:', error);
      this.showError('Failed to delete token: ' + error.message);
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

  updateTokensTable() {
    const tableBody = this.shadowRoot.querySelector('#tokens-table tbody');
    if (!tableBody) return;

    if (this.data.tokens.length === 0) {
      tableBody.innerHTML = `
        <tr>
          <td colspan="5" style="text-align: center; color: #999; padding: 40px;">
            No tokens configured. Click "Add Token" to create your first token.
          </td>
        </tr>
      `;
      return;
    }

    tableBody.innerHTML = this.data.tokens.map(token => `
      <tr>
        <td><code>${token.id}</code></td>
        <td><code>${token.token_mask}</code></td>
        <td>${token.app_id}</td>
        <td>${token.user_id || '<em>none</em>'}</td>
        <td>${token.description || '<em>none</em>'}</td>
        <td>
          <button class="btn-edit" data-id="${token.id}">Edit</button>
          <button class="btn-delete" data-id="${token.id}">Delete</button>
        </td>
      </tr>
    `).join('');
  }

  updateFormVisibility() {
    const addForm = this.shadowRoot.querySelector('#add-token-form');
    const addButton = this.shadowRoot.querySelector('#add-token-btn');

    if (addForm && addButton) {
      addForm.style.display = this.data.showAddForm ? 'block' : 'none';
      addButton.style.display = this.data.showAddForm ? 'none' : 'inline-block';
    }
  }

  setupEventListeners() {
    // Add token button
    this.shadowRoot.querySelector('#add-token-btn')?.addEventListener('click', () => {
      this.data.showAddForm = true;
      this.updateFormVisibility();

      // Clear form
      const form = this.shadowRoot.querySelector('#add-token-form');
      if (form) {
        form.querySelector('[name="token"]').value = '';
        form.querySelector('[name="app_id"]').value = '';
        form.querySelector('[name="user_id"]').value = '';
        form.querySelector('[name="description"]').value = '';
      }
    });

    // Cancel add token
    this.shadowRoot.querySelector('#cancel-add-btn')?.addEventListener('click', () => {
      this.data.showAddForm = false;
      this.updateFormVisibility();
    });

    // Submit add token form
    this.shadowRoot.querySelector('#submit-add-btn')?.addEventListener('click', async () => {
      const form = this.shadowRoot.querySelector('#add-token-form');
      const token = form.querySelector('[name="token"]').value.trim();
      const appId = form.querySelector('[name="app_id"]').value.trim();
      const userId = form.querySelector('[name="user_id"]').value.trim();
      const description = form.querySelector('[name="description"]').value.trim();

      // Validate
      if (!token) {
        this.showError('Token is required');
        return;
      }
      if (!appId || parseInt(appId, 10) <= 0) {
        this.showError('Valid App ID is required (must be > 0)');
        return;
      }

      // Disable button during request
      const btn = this.shadowRoot.querySelector('#submit-add-btn');
      btn.disabled = true;
      btn.textContent = 'Adding...';

      try {
        await this.addToken(token, appId, userId, description);
      } catch (error) {
        this.showError('Failed to add token: ' + error.message);
      } finally {
        btn.disabled = false;
        btn.textContent = 'Add Token';
      }
    });

    // Refresh button
    this.shadowRoot.querySelector('#refresh-btn')?.addEventListener('click', () => {
      this.loadTokens();
    });

    // Edit and delete buttons (event delegation)
    this.shadowRoot.addEventListener('click', async (e) => {
      if (e.target.classList.contains('btn-edit')) {
        const tokenId = e.target.getAttribute('data-id');
        this.showEditDialog(tokenId);
      } else if (e.target.classList.contains('btn-delete')) {
        const tokenId = e.target.getAttribute('data-id');
        await this.deleteToken(tokenId);
      }
    });
  }

  showEditDialog(tokenId) {
    const token = this.data.tokens.find(t => t.id === tokenId);
    if (!token) {
      this.showError('Token not found');
      return;
    }

    // Create edit dialog
    const dialog = document.createElement('div');
    dialog.className = 'edit-dialog-overlay';
    dialog.innerHTML = `
      <div class="edit-dialog">
        <h3>Edit Token: ${token.id}</h3>
        <p style="color: #666; margin-bottom: 16px;">Token value cannot be changed after creation.</p>

        <div class="form-group">
          <label>Token (read-only)</label>
          <input type="text" value="${token.token_mask}" readonly style="background: #f5f5f5; cursor: not-allowed;" />
        </div>

        <div class="form-group">
          <label>App ID <span style="color: red;">*</span></label>
          <input type="number" name="app_id" value="${token.app_id}" min="1" required />
        </div>

        <div class="form-group">
          <label>User ID</label>
          <input type="text" name="user_id" value="${token.user_id || ''}" />
        </div>

        <div class="form-group">
          <label>Description</label>
          <textarea name="description" rows="3">${token.description || ''}</textarea>
        </div>

        <div class="dialog-actions">
          <button class="btn-cancel">Cancel</button>
          <button class="btn-primary">Save Changes</button>
        </div>
      </div>
    `;

    this.shadowRoot.appendChild(dialog);

    // Handle cancel
    dialog.querySelector('.btn-cancel').addEventListener('click', () => {
      dialog.remove();
    });

    // Handle save
    dialog.querySelector('.btn-primary').addEventListener('click', async () => {
      const appId = dialog.querySelector('[name="app_id"]').value.trim();
      const userId = dialog.querySelector('[name="user_id"]').value.trim();
      const description = dialog.querySelector('[name="description"]').value.trim();

      if (!appId || parseInt(appId, 10) <= 0) {
        this.showError('Valid App ID is required (must be > 0)');
        return;
      }

      const btn = dialog.querySelector('.btn-primary');
      btn.disabled = true;
      btn.textContent = 'Saving...';

      try {
        await this.updateToken(tokenId, appId, userId, description);
        dialog.remove();
      } catch (error) {
        this.showError('Failed to update token: ' + error.message);
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

    // Hide after 5 seconds
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
        }

        .btn-primary {
          background: #1976d2;
          color: white;
        }

        .btn-primary:hover {
          background: #1565c0;
        }

        .btn-success {
          background: #4caf50;
          color: white;
        }

        .btn-success:hover {
          background: #45a049;
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

        #add-token-form {
          background: #f9f9f9;
          padding: 24px;
          border-radius: 8px;
          margin: 20px;
          display: none;
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

        .form-actions {
          display: flex;
          gap: 12px;
          margin-top: 20px;
        }

        .edit-dialog-overlay {
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

        .edit-dialog {
          background: white;
          border-radius: 8px;
          padding: 24px;
          width: 500px;
          max-width: 90vw;
          max-height: 90vh;
          overflow-y: auto;
        }

        .edit-dialog h3 {
          margin: 0 0 16px 0;
          color: #333;
        }

        .dialog-actions {
          display: flex;
          gap: 12px;
          justify-content: flex-end;
          margin-top: 24px;
        }
      </style>

      <div id="loading-spinner">Loading Token Management...</div>
      <div id="message"></div>

      <div id="content" style="display: none;">
        <div class="header">
          <h2>Custom Auth Token Management</h2>
          <div class="header-actions">
            <button id="refresh-btn" class="btn-primary">Refresh</button>
            <button id="add-token-btn" class="btn-success">Add Token</button>
          </div>
        </div>

        <div class="stats-grid">
          <div class="stat-card">
            <div class="stat-value" id="token-count">0</div>
            <div class="stat-label">Total Tokens</div>
          </div>
        </div>

        <div id="add-token-form">
          <h3 style="margin-top: 0; color: #333;">Add New Token</h3>

          <div class="form-group">
            <label>Token Value <span style="color: red;">*</span></label>
            <input type="text" name="token" placeholder="Enter authentication token" required />
            <small style="color: #666; display: block; margin-top: 4px;">
              The token that will be used for authentication (without 'Bearer ' prefix)
            </small>
          </div>

          <div class="form-group">
            <label>App ID <span style="color: red;">*</span></label>
            <input type="number" name="app_id" placeholder="1" min="1" required />
            <small style="color: #666; display: block; margin-top: 4px;">
              Numeric App ID from the database that this token authenticates
            </small>
          </div>

          <div class="form-group">
            <label>User ID</label>
            <input type="text" name="user_id" placeholder="Optional user identifier" />
          </div>

          <div class="form-group">
            <label>Description</label>
            <textarea name="description" rows="3" placeholder="Optional description (e.g., 'Production API token')"></textarea>
          </div>

          <div class="form-actions">
            <button id="cancel-add-btn" class="btn-cancel">Cancel</button>
            <button id="submit-add-btn" class="btn-success">Add Token</button>
          </div>
        </div>

        <div class="section">
          <div class="section-header">Configured Tokens</div>
          <table id="tokens-table">
            <thead>
              <tr>
                <th>ID</th>
                <th>Token</th>
                <th>App ID</th>
                <th>User ID</th>
                <th>Description</th>
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
customElements.define('custom-auth-manager', CustomAuthManager);
