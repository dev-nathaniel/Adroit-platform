package userservice

import (
	"html/template"
	"net/http"
)

var inviteHTMLTemplate = template.Must(template.New("invite_page").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{ if .Success }}Invitation Accepted{{ else if .Error }}Invitation Error{{ else }}Join {{ .Preview.OrganizationName }} on Adroit{{ end }}</title>
  <style>
    *, *::before, *::after { box-sizing: border-box; }
    body {
      margin: 0;
      padding: 0;
      min-height: 100vh;
      background-color: #0b0f17;
      color: #f1f5f9;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      -webkit-font-smoothing: antialiased;
    }
    .container {
      width: 100%;
      max-width: 480px;
      padding: 24px 16px;
    }
    .brand-header {
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 10px;
      margin-bottom: 24px;
    }
    .brand-logo {
      width: 32px;
      height: 32px;
      background: linear-gradient(135deg, #0284c7 0%, #2563eb 100%);
      border-radius: 8px;
      display: flex;
      align-items: center;
      justify-content: center;
      color: #ffffff;
      font-weight: 800;
      font-size: 16px;
      box-shadow: 0 2px 8px rgba(2, 132, 199, 0.35);
    }
    .brand-name {
      font-size: 19px;
      font-weight: 700;
      letter-spacing: -0.02em;
      color: #ffffff;
    }
    .card {
      background: #111827;
      border: 1px solid #1f2937;
      border-radius: 12px;
      padding: 32px 28px;
      box-shadow: 0 10px 30px -10px rgba(0, 0, 0, 0.5), 0 0 0 1px rgba(255, 255, 255, 0.05);
    }
    h1 {
      margin: 0 0 8px 0;
      font-size: 21px;
      font-weight: 600;
      letter-spacing: -0.02em;
      color: #ffffff;
      text-align: center;
    }
    .subtitle {
      margin: 0 0 20px 0;
      font-size: 14px;
      line-height: 1.5;
      color: #94a3b8;
      text-align: center;
    }
    .details-table {
      width: 100%;
      background: #0f172a;
      border: 1px solid #1e293b;
      border-radius: 8px;
      padding: 14px 16px;
      margin-bottom: 20px;
    }
    .details-row {
      display: flex;
      justify-content: space-between;
      align-items: center;
      padding: 7px 0;
      font-size: 13px;
      border-bottom: 1px solid #1e293b;
    }
    .details-row:last-child {
      border-bottom: none;
      padding-bottom: 0;
    }
    .details-row:first-child {
      padding-top: 0;
    }
    .details-label {
      color: #64748b;
      font-weight: 500;
    }
    .details-value {
      color: #f8fafc;
      font-weight: 600;
      text-align: right;
    }
    .badge {
      display: inline-block;
      padding: 2px 8px;
      border-radius: 9999px;
      font-size: 11px;
      font-weight: 600;
      text-transform: lowercase;
      background: rgba(2, 132, 199, 0.15);
      color: #38bdf8;
      border: 1px solid rgba(2, 132, 199, 0.3);
    }
    .verified-badge {
      display: inline-flex;
      align-items: center;
      gap: 4px;
      color: #34d399;
      font-size: 12px;
      font-weight: 600;
      background: rgba(16, 185, 129, 0.1);
      padding: 2px 8px;
      border-radius: 4px;
      border: 1px solid rgba(16, 185, 129, 0.2);
    }
    .btn {
      display: block;
      width: 100%;
      padding: 12px 18px;
      background: #0284c7;
      color: #ffffff;
      border: none;
      border-radius: 7px;
      font-size: 14px;
      font-weight: 600;
      cursor: pointer;
      text-align: center;
      text-decoration: none;
      transition: all 0.15s ease;
      box-shadow: 0 2px 8px rgba(2, 132, 199, 0.35);
    }
    .btn:hover {
      background: #0369a1;
      transform: translateY(-1px);
    }
    .btn:disabled {
      opacity: 0.6;
      cursor: not-allowed;
      transform: none;
    }
    .btn-secondary {
      background: #1e293b;
      color: #cbd5e1;
      box-shadow: none;
      border: 1px solid #334155;
      margin-top: 10px;
    }
    .btn-secondary:hover {
      background: #334155;
      color: #ffffff;
    }
    .tabs {
      display: flex;
      background: #0f172a;
      border: 1px solid #1e293b;
      border-radius: 8px;
      padding: 3px;
      margin-bottom: 20px;
    }
    .tab-btn {
      flex: 1;
      padding: 8px 12px;
      background: transparent;
      border: none;
      border-radius: 6px;
      color: #94a3b8;
      font-size: 13px;
      font-weight: 600;
      cursor: pointer;
      text-align: center;
      transition: all 0.15s ease;
    }
    .tab-btn.active {
      background: #1e293b;
      color: #ffffff;
      box-shadow: 0 1px 3px rgba(0,0,0,0.3);
    }
    .form-group {
      margin-bottom: 16px;
      text-align: left;
    }
    .form-label {
      display: block;
      font-size: 12px;
      font-weight: 600;
      color: #94a3b8;
      margin-bottom: 6px;
      text-transform: uppercase;
      letter-spacing: 0.04em;
    }
    .form-input {
      width: 100%;
      padding: 10px 14px;
      background: #0f172a;
      border: 1px solid #334155;
      border-radius: 6px;
      color: #f1f5f9;
      font-size: 14px;
      transition: border-color 0.15s ease;
    }
    .form-input:focus {
      outline: none;
      border-color: #0284c7;
      box-shadow: 0 0 0 3px rgba(2, 132, 199, 0.2);
    }
    .form-input[readonly] {
      background: #141c2e;
      color: #94a3b8;
      border-color: #1e293b;
      cursor: default;
    }
    .form-footer {
      margin-top: 14px;
      font-size: 12px;
      color: #64748b;
      text-align: center;
    }
    .form-footer a {
      color: #38bdf8;
      text-decoration: none;
      cursor: pointer;
    }
    .form-footer a:hover {
      text-decoration: underline;
    }
    .status-banner {
      display: flex;
      align-items: center;
      gap: 12px;
      padding: 12px 14px;
      border-radius: 7px;
      margin-bottom: 20px;
      font-size: 13px;
    }
    .status-success {
      background: rgba(16, 185, 129, 0.12);
      border: 1px solid rgba(16, 185, 129, 0.25);
      color: #34d399;
    }
    .status-error {
      background: rgba(239, 68, 68, 0.12);
      border: 1px solid rgba(239, 68, 68, 0.25);
      color: #f87171;
    }
    .status-warning {
      background: rgba(245, 158, 11, 0.12);
      border: 1px solid rgba(245, 158, 11, 0.25);
      color: #fbbf24;
    }
    .session-card {
      background: #0f172a;
      border: 1px solid #1e293b;
      border-radius: 8px;
      padding: 14px 16px;
      margin-bottom: 20px;
      display: flex;
      align-items: center;
      justify-content: space-between;
    }
    .session-info {
      text-align: left;
    }
    .session-label {
      font-size: 11px;
      color: #64748b;
      text-transform: uppercase;
      font-weight: 600;
    }
    .session-email {
      font-size: 14px;
      color: #f8fafc;
      font-weight: 600;
    }
    .switch-link {
      font-size: 12px;
      color: #38bdf8;
      text-decoration: none;
      cursor: pointer;
    }
    .switch-link:hover {
      text-decoration: underline;
    }
    .icon {
      font-size: 18px;
      line-height: 1;
      flex-shrink: 0;
    }
    details {
      margin-top: 20px;
      padding-top: 14px;
      border-top: 1px solid #1f2937;
      font-size: 12px;
      color: #64748b;
      text-align: left;
    }
    summary {
      cursor: pointer;
      font-weight: 600;
      color: #94a3b8;
      user-select: none;
    }
    summary:hover {
      color: #cbd5e1;
    }
    pre {
      background: #090d16;
      border: 1px solid #1e293b;
      border-radius: 6px;
      padding: 10px;
      overflow-x: auto;
      color: #38bdf8;
      font-size: 11px;
      margin-top: 8px;
    }
    .footer {
      margin-top: 24px;
      text-align: center;
      font-size: 12px;
      color: #64748b;
    }
    .hidden {
      display: none !important;
    }
  </style>
</head>
<body>
  <div class="container">
    <div class="brand-header">
      <div class="brand-logo">A</div>
      <div class="brand-name">Adroit Platform</div>
    </div>

    <div class="card">
      {{ if .Success }}
        <!-- Success State -->
        <div class="status-banner status-success">
          <span class="icon">&#10003;</span>
          <div>
            <strong>Invitation Accepted!</strong>
            <div>You have joined {{ .Success.OrganizationName }}.</div>
          </div>
        </div>

        <h1>{{ .Success.OrganizationName }}</h1>
        <p class="subtitle">You are now an active <strong>{{ .Success.Role }}</strong> in this workspace.</p>

        <div class="details-table">
          <div class="details-row">
            <span class="details-label">Workspace</span>
            <span class="details-value">{{ .Success.OrganizationName }}</span>
          </div>
          <div class="details-row">
            <span class="details-label">Assigned Role</span>
            <span class="details-value"><span class="badge">{{ .Success.Role }}</span></span>
          </div>
          <div class="details-row">
            <span class="details-label">Organization ID</span>
            <span class="details-value" style="font-family: monospace; font-size: 11px;">{{ .Success.OrganizationID }}</span>
          </div>
        </div>

        <a href="/api/v1/users/organizations/{{ .Success.OrganizationID }}/members" class="btn">View Organization Members</a>
        <a href="/" class="btn btn-secondary">Platform Home</a>

      {{ else if .Error }}
        <!-- Error State -->
        <div class="status-banner status-error">
          <span class="icon">&#9888;</span>
          <div>
            <strong>Invitation Unavailable</strong>
            <div>{{ .Error }}</div>
          </div>
        </div>

        <h1>Cannot Join Workspace</h1>
        <p class="subtitle">This invitation link is invalid or has expired.</p>

        <a href="/" class="btn btn-secondary">Return to Platform Home</a>

      {{ else if and .Preview (eq .Preview.Status "accepted") }}
        <!-- Already Accepted State -->
        <div class="status-banner status-success">
          <span class="icon">&#10003;</span>
          <div>
            <strong>Invitation Already Accepted</strong>
            <div>This workspace invitation has already been accepted.</div>
          </div>
        </div>

        <h1>{{ .Preview.OrganizationName }}</h1>
        <p class="subtitle">You or a team member have already joined as <span class="badge">{{ .Preview.Role }}</span>.</p>

        <a href="/api/v1/users/organizations/{{ .Preview.OrganizationID }}/members" class="btn">View Organization Members</a>
        <a href="/" class="btn btn-secondary">Platform Home</a>

      {{ else if and .IsLoggedIn .EmailMatches }}
        <!-- State 1: Logged in and email matches -> Acceptance Screen -->
        <h1>Join {{ .Preview.OrganizationName }}</h1>
        <p class="subtitle">
          <strong>{{ .Preview.InvitedByName }}</strong> invited you to collaborate as <span class="badge">{{ .Preview.Role }}</span>.
        </p>

        <div class="session-card">
          <div class="session-info">
            <div class="session-label">Signed in as</div>
            <div class="session-email">{{ .LoggedInEmail }}</div>
          </div>
          <span class="verified-badge">&#10003; Verified</span>
        </div>

        <div class="details-table">
          <div class="details-row">
            <span class="details-label">Workspace</span>
            <span class="details-value">{{ .Preview.OrganizationName }}</span>
          </div>
          <div class="details-row">
            <span class="details-label">Role</span>
            <span class="details-value"><span class="badge">{{ .Preview.Role }}</span></span>
          </div>
          <div class="details-row">
            <span class="details-label">Invited By</span>
            <span class="details-value">{{ .Preview.InvitedByName }}</span>
          </div>
          <div class="details-row">
            <span class="details-label">Expires</span>
            <span class="details-value">{{ .ExpiresAtFormatted }}</span>
          </div>
        </div>

        <form method="POST" action="/invite/accept" id="acceptForm">
          <input type="hidden" name="token" value="{{ .Preview.Token }}">
          <button type="submit" class="btn" id="acceptBtn">Accept Invitation & Join Workspace</button>
        </form>

        <p class="form-footer" style="margin-top: 14px;">
          Not you? <a onclick="logoutAndSwitch()">Sign in with a different account</a>
        </p>

      {{ else if and .IsLoggedIn (not .EmailMatches) }}
        <!-- State 2: Logged in with mismatched email -->
        <div class="status-banner status-warning">
          <span class="icon">&#9888;</span>
          <div>
            <strong>Account Mismatch</strong>
            <div>Signed in as <strong>{{ .LoggedInEmail }}</strong></div>
          </div>
        </div>

        <h1>Invitation for {{ .Preview.Email }}</h1>
        <p class="subtitle">
          This invitation was sent to <strong>{{ .Preview.Email }}</strong>. To accept it, please sign in with that email address.
        </p>

        <button onclick="logoutAndSwitch()" class="btn">Sign In as {{ .Preview.Email }}</button>
        <a href="/" class="btn btn-secondary">Platform Home</a>

      {{ else }}
        <!-- State 3: Not logged in -> Login / Sign Up Page -->
        <h1>Join {{ .Preview.OrganizationName }}</h1>
        <p class="subtitle">
          <strong>{{ .Preview.InvitedByName }}</strong> invited you to collaborate as <span class="badge">{{ .Preview.Role }}</span>.
        </p>

        <div id="authAlert" class="status-banner status-error hidden">
          <span class="icon">&#9888;</span>
          <div id="authAlertMsg"></div>
        </div>

        <!-- Tab Selector -->
        <div class="tabs">
          <button type="button" class="tab-btn {{ if .Preview.UserExists }}active{{ end }}" id="tabLogin" onclick="switchTab('login')">Sign In</button>
          <button type="button" class="tab-btn {{ if not .Preview.UserExists }}active{{ end }}" id="tabRegister" onclick="switchTab('register')">Create Account</button>
        </div>

        <!-- Sign In Form -->
        <form id="loginForm" class="{{ if not .Preview.UserExists }}hidden{{ end }}" onsubmit="handleLogin(event)">
          <div class="form-group">
            <label class="form-label">Email Address</label>
            <input type="email" class="form-input" id="loginEmail" value="{{ .Preview.Email }}" readonly>
          </div>
          <div class="form-group">
            <label class="form-label">Password</label>
            <input type="password" class="form-input" id="loginPassword" placeholder="Enter your password" required autocomplete="current-password">
          </div>
          <button type="submit" class="btn" id="loginBtn">Sign In & Accept Invitation</button>
          <div class="form-footer">
            Don't have an account yet? <a onclick="switchTab('register')">Create an account</a>
          </div>
        </form>

        <!-- Create Account Form -->
        <form id="registerForm" class="{{ if .Preview.UserExists }}hidden{{ end }}" onsubmit="handleRegister(event)">
          <div class="form-group">
            <label class="form-label">Full Name</label>
            <input type="text" class="form-input" id="regName" placeholder="Your Full Name" required autocomplete="name">
          </div>
          <div class="form-group">
            <label class="form-label">Email Address</label>
            <input type="email" class="form-input" id="regEmail" value="{{ .Preview.Email }}" readonly>
          </div>
          <div class="form-group">
            <label class="form-label">Password</label>
            <input type="password" class="form-input" id="regPassword" placeholder="Choose a password (min. 8 characters)" required minlength="8" autocomplete="new-password">
          </div>
          <button type="submit" class="btn" id="regBtn">Create Account & Join Workspace</button>
          <div class="form-footer">
            Already have an account? <a onclick="switchTab('login')">Sign in</a>
          </div>
        </form>

      {{ end }}

      {{ if and .Preview (not .Success) (not .Error) }}
        <details>
          <summary>Developer & CLI Testing</summary>
          <p style="margin: 8px 0 4px;">To accept via authenticated cURL:</p>
          <pre>curl -X POST http://localhost:8080/api/v1/users/invitations/{{ .Preview.Token }}/accept \
  -H "Authorization: Bearer &lt;YOUR_JWT_TOKEN&gt;"</pre>
          <p style="margin: 8px 0 4px;">Invitation Details:</p>
          <pre>Org ID: {{ .Preview.OrganizationID }}
Role:   {{ .Preview.Role }}
Email:  {{ .Preview.Email }}</pre>
        </details>
      {{ end }}
    </div>

    <div class="footer">
      Adroit Platform • Developer Cloud Infrastructure
    </div>
  </div>

  <script>
    const inviteToken = "{{ if .Preview }}{{ .Preview.Token }}{{ end }}";

    // Auto-sync token from localStorage if present
    (function checkStoredAuth() {
      const storedToken = localStorage.getItem('adroit_token');
      const hasCookie = document.cookie.split(';').some(c => c.trim().startsWith('adroit_token='));
      {{ if not .IsLoggedIn }}
      if (storedToken && !hasCookie) {
        document.cookie = "adroit_token=" + storedToken + "; path=/; max-age=86400; SameSite=Lax";
        window.location.reload();
      }
      {{ end }}
    })();

    function switchTab(tab) {
      clearAuthAlert();
      const isLogin = tab === 'login';
      document.getElementById('tabLogin').classList.toggle('active', isLogin);
      document.getElementById('tabRegister').classList.toggle('active', !isLogin);
      document.getElementById('loginForm').classList.toggle('hidden', !isLogin);
      document.getElementById('registerForm').classList.toggle('hidden', isLogin);
    }

    function showAuthAlert(msg) {
      const el = document.getElementById('authAlert');
      const text = document.getElementById('authAlertMsg');
      if (el && text) {
        text.textContent = msg;
        el.classList.remove('hidden');
      }
    }

    function clearAuthAlert() {
      const el = document.getElementById('authAlert');
      if (el) el.classList.add('hidden');
    }

    function logoutAndSwitch() {
      localStorage.removeItem('adroit_token');
      document.cookie = "adroit_token=; path=/; max-age=0; expires=Thu, 01 Jan 1970 00:00:00 GMT";
      window.location.reload();
    }

    async function handleLogin(e) {
      e.preventDefault();
      clearAuthAlert();
      const btn = document.getElementById('loginBtn');
      const email = document.getElementById('loginEmail').value.trim();
      const password = document.getElementById('loginPassword').value;

      btn.disabled = true;
      btn.textContent = 'Signing in...';

      try {
        const res = await fetch('/api/v1/auth/login', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ email, password })
        });
        const data = await res.json();
        if (!res.ok) {
          showAuthAlert(data.error ? data.error.message : 'Invalid credentials. Please try again.');
          btn.disabled = false;
          btn.textContent = 'Sign In & Accept Invitation';
          return;
        }

        const token = (data.data && data.data.token) || data.token;
        localStorage.setItem('adroit_token', token);
        document.cookie = "adroit_token=" + token + "; path=/; max-age=86400; SameSite=Lax";

        btn.textContent = 'Joining Workspace...';
        await acceptWithToken(token);
      } catch (err) {
        showAuthAlert('Network error: ' + err.message);
        btn.disabled = false;
        btn.textContent = 'Sign In & Accept Invitation';
      }
    }

    async function handleRegister(e) {
      e.preventDefault();
      clearAuthAlert();
      const btn = document.getElementById('regBtn');
      const name = document.getElementById('regName').value.trim();
      const email = document.getElementById('regEmail').value.trim();
      const password = document.getElementById('regPassword').value;

      if (password.length < 8) {
        showAuthAlert('Password must be at least 8 characters long.');
        return;
      }

      btn.disabled = true;
      btn.textContent = 'Creating account...';

      try {
        const res = await fetch('/api/v1/auth/register', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ name, email, password })
        });
        const data = await res.json();
        if (!res.ok) {
          showAuthAlert(data.error ? data.error.message : 'Registration failed.');
          btn.disabled = false;
          btn.textContent = 'Create Account & Join Workspace';
          return;
        }

        const token = (data.data && data.data.token) || data.token;
        localStorage.setItem('adroit_token', token);
        document.cookie = "adroit_token=" + token + "; path=/; max-age=86400; SameSite=Lax";

        btn.textContent = 'Joining Workspace...';
        await acceptWithToken(token);
      } catch (err) {
        showAuthAlert('Network error: ' + err.message);
        btn.disabled = false;
        btn.textContent = 'Create Account & Join Workspace';
      }
    }

    async function acceptWithToken(jwtToken) {
      try {
        const res = await fetch('/invite/accept?token=' + encodeURIComponent(inviteToken), {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': 'Bearer ' + jwtToken
          },
          body: JSON.stringify({ token: inviteToken })
        });
        const data = await res.json();
        if (res.ok) {
          renderSuccessState(data.data || data);
        } else {
          showAuthAlert(data.error ? data.error.message : (data.message || 'Failed to join workspace.'));
        }
      } catch (err) {
        window.location.reload();
      }
    }

    function renderSuccessState(payload) {
      var h1 = document.querySelector('h1');
      var orgName = payload.organization_name || (h1 ? h1.textContent.replace(/^Join\s+/, '') : 'Workspace');
      var role = payload.role || 'member';
      var orgID = payload.organization_id || '';

      var card = document.querySelector('.card');
      if (card) {
        var html = '<div class="status-banner status-success">' +
          '<span class="icon">&#10003;</span>' +
          '<div>' +
            '<strong>Invitation Accepted!</strong>' +
            '<div>You have joined ' + orgName + '.</div>' +
          '</div>' +
        '</div>' +
        '<h1>' + orgName + '</h1>' +
        '<p class="subtitle">You are now an active <strong>' + role + '</strong> in this workspace.</p>' +
        '<div class="details-table">' +
          '<div class="details-row">' +
            '<span class="details-label">Workspace</span>' +
            '<span class="details-value">' + orgName + '</span>' +
          '</div>' +
          '<div class="details-row">' +
            '<span class="details-label">Assigned Role</span>' +
            '<span class="details-value"><span class="badge">' + role + '</span></span>' +
          '</div>';

        if (orgID) {
          html += '<div class="details-row">' +
            '<span class="details-label">Organization ID</span>' +
            '<span class="details-value" style="font-family: monospace; font-size: 11px;">' + orgID + '</span>' +
          '</div>';
        }

        html += '</div>';
        if (orgID) {
          html += '<a href="/api/v1/users/organizations/' + orgID + '/members" class="btn">View Organization Members</a>';
        }
        html += '<a href="/" class="btn btn-secondary">Platform Home</a>';

        card.innerHTML = html;
      }
    }

    // Attach listener for direct acceptance form when already logged in
    const acceptForm = document.getElementById('acceptForm');
    if (acceptForm) {
      acceptForm.addEventListener('submit', function(e) {
        const btn = document.getElementById('acceptBtn');
        if (btn) {
          btn.disabled = true;
          btn.textContent = 'Joining Workspace...';
        }
      });
    }
  </script>
</body>
</html>`))

type invitePageData struct {
	Preview            *InvitationPreview
	Success            *AcceptInvitationResponse
	Error              string
	ExpiresAtFormatted string
	IsLoggedIn         bool
	LoggedInEmail      string
	EmailMatches       bool
}

func renderInvitePage(
	w http.ResponseWriter,
	status int,
	preview *InvitationPreview,
	success *AcceptInvitationResponse,
	errMsg string,
	isLoggedIn bool,
	loggedInEmail string,
	emailMatches bool,
) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)

	data := invitePageData{
		Preview:       preview,
		Success:       success,
		Error:         errMsg,
		IsLoggedIn:    isLoggedIn,
		LoggedInEmail: loggedInEmail,
		EmailMatches:  emailMatches,
	}
	if preview != nil && !preview.ExpiresAt.IsZero() {
		data.ExpiresAtFormatted = preview.ExpiresAt.Format("Jan 02, 2006 at 15:04 MST")
	}

	_ = inviteHTMLTemplate.Execute(w, data)
}
