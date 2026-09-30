package notificationservice

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
	texttemplate "text/template"
)

// TemplateManager renders HTML and plain text email templates
type TemplateManager struct {
	htmlTemplates map[string]*template.Template
	textTemplates map[string]*texttemplate.Template
}

// RenderedEmail represents both HTML and plain-text output
type RenderedEmail struct {
	Subject   string
	HTMLBody  string
	PlainBody string
}

// NewTemplateManager initializes pre-compiled templates
func NewTemplateManager() (*TemplateManager, error) {
	tm := &TemplateManager{
		htmlTemplates: make(map[string]*template.Template),
		textTemplates: make(map[string]*texttemplate.Template),
	}

	// 1. Organization Invitation
	htmlInvite, err := template.New("org_invitation").Parse(orgInvitationHTML)
	if err != nil {
		return nil, fmt.Errorf("failed to parse org_invitation html: %w", err)
	}
	textInvite, err := texttemplate.New("org_invitation").Parse(orgInvitationText)
	if err != nil {
		return nil, fmt.Errorf("failed to parse org_invitation text: %w", err)
	}
	tm.htmlTemplates["org_invitation"] = htmlInvite
	tm.textTemplates["org_invitation"] = textInvite

	// 2. Welcome Email
	htmlWelcome, err := template.New("welcome").Parse(welcomeHTML)
	if err != nil {
		return nil, fmt.Errorf("failed to parse welcome html: %w", err)
	}
	textWelcome, err := texttemplate.New("welcome").Parse(welcomeText)
	if err != nil {
		return nil, fmt.Errorf("failed to parse welcome text: %w", err)
	}
	tm.htmlTemplates["welcome"] = htmlWelcome
	tm.textTemplates["welcome"] = textWelcome

	// 3. System Alert
	htmlAlert, err := template.New("alert").Parse(alertHTML)
	if err != nil {
		return nil, fmt.Errorf("failed to parse alert html: %w", err)
	}
	textAlert, err := texttemplate.New("alert").Parse(alertText)
	if err != nil {
		return nil, fmt.Errorf("failed to parse alert text: %w", err)
	}
	tm.htmlTemplates["alert"] = htmlAlert
	tm.textTemplates["alert"] = textAlert

	return tm, nil
}

// Render compiles template with data and returns RenderedEmail
func (tm *TemplateManager) Render(templateName string, data map[string]any) (*RenderedEmail, error) {
	htmlTpl, exists := tm.htmlTemplates[templateName]
	if !exists {
		return nil, fmt.Errorf("html template '%s' not found", templateName)
	}

	textTpl, exists := tm.textTemplates[templateName]
	if !exists {
		return nil, fmt.Errorf("text template '%s' not found", templateName)
	}

	var htmlBuf bytes.Buffer
	if err := htmlTpl.Execute(&htmlBuf, data); err != nil {
		return nil, fmt.Errorf("failed to execute html template %s: %w", templateName, err)
	}

	var textBuf bytes.Buffer
	if err := textTpl.Execute(&textBuf, data); err != nil {
		return nil, fmt.Errorf("failed to execute text template %s: %w", templateName, err)
	}

	var subject string
	switch templateName {
	case "org_invitation":
		orgName, _ := data["OrganizationName"].(string)
		if orgName == "" {
			orgName = "an organization"
		}
		subject = fmt.Sprintf("You've been invited to join %s on Adroit Platform", orgName)
	case "welcome":
		name, _ := data["Name"].(string)
		if name == "" {
			name = "Developer"
		}
		subject = fmt.Sprintf("Welcome to Adroit Platform, %s!", name)
	case "alert":
		title, _ := data["Title"].(string)
		if title == "" {
			title = "Platform Notification"
		}
		subject = fmt.Sprintf("[Adroit Alert] %s", title)
	default:
		subject = "Notification from Adroit Platform"
	}

	return &RenderedEmail{
		Subject:   subject,
		HTMLBody:  htmlBuf.String(),
		PlainBody: strings.TrimSpace(textBuf.String()),
	}, nil
}

// ============================================================================
// Raw Templates (HTML & Plain Text)
// ============================================================================

const orgInvitationHTML = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Organization Invitation</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; background-color: #0b0f19; color: #f3f4f6; margin: 0; padding: 24px; }
    .card { max-width: 580px; margin: 0 auto; background: #111827; border: 1px solid #1f2937; border-radius: 12px; overflow: hidden; box-shadow: 0 10px 25px rgba(0,0,0,0.5); }
    .header { background: linear-gradient(135deg, #0284c7 0%, #6366f1 100%); padding: 32px 24px; text-align: center; }
    .header h1 { margin: 0; color: #ffffff; font-size: 24px; font-weight: 700; letter-spacing: -0.5px; }
    .content { padding: 32px 28px; line-height: 1.6; }
    .content p { color: #d1d5db; font-size: 15px; margin: 0 0 16px; }
    .highlight { color: #38bdf8; font-weight: 600; }
    .badge { display: inline-block; padding: 4px 10px; background: #1e293b; border: 1px solid #334155; border-radius: 6px; font-size: 13px; color: #a5f3fc; font-weight: 600; }
    .cta-container { text-align: center; margin: 32px 0; }
    .cta-btn { display: inline-block; background: #0284c7; color: #ffffff !important; text-decoration: none; padding: 14px 32px; border-radius: 8px; font-weight: 600; font-size: 16px; box-shadow: 0 4px 14px rgba(2, 132, 199, 0.4); }
    .cta-btn:hover { background: #0369a1; }
    .footer { border-top: 1px solid #1f2937; padding: 20px 28px; font-size: 12px; color: #6b7280; text-align: center; }
    .link-fallback { word-break: break-all; color: #9ca3af; font-size: 12px; margin-top: 12px; }
  </style>
</head>
<body>
  <div class="card">
    <div class="header">
      <h1>ADROIT PLATFORM</h1>
    </div>
    <div class="content">
      <p>Hello,</p>
      <p>
        <span class="highlight">{{ if .InvitedByName }}{{ .InvitedByName }}{{ else if .InviterName }}{{ .InviterName }}{{ else }}A workspace administrator{{ end }}</span> has invited you to join 
        <strong style="color: #ffffff;">{{ .OrganizationName }}</strong> on Adroit Platform as 
        <span class="badge">{{ .Role }}</span>.
      </p>
      <p>
        Adroit is the developer-centric cloud PaaS for automated building, isolated container & microVM execution, managed databases, and dynamic routing.
      </p>
      <div class="cta-container">
        <a href="{{ .InviteURL }}" class="cta-btn" target="_blank">Accept Invitation</a>
      </div>
      <p style="font-size: 13px; color: #9ca3af;">
        This invitation will expire on <strong>{{ .ExpiresAt }}</strong>. If you did not expect this invitation, you can safely ignore this email.
      </p>
      <div class="link-fallback">
        If the button doesn't work, copy and paste this link into your browser:<br>
        <a href="{{ .InviteURL }}" style="color: #38bdf8;">{{ .InviteURL }}</a>
      </div>
    </div>
    <div class="footer">
      &copy; Adroit Platform. Modern Cloud Platform-as-a-Service.
    </div>
  </div>
</body>
</html>`

const orgInvitationText = `ADROIT PLATFORM INVITATION
=========================================

Hello,

{{ if .InvitedByName }}{{ .InvitedByName }}{{ else if .InviterName }}{{ .InviterName }}{{ else }}A workspace administrator{{ end }} has invited you to join {{ .OrganizationName }} on Adroit Platform as {{ .Role }}.

To accept this invitation and join the workspace, open the link below in your browser:
{{ .InviteURL }}

This invitation will expire on: {{ .ExpiresAt }}.

If you did not expect this invitation, you can safely ignore this email.

--
Adroit Platform Team
`

const welcomeHTML = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>Welcome to Adroit</title>
</head>
<body style="font-family: sans-serif; background: #0b0f19; color: #f3f4f6; padding: 24px;">
  <div style="max-width: 580px; margin: 0 auto; background: #111827; padding: 32px; border-radius: 8px; border: 1px solid #1f2937;">
    <h2 style="color: #38bdf8; margin-top: 0;">Welcome to Adroit, {{ .Name }}!</h2>
    <p>We are thrilled to have you. Your personal developer workspace is ready to deploy containers, managed databases, and microVMs.</p>
    <p><a href="{{ .DashboardURL }}" style="background: #0284c7; color: white; padding: 10px 20px; text-decoration: none; border-radius: 6px; display: inline-block;">Open Dashboard</a></p>
  </div>
</body>
</html>`

const welcomeText = `Welcome to Adroit, {{ .Name }}!

We're thrilled to have you. Your personal developer workspace is ready.
Open your dashboard: {{ .DashboardURL }}

--
Adroit Platform Team
`

const alertHTML = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>Adroit Alert</title>
</head>
<body style="font-family: sans-serif; background: #0b0f19; color: #f3f4f6; padding: 24px;">
  <div style="max-width: 580px; margin: 0 auto; background: #111827; padding: 32px; border-radius: 8px; border: 1px solid #ef4444;">
    <h2 style="color: #f87171; margin-top: 0;">[Alert] {{ .Title }}</h2>
    <p>{{ .Description }}</p>
    {{ if .ActionURL }}
    <p><a href="{{ .ActionURL }}" style="background: #ef4444; color: white; padding: 10px 20px; text-decoration: none; border-radius: 6px; display: inline-block;">View in Dashboard</a></p>
    {{ end }}
  </div>
</body>
</html>`

const alertText = `[ADROIT ALERT] {{ .Title }}
=========================================

{{ .Description }}

{{ if .ActionURL }}
View in Dashboard: {{ .ActionURL }}
{{ end }}

--
Adroit Platform Observability
`
