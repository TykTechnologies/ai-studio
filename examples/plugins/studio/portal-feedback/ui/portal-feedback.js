/**
 * Portal Feedback Form - WebComponent for end-user feedback submission
 *
 * This component is rendered in the AI Portal and uses portalPluginAPI.call()
 * to submit feedback via the portal-scoped RPC endpoint.
 *
 * Note: Feedback is stored in-memory in the plugin process. It will be
 * lost when the plugin is restarted. For production use, store data
 * via ctx.Services.KV() in the Go plugin code.
 */
class FeedbackPortalForm extends HTMLElement {
  constructor() {
    super();
    this.attachShadow({ mode: 'open' });
  }

  connectedCallback() {
    this.render();
    this.setupEventListeners();
    this.waitForAPIAndLoad();
  }

  /**
   * Wait for portalPluginAPI to be injected by the React wrapper, then load data.
   */
  waitForAPIAndLoad(attempts = 0) {
    if (this.portalPluginAPI) {
      this.loadMyFeedback();
      return;
    }
    if (attempts < 20) {
      setTimeout(() => this.waitForAPIAndLoad(attempts + 1), 100);
    } else {
      this.shadowRoot.getElementById('feedback-items').innerHTML =
        '<p style="color: #c62828;">Plugin API not available. Try refreshing the page.</p>';
    }
  }

  render() {
    this.shadowRoot.innerHTML = `
      <style>
        :host { display: block; padding: 24px; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; }
        h2 { margin-top: 0; color: #1a1a1a; }
        .form-group { margin-bottom: 16px; }
        label { display: block; margin-bottom: 4px; font-weight: 500; color: #333; }
        input, textarea { width: 100%; padding: 8px 12px; border: 1px solid #ddd; border-radius: 6px; font-size: 14px; box-sizing: border-box; }
        textarea { resize: vertical; min-height: 100px; }
        .rating { display: flex; gap: 8px; }
        .rating button { width: 36px; height: 36px; border: 1px solid #ddd; border-radius: 50%; cursor: pointer; font-size: 16px; background: #fff; }
        .rating button.selected { background: #1976d2; color: white; border-color: #1976d2; }
        .submit-btn { background: #1976d2; color: white; border: none; padding: 10px 24px; border-radius: 6px; cursor: pointer; font-size: 14px; }
        .submit-btn:hover { background: #1565c0; }
        .submit-btn:disabled { background: #ccc; cursor: not-allowed; }
        .message { padding: 12px; border-radius: 6px; margin-bottom: 16px; }
        .message.success { background: #e8f5e9; color: #2e7d32; }
        .message.error { background: #ffebee; color: #c62828; }
        .feedback-list { margin-top: 32px; }
        .feedback-item { padding: 12px; border: 1px solid #eee; border-radius: 8px; margin-bottom: 8px; }
        .feedback-item .title { font-weight: 600; }
        .feedback-item .meta { font-size: 12px; color: #888; margin-top: 4px; }
      </style>
      <h2>Send Feedback</h2>
      <div id="message"></div>
      <div class="form-group">
        <label for="title">Title</label>
        <input type="text" id="title" placeholder="Brief summary of your feedback" />
      </div>
      <div class="form-group">
        <label>Rating</label>
        <div class="rating" id="rating">
          ${[1,2,3,4,5].map(n => `<button data-rating="${n}">${n}</button>`).join('')}
        </div>
      </div>
      <div class="form-group">
        <label for="message-input">Message</label>
        <textarea id="message-input" placeholder="Tell us more about your experience..."></textarea>
      </div>
      <button class="submit-btn" id="submit">Submit Feedback</button>
      <div class="feedback-list" id="my-feedback">
        <h3>My Previous Feedback</h3>
        <div id="feedback-items">Loading...</div>
      </div>
    `;
  }

  setupEventListeners() {
    this.selectedRating = 0;

    this.shadowRoot.querySelectorAll('.rating button').forEach(btn => {
      btn.addEventListener('click', () => {
        this.selectedRating = parseInt(btn.dataset.rating);
        this.shadowRoot.querySelectorAll('.rating button').forEach(b => b.classList.remove('selected'));
        btn.classList.add('selected');
      });
    });

    this.shadowRoot.getElementById('submit').addEventListener('click', () => this.handleSubmit());
  }

  async handleSubmit() {
    if (!this.portalPluginAPI) {
      this.showMessage('Plugin API not ready. Please wait a moment and try again.', 'error');
      return;
    }

    const title = this.shadowRoot.getElementById('title').value.trim();
    const message = this.shadowRoot.getElementById('message-input').value.trim();
    const rating = this.selectedRating;

    if (!title || !message || !rating) {
      this.showMessage('Please fill in all fields and select a rating.', 'error');
      return;
    }

    const submitBtn = this.shadowRoot.getElementById('submit');
    submitBtn.disabled = true;

    try {
      const result = await this.portalPluginAPI.call('submit_feedback', { title, message, rating });
      if (result.success) {
        this.showMessage('Thank you for your feedback!', 'success');
        this.shadowRoot.getElementById('title').value = '';
        this.shadowRoot.getElementById('message-input').value = '';
        this.selectedRating = 0;
        this.shadowRoot.querySelectorAll('.rating button').forEach(b => b.classList.remove('selected'));
        this.loadMyFeedback();
      } else {
        this.showMessage(result.error || 'Failed to submit feedback', 'error');
      }
    } catch (err) {
      this.showMessage('An error occurred. Please try again.', 'error');
    } finally {
      submitBtn.disabled = false;
    }
  }

  async loadMyFeedback() {
    try {
      const result = await this.portalPluginAPI.call('my_feedback', {});
      const items = result.feedback || [];
      const container = this.shadowRoot.getElementById('feedback-items');
      if (items.length === 0) {
        container.innerHTML = '<p style="color: #888;">No feedback submitted yet.</p>';
        return;
      }
      container.innerHTML = items.map(fb => `
        <div class="feedback-item">
          <div class="title">${this.escapeHtml(fb.title)} ${'★'.repeat(fb.rating)}</div>
          <div>${this.escapeHtml(fb.message)}</div>
          <div class="meta">${new Date(fb.created_at).toLocaleString()}</div>
        </div>
      `).join('');
    } catch (err) {
      console.error('Failed to load feedback:', err);
      this.shadowRoot.getElementById('feedback-items').innerHTML =
        '<p style="color: #c62828;">Failed to load feedback.</p>';
    }
  }

  showMessage(text, type) {
    const el = this.shadowRoot.getElementById('message');
    el.className = `message ${type}`;
    el.textContent = text;
    setTimeout(() => { el.textContent = ''; el.className = ''; }, 5000);
  }

  escapeHtml(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
  }
}

customElements.define('feedback-portal-form', FeedbackPortalForm);
