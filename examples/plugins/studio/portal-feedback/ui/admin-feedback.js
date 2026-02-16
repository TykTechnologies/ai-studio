/**
 * Admin Feedback View - WebComponent for admins to view all user feedback
 *
 * This component is rendered in the Admin UI and uses pluginAPI.call()
 * to fetch feedback via the admin-scoped RPC endpoint.
 *
 * Note: Feedback is stored in-memory in the plugin process. It will be
 * lost when the plugin is restarted. For production use, store data
 * via ctx.Services.KV() in the Go plugin code.
 */
class FeedbackAdminView extends HTMLElement {
  constructor() {
    super();
    this.attachShadow({ mode: 'open' });
  }

  connectedCallback() {
    this.render();
    this.waitForAPIAndLoad();
  }

  /**
   * Wait for pluginAPI to be injected by the React wrapper, then load data.
   * The wrapper injects pluginAPI after the element is mounted in the DOM.
   */
  waitForAPIAndLoad(attempts = 0) {
    if (this.pluginAPI) {
      this.loadFeedback();
      return;
    }
    if (attempts < 20) {
      setTimeout(() => this.waitForAPIAndLoad(attempts + 1), 100);
    } else {
      this.shadowRoot.getElementById('table-container').innerHTML =
        '<div class="empty" style="color: #c62828;">Plugin API not available. Try refreshing the page.</div>';
    }
  }

  render() {
    this.shadowRoot.innerHTML = `
      <style>
        :host { display: block; padding: 24px; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; }
        h2 { margin-top: 0; color: #1a1a1a; }
        .stats { display: flex; gap: 16px; margin-bottom: 24px; }
        .stat-card { padding: 16px; background: #f5f5f5; border-radius: 8px; flex: 1; }
        .stat-card .value { font-size: 24px; font-weight: 700; color: #1976d2; }
        .stat-card .label { font-size: 12px; color: #888; margin-top: 4px; }
        table { width: 100%; border-collapse: collapse; }
        th, td { text-align: left; padding: 12px; border-bottom: 1px solid #eee; }
        th { font-weight: 600; color: #666; font-size: 12px; text-transform: uppercase; }
        .delete-btn { background: none; border: 1px solid #f44336; color: #f44336; padding: 4px 12px; border-radius: 4px; cursor: pointer; font-size: 12px; }
        .delete-btn:hover { background: #ffebee; }
        .empty { text-align: center; padding: 40px; color: #888; }
        .refresh-btn { background: #1976d2; color: white; border: none; padding: 8px 16px; border-radius: 6px; cursor: pointer; font-size: 13px; margin-bottom: 16px; }
      </style>
      <h2>User Feedback</h2>
      <button class="refresh-btn" id="refresh">Refresh</button>
      <div class="stats" id="stats"></div>
      <div id="table-container">Loading...</div>
    `;

    this.shadowRoot.getElementById('refresh').addEventListener('click', () => this.loadFeedback());
  }

  async loadFeedback() {
    try {
      const result = await this.pluginAPI.call('list_feedback', {});
      const items = result.feedback || [];

      const avgRating = items.length > 0
        ? (items.reduce((sum, fb) => sum + fb.rating, 0) / items.length).toFixed(1)
        : '0';
      this.shadowRoot.getElementById('stats').innerHTML = `
        <div class="stat-card">
          <div class="value">${items.length}</div>
          <div class="label">Total Submissions</div>
        </div>
        <div class="stat-card">
          <div class="value">${avgRating}</div>
          <div class="label">Average Rating</div>
        </div>
      `;

      const container = this.shadowRoot.getElementById('table-container');
      if (items.length === 0) {
        container.innerHTML = '<div class="empty">No feedback submissions yet.</div>';
        return;
      }

      container.innerHTML = `
        <table>
          <thead>
            <tr>
              <th>User</th>
              <th>Title</th>
              <th>Message</th>
              <th>Rating</th>
              <th>Date</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            ${items.map(fb => `
              <tr>
                <td>${this.escapeHtml(fb.user_name || fb.user_email)}</td>
                <td>${this.escapeHtml(fb.title)}</td>
                <td>${this.escapeHtml(fb.message).substring(0, 100)}${fb.message.length > 100 ? '...' : ''}</td>
                <td>${'★'.repeat(fb.rating)}</td>
                <td>${new Date(fb.created_at).toLocaleDateString()}</td>
                <td><button class="delete-btn" data-id="${fb.id}">Delete</button></td>
              </tr>
            `).join('')}
          </tbody>
        </table>
      `;

      container.querySelectorAll('.delete-btn').forEach(btn => {
        btn.addEventListener('click', () => this.deleteFeedback(btn.dataset.id));
      });
    } catch (err) {
      console.error('Failed to load feedback:', err);
      this.shadowRoot.getElementById('table-container').innerHTML =
        '<div class="empty" style="color: #c62828;">Failed to load feedback.</div>';
    }
  }

  async deleteFeedback(id) {
    try {
      await this.pluginAPI.call('delete_feedback', { id });
      this.loadFeedback();
    } catch (err) {
      console.error('Failed to delete feedback:', err);
    }
  }

  escapeHtml(str) {
    const div = document.createElement('div');
    div.textContent = str || '';
    return div.innerHTML;
  }
}

customElements.define('feedback-admin-view', FeedbackAdminView);
