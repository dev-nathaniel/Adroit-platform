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

const orgInvitationHTML = `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">
<html xmlns="http://www.w3.org/1999/xhtml" lang="en">
<head>
  <meta http-equiv="Content-Type" content="text/html; charset=UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0"/>
  <title>Join {{ .OrganizationName }} on Adroit</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f8fafc; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; -webkit-font-smoothing: antialiased; -moz-osx-font-smoothing: grayscale; color: #334155;">
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background-color: #f8fafc; padding: 40px 16px;">
    <tr>
      <td align="center">
        <!-- Main Card -->
        <table role="presentation" cellpadding="0" cellspacing="0" border="0" style="max-width: 540px; width: 100%; background-color: #ffffff; border: 1px solid #e2e8f0; border-radius: 8px; box-shadow: 0 1px 3px rgba(0,0,0,0.04); overflow: hidden; text-align: left;">
          
          <!-- Header / Brand -->
          <tr>
            <td style="padding: 32px 36px 20px 36px;">
              <table role="presentation" cellpadding="0" cellspacing="0" border="0">
                <tr>
                  <td style="width: 28px; height: 28px; background-color: #0f172a; border-radius: 6px; text-align: center; vertical-align: middle;">
                    <span style="font-size: 15px; font-weight: 800; color: #ffffff; line-height: 28px; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;">A</span>
                  </td>
                  <td style="padding-left: 10px; font-size: 17px; font-weight: 700; color: #0f172a; letter-spacing: -0.02em;">
                    Adroit
                  </td>
                </tr>
              </table>
            </td>
          </tr>

          <!-- Main Content -->
          <tr>
            <td style="padding: 0 36px 36px 36px;">
              <h1 style="margin: 8px 0 16px 0; font-size: 20px; font-weight: 600; color: #0f172a; letter-spacing: -0.02em; line-height: 1.3;">
                Join {{ .OrganizationName }} on Adroit
              </h1>

              <p style="margin: 0 0 20px 0; font-size: 15px; line-height: 1.6; color: #334155;">
                <strong style="color: #0f172a;">{{ if .InvitedByName }}{{ .InvitedByName }}{{ else if .InviterName }}{{ .InviterName }}{{ else }}A team administrator{{ end }}</strong>{{ if .InvitedByEmail }} ({{ .InvitedByEmail }}){{ end }} has invited you to join the <strong style="color: #0f172a;">{{ .OrganizationName }}</strong> organization as <span style="display: inline-block; padding: 2px 8px; font-size: 12px; font-weight: 600; color: #0284c7; background-color: #f0f9ff; border: 1px solid #bae6fd; border-radius: 4px; text-transform: lowercase;">{{ .Role }}</span>.
              </p>

              <!-- Workspace Details Panel -->
              <table role="presentation" cellpadding="0" cellspacing="0" border="0" style="width: 100%; margin: 20px 0 24px 0; background-color: #f8fafc; border: 1px solid #e2e8f0; border-radius: 6px; font-size: 14px;">
                <tr>
                  <td style="padding: 12px 16px; border-bottom: 1px solid #e2e8f0; color: #64748b; width: 30%;">Organization</td>
                  <td style="padding: 12px 16px; border-bottom: 1px solid #e2e8f0; color: #0f172a; font-weight: 600;">{{ .OrganizationName }}</td>
                </tr>
                <tr>
                  <td style="padding: 12px 16px; border-bottom: 1px solid #e2e8f0; color: #64748b;">Assigned Role</td>
                  <td style="padding: 12px 16px; border-bottom: 1px solid #e2e8f0; color: #0f172a; font-weight: 600;">{{ .Role }}</td>
                </tr>
                <tr>
                  <td style="padding: 12px 16px; color: #64748b;">Invited By</td>
                  <td style="padding: 12px 16px; color: #0f172a; font-weight: 600;">{{ if .InvitedByName }}{{ .InvitedByName }}{{ else if .InviterName }}{{ .InviterName }}{{ else }}Administrator{{ end }}</td>
                </tr>
              </table>

              <!-- Action Button -->
              <table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin: 28px 0 20px 0;">
                <tr>
                  <td>
                    <a href="{{ .InviteURL }}" target="_blank" style="display: inline-block; background-color: #0284c7; color: #ffffff !important; font-size: 14px; font-weight: 600; text-decoration: none; padding: 12px 28px; border-radius: 6px; letter-spacing: -0.01em;">
                      Accept Invitation
                    </a>
                  </td>
                </tr>
              </table>

              <p style="margin: 0 0 16px 0; font-size: 13px; line-height: 1.5; color: #64748b;">
                This invitation link will expire on {{ .ExpiresAt }}. If you were not expecting this invitation, you can safely ignore this email.
              </p>

              <!-- Link Fallback -->
              <div style="margin-top: 24px; padding-top: 18px; border-top: 1px solid #f1f5f9;">
                <p style="margin: 0 0 6px 0; font-size: 12px; color: #94a3b8;">
                  If the button above does not work, copy and paste this link into your browser:
                </p>
                <p style="margin: 0; font-size: 12px; word-break: break-all;">
                  <a href="{{ .InviteURL }}" style="color: #0284c7; text-decoration: underline;">{{ .InviteURL }}</a>
                </p>
              </div>
            </td>
          </tr>
        </table>

        <!-- Sub-card Footer -->
        <table role="presentation" cellpadding="0" cellspacing="0" border="0" style="max-width: 540px; width: 100%; margin-top: 20px; text-align: center;">
          <tr>
            <td style="font-size: 12px; color: #94a3b8; line-height: 1.5;">
              Adroit Platform • Developer Cloud Infrastructure
            </td>
          </tr>
        </table>

      </td>
    </tr>
  </table>
</body>
</html>`

const orgInvitationText = `Join {{ .OrganizationName }} on Adroit
========================================

{{ if .InvitedByName }}{{ .InvitedByName }}{{ else if .InviterName }}{{ .InviterName }}{{ else }}A team administrator{{ end }} has invited you to join {{ .OrganizationName }} on Adroit as {{ .Role }}.

To accept this invitation and access the workspace, open the link below in your browser:
{{ .InviteURL }}

This invitation will expire on: {{ .ExpiresAt }}.

If you were not expecting this invitation, you can safely ignore this email.

--
Adroit Platform
`

const welcomeHTML = `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">
<html xmlns="http://www.w3.org/1999/xhtml" lang="en">
<head>
  <meta http-equiv="Content-Type" content="text/html; charset=UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0"/>
  <title>Welcome to Adroit</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f8fafc; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; -webkit-font-smoothing: antialiased; color: #334155;">
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background-color: #f8fafc; padding: 40px 16px;">
    <tr>
      <td align="center">
        <table role="presentation" cellpadding="0" cellspacing="0" border="0" style="max-width: 540px; width: 100%; background-color: #ffffff; border: 1px solid #e2e8f0; border-radius: 8px; box-shadow: 0 1px 3px rgba(0,0,0,0.04); overflow: hidden; text-align: left;">
          <tr>
            <td style="padding: 32px 36px 20px 36px;">
              <table role="presentation" cellpadding="0" cellspacing="0" border="0">
                <tr>
                  <td style="width: 28px; height: 28px; background-color: #0f172a; border-radius: 6px; text-align: center; vertical-align: middle;">
                    <span style="font-size: 15px; font-weight: 800; color: #ffffff; line-height: 28px;">A</span>
                  </td>
                  <td style="padding-left: 10px; font-size: 17px; font-weight: 700; color: #0f172a; letter-spacing: -0.02em;">
                    Adroit
                  </td>
                </tr>
              </table>
            </td>
          </tr>
          <tr>
            <td style="padding: 0 36px 36px 36px;">
              <h1 style="margin: 8px 0 16px 0; font-size: 20px; font-weight: 600; color: #0f172a; letter-spacing: -0.02em;">
                Welcome to Adroit, {{ .Name }}
              </h1>
              <p style="margin: 0 0 20px 0; font-size: 15px; line-height: 1.6; color: #334155;">
                Your developer account is active and ready. You can now build, deploy, and manage your isolated services, databases, and microVMs.
              </p>
              <table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin: 28px 0 24px 0;">
                <tr>
                  <td>
                    <a href="{{ .DashboardURL }}" target="_blank" style="display: inline-block; background-color: #0284c7; color: #ffffff !important; font-size: 14px; font-weight: 600; text-decoration: none; padding: 12px 28px; border-radius: 6px; letter-spacing: -0.01em;">
                      Open Dashboard
                    </a>
                  </td>
                </tr>
              </table>
              <div style="margin-top: 24px; padding-top: 18px; border-top: 1px solid #f1f5f9;">
                <p style="margin: 0 0 6px 0; font-size: 12px; color: #94a3b8;">Direct Dashboard link:</p>
                <p style="margin: 0; font-size: 12px; word-break: break-all;">
                  <a href="{{ .DashboardURL }}" style="color: #0284c7; text-decoration: underline;">{{ .DashboardURL }}</a>
                </p>
              </div>
            </td>
          </tr>
        </table>
        <table role="presentation" cellpadding="0" cellspacing="0" border="0" style="max-width: 540px; width: 100%; margin-top: 20px; text-align: center;">
          <tr>
            <td style="font-size: 12px; color: #94a3b8; line-height: 1.5;">
              Adroit Platform • Developer Cloud Infrastructure
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`

const welcomeText = `Welcome to Adroit, {{ .Name }}!
========================================

Your personal developer workspace is ready.
Open your dashboard: {{ .DashboardURL }}

--
Adroit Platform
`

const alertHTML = `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">
<html xmlns="http://www.w3.org/1999/xhtml" lang="en">
<head>
  <meta http-equiv="Content-Type" content="text/html; charset=UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0"/>
  <title>Adroit Alert: {{ .Title }}</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f8fafc; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; -webkit-font-smoothing: antialiased; color: #334155;">
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background-color: #f8fafc; padding: 40px 16px;">
    <tr>
      <td align="center">
        <table role="presentation" cellpadding="0" cellspacing="0" border="0" style="max-width: 540px; width: 100%; background-color: #ffffff; border: 1px solid #fecaca; border-top: 3px solid #ef4444; border-radius: 8px; box-shadow: 0 1px 3px rgba(0,0,0,0.04); overflow: hidden; text-align: left;">
          <tr>
            <td style="padding: 28px 36px 24px 36px;">
              <table role="presentation" cellpadding="0" cellspacing="0" border="0">
                <tr>
                  <td>
                    <span style="display: inline-block; padding: 3px 8px; background-color: #fee2e2; color: #b91c1c; font-size: 11px; font-weight: 700; border-radius: 4px; text-transform: uppercase; letter-spacing: 0.05em;">Alert</span>
                  </td>
                </tr>
              </table>
              <h1 style="margin: 12px 0 16px 0; font-size: 18px; font-weight: 600; color: #0f172a; letter-spacing: -0.01em;">
                {{ .Title }}
              </h1>
              <div style="background-color: #f8fafc; border: 1px solid #e2e8f0; border-radius: 6px; padding: 14px 16px; font-family: monospace; font-size: 13px; line-height: 1.5; color: #334155; margin: 16px 0 24px 0;">
                {{ .Description }}
              </div>
              {{ if .ActionURL }}
              <table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin: 20px 0 12px 0;">
                <tr>
                  <td>
                    <a href="{{ .ActionURL }}" target="_blank" style="display: inline-block; background-color: #ef4444; color: #ffffff !important; font-size: 14px; font-weight: 600; text-decoration: none; padding: 10px 24px; border-radius: 6px;">
                      View in Dashboard
                    </a>
                  </td>
                </tr>
              </table>
              {{ end }}
            </td>
          </tr>
        </table>
        <table role="presentation" cellpadding="0" cellspacing="0" border="0" style="max-width: 540px; width: 100%; margin-top: 20px; text-align: center;">
          <tr>
            <td style="font-size: 12px; color: #94a3b8; line-height: 1.5;">
              Adroit Platform Monitoring & Observability
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
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
