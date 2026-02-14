// Echo Endpoint Config Editor Web Component
class EchoConfigEditor extends HTMLElement {
  constructor() {
    super();
    this.attachShadow({ mode: 'open' });
    this.data = {
      slug: '',
      content: '',
      loading: true,
      saving: false,
      message: null,
      messageType: null
    };
  }

  connectedCallback() {
    console.log('EchoConfigEditor component initialized');
    this.render();
    this.waitForPluginAPI();
  }

  async waitForPluginAPI() {
    for (let i = 0; i < 50; i++) {
      if (this.pluginAPI) {
        console.log('Plugin API found, loading config...');
        this.loadConfig();
        return;
      }
      await new Promise(resolve => setTimeout(resolve, 100));
    }
    console.error('Plugin API injection timeout');
    this.showMessage('Plugin API initialization timeout — please refresh the page', 'error');
    this.data.loading = false;
    this.updateUI();
  }

  async loadConfig() {
    this.data.loading = true;
    this.updateUI();
    try {
      const result = await this.pluginAPI.call('get_config', {});
      console.log('Config loaded:', result);
      this.data.slug = result.slug || '';
      this.data.content = result.content || '';

      const slugInput = this.shadowRoot.querySelector('#slug-input');
      const contentInput = this.shadowRoot.querySelector('#content-input');
      if (slugInput) slugInput.value = this.data.slug;
      if (contentInput) contentInput.value = this.data.content;

      this.updateEndpointHint();
    } catch (error) {
      console.error('Failed to load config:', error);
      this.showMessage('Failed to load config: ' + error.message, 'error');
    } finally {
      this.data.loading = false;
      this.updateUI();
    }
  }

  async saveConfig() {
    const slugInput = this.shadowRoot.querySelector('#slug-input');
    const contentInput = this.shadowRoot.querySelector('#content-input');
    if (!slugInput || !contentInput) return;

    const slug = slugInput.value.trim();
    const content = contentInput.value;

    if (!slug) {
      this.showMessage('Slug is required — it determines the endpoint URL path.', 'error');
      return;
    }

    this.data.saving = true;
    this.updateUI();

    try {
      const result = await this.pluginAPI.call('save_config', { slug, content });
      console.log('Config saved:', result);
      this.data.slug = slug;
      this.data.content = content;
      this.updateEndpointHint();
      this.showMessage(result.message || 'Configuration saved successfully', 'success');
    } catch (error) {
      console.error('Failed to save config:', error);
      this.showMessage('Failed to save: ' + error.message, 'error');
    } finally {
      this.data.saving = false;
      this.updateUI();
    }
  }

  updateEndpointHint() {
    const hintEl = this.shadowRoot.querySelector('#endpoint-url');
    if (hintEl) {
      const slug = this.data.slug || 'custom-echo-endpoint';
      hintEl.textContent = `curl http://<gateway-host>:8081/plugins/${slug}/hello?foo=bar`;
    }
  }

  showMessage(text, type) {
    this.data.message = text;
    this.data.messageType = type;
    this.updateUI();

    if (type === 'success') {
      setTimeout(() => {
        if (this.data.message === text) {
          this.data.message = null;
          this.updateUI();
        }
      }, 5000);
    }
  }

  updateUI() {
    const loadingEl = this.shadowRoot.querySelector('#loading');
    const mainEl = this.shadowRoot.querySelector('#main');
    const saveBtn = this.shadowRoot.querySelector('#save-btn');
    const messageEl = this.shadowRoot.querySelector('#message');

    if (loadingEl) {
      loadingEl.style.display = this.data.loading ? 'block' : 'none';
    }
    if (mainEl) {
      mainEl.style.display = this.data.loading ? 'none' : 'block';
    }
    if (saveBtn) {
      saveBtn.disabled = this.data.saving;
      saveBtn.textContent = this.data.saving ? 'Saving...' : 'Save Configuration';
    }
    if (messageEl) {
      if (this.data.message) {
        messageEl.style.display = 'block';
        messageEl.textContent = this.data.message;
        messageEl.className = 'message ' + (this.data.messageType || 'info');
      } else {
        messageEl.style.display = 'none';
      }
    }
  }

  render() {
    this.shadowRoot.innerHTML = `
      <style>
        :host {
          display: block;
          font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, sans-serif;
          color: #1a1a2e;
          padding: 24px;
        }

        .card {
          background: #fff;
          border: 1px solid #e2e8f0;
          border-radius: 8px;
          padding: 24px;
          max-width: 720px;
        }

        h2 {
          margin: 0 0 8px 0;
          font-size: 20px;
          font-weight: 600;
          color: #1a1a2e;
        }

        .description {
          color: #64748b;
          font-size: 14px;
          margin: 0 0 20px 0;
          line-height: 1.5;
        }

        .field {
          margin-bottom: 16px;
        }

        label {
          display: block;
          font-size: 13px;
          font-weight: 500;
          color: #475569;
          margin-bottom: 6px;
        }

        input[type="text"] {
          width: 100%;
          padding: 8px 12px;
          border: 1px solid #e2e8f0;
          border-radius: 6px;
          font-family: inherit;
          font-size: 14px;
          color: #1a1a2e;
          box-sizing: border-box;
          transition: border-color 0.15s;
        }
        input[type="text"]:focus {
          outline: none;
          border-color: #6366f1;
          box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.1);
        }

        textarea {
          width: 100%;
          min-height: 120px;
          padding: 10px 12px;
          border: 1px solid #e2e8f0;
          border-radius: 6px;
          font-family: inherit;
          font-size: 14px;
          color: #1a1a2e;
          resize: vertical;
          box-sizing: border-box;
          transition: border-color 0.15s;
        }
        textarea:focus {
          outline: none;
          border-color: #6366f1;
          box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.1);
        }

        .actions {
          margin-top: 16px;
          display: flex;
          align-items: center;
          gap: 12px;
        }

        button {
          padding: 8px 20px;
          border: none;
          border-radius: 6px;
          font-size: 14px;
          font-weight: 500;
          cursor: pointer;
          transition: background-color 0.15s, opacity 0.15s;
        }
        button:disabled {
          opacity: 0.6;
          cursor: not-allowed;
        }

        .btn-primary {
          background: #6366f1;
          color: #fff;
        }
        .btn-primary:hover:not(:disabled) {
          background: #4f46e5;
        }

        .message {
          margin-top: 16px;
          padding: 10px 14px;
          border-radius: 6px;
          font-size: 13px;
          line-height: 1.4;
        }
        .message.success {
          background: #ecfdf5;
          color: #065f46;
          border: 1px solid #a7f3d0;
        }
        .message.error {
          background: #fef2f2;
          color: #991b1b;
          border: 1px solid #fecaca;
        }

        .loading {
          color: #64748b;
          font-size: 14px;
          padding: 20px 0;
        }

        .hint {
          color: #94a3b8;
          font-size: 12px;
          margin-top: 6px;
        }

        .endpoint-info {
          background: #f8fafc;
          border: 1px solid #e2e8f0;
          border-radius: 6px;
          padding: 14px;
          margin-top: 20px;
          font-size: 13px;
          color: #475569;
        }
        .endpoint-info code {
          background: #e2e8f0;
          padding: 2px 6px;
          border-radius: 3px;
          font-size: 12px;
          color: #1e293b;
          word-break: break-all;
        }
      </style>

      <div class="card">
        <h2>Echo Endpoint Configuration</h2>
        <p class="description">
          Configure the URL slug and custom content for the echo endpoint.
          The endpoint is served by the gateway at <code>/plugins/{slug}/</code>.
        </p>

        <div id="loading" class="loading">Loading current configuration...</div>

        <div id="main" style="display:none">
          <div class="field">
            <label for="slug-input">URL Slug</label>
            <input type="text" id="slug-input" placeholder="custom-echo-endpoint" />
            <p class="hint">Determines the endpoint URL path: <code>/plugins/{slug}/...</code></p>
          </div>

          <div class="field">
            <label for="content-input">Custom Content</label>
            <textarea id="content-input" placeholder="Enter custom content to include in echo responses..."></textarea>
            <p class="hint">This text appears in the <code>custom_content</code> field of every echo response.</p>
          </div>

          <div class="actions">
            <button id="save-btn" class="btn-primary" onclick="this.getRootNode().host.saveConfig()">
              Save Configuration
            </button>
          </div>

          <div id="message" class="message" style="display:none"></div>

          <div class="endpoint-info">
            <strong>Test the endpoint:</strong><br>
            <code id="endpoint-url">curl http://&lt;gateway-host&gt;:8081/plugins/custom-echo-endpoint/hello?foo=bar</code>
          </div>
        </div>
      </div>
    `;
  }
}

customElements.define('echo-config-editor', EchoConfigEditor);
