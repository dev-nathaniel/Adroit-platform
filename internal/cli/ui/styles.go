package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	// Brand color palette
	ColorCyanElectric = lipgloss.Color("#00F2FE")
	ColorCyanBright   = lipgloss.Color("#00D2FE")
	ColorAzureMid     = lipgloss.Color("#2596FE")
	ColorBlueDeep     = lipgloss.Color("#4F75FE")
	ColorIndigo       = lipgloss.Color("#6366F1")
	ColorViolet       = lipgloss.Color("#7C5CFC")

	ColorPrimary = lipgloss.Color("#00D2FE")
	ColorBlue    = lipgloss.Color("#2563EB")
	ColorSuccess = lipgloss.Color("#10B981") // Emerald
	ColorWarning = lipgloss.Color("#F59E0B") // Amber
	ColorError   = lipgloss.Color("#EF4444") // Red
	ColorMuted   = lipgloss.Color("#64748B") // Slate gray

	// Base typography
	StyleBold    = lipgloss.NewStyle().Bold(true)
	StylePrimary = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	StyleSuccess = lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true)
	StyleWarning = lipgloss.NewStyle().Foreground(ColorWarning).Bold(true)
	StyleError   = lipgloss.NewStyle().Foreground(ColorError).Bold(true)
	StyleMuted   = lipgloss.NewStyle().Foreground(ColorMuted)
	StyleCode    = lipgloss.NewStyle().Foreground(ColorViolet).Background(lipgloss.Color("#1F2937")).Padding(0, 1)

	// Headers and banners
	StyleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			MarginTop(1).
			MarginBottom(1)

	StyleBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(0, 1)
)

// Banner returns the currently active header banner.
// To switch to any version, uncomment its return statement below:
func Banner() string {
	// --- VERSION 1 (Active): 5-step slant neon gradient with pills & divider ---
	// return bannerSlantNeonGradient()

	// --- VERSION 2: White block with empty line gap between row 1 and row 2 ---
	// return bannerWhiteBlockWithLineGap()

	// --- VERSION 3: White block with pills & divider underneath ---
	// return bannerWhiteBlockWithBadgesBelow()

	// --- VERSION 4: Minimalist white block with inline text on line 2 ---
	// return bannerWhiteBlockInline()

	// --- VERSION 5: Dual-tone side-by-side logo with badges on the right ---
	// return bannerDualToneSideBySide()

	// --- VERSION 6: Original flat-cyan FIGlet with underscores ---
	// return bannerOriginalFiglet()

	// --- VERSION 7: White two layer block logo with badges on the right ---
	return bannerWhiteTwoLayerBlockLogoWithBadgesOnTheRight()
}

// ============================================================================
// ALL BANNER VARIATIONS (Commented and ready to switch)
// ============================================================================

// VARIATION 1: Modern 5-line Slant Font with 5-Step Vertical Neon Gradient
//
//     ___    ____  ____  ____  ________
//    /   |  / __ \/ __ \/ __ \/  _/_  __/
//   / /| | / / / / /_/ / / / // /  / /   
//  / ___ |/ /_/ / _, _/ /_/ // /  / /    
// /_/  |_/_____/_/ |_|\____/___/ /_/     
//     ADROIT   v1.0.0   Cloud Platform-as-a-Service • Nomad & BuildKit
//   ─────────────────────────────────────────────────────────
func bannerSlantNeonGradient() string {
	lines := []string{
		`    ___    ____  ____  ____  ________`,
		`   /   |  / __ \/ __ \/ __ \/  _/_  __/`,
		`  / /| | / / / / /_/ / / / // /  / /   `,
		` / ___ |/ /_/ / _, _/ /_/ // /  / /    `,
		`/_/  |_/_____/_/ |_|\____/___/ /_/     `,
	}

	colors := []lipgloss.Color{
		lipgloss.Color("#00F2FE"), // Line 1: Neon Cyan
		lipgloss.Color("#00D2FE"), // Line 2: Vivid Cyan
		lipgloss.Color("#2596FE"), // Line 3: Sky Azure
		lipgloss.Color("#4F75FE"), // Line 4: Deep Azure
		lipgloss.Color("#7C5CFC"), // Line 5: Electric Violet
	}

	var sb strings.Builder
	sb.WriteString("\n")

	for i, line := range lines {
		style := lipgloss.NewStyle().Foreground(colors[i]).Bold(true)
		sb.WriteString(style.Render(line))
		sb.WriteString("\n")
	}

	badgePill := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#2563EB")).
		Padding(0, 1).
		Render("ADROIT")

	versionPill := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#38BDF8")).
		Background(lipgloss.Color("#0F172A")).
		Padding(0, 1).
		Render("v1.0.0")

	tagline := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#94A3B8")).
		Render("Cloud Platform-as-a-Service • Nomad & BuildKit")

	divider := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#334155")).
		Render("  ─────────────────────────────────────────────────────────")

	sb.WriteString(fmt.Sprintf("   %s %s  %s\n", badgePill, versionPill, tagline))
	sb.WriteString(divider)
	sb.WriteString("\n")

	return sb.String()
}

// VARIATION 2: White Half-Block with an Empty Spacing Line between the two block lines
//
//   ▄▀█ █▀▄ █▀█ █▀█ █ ▀█▀
//
//   █▀█ █▄▀ █▀▄ █▄█ █  █ 
//
//    ADROIT   v1.0.0   Cloud Platform-as-a-Service • Nomad & BuildKit
//   ─────────────────────────────────────────────────────────

func bannerWhiteBlockWithLineGap() string {
	line1 := `  ▄▀█ █▀▄ █▀█ █▀█ █ ▀█▀`
	line2 := `  █▀█ █▄▀ █▀▄ █▄█ █  █ `

	whiteLogo := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true)

	badgePill := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#2563EB")).
		Padding(0, 1).
		Render("ADROIT")

	versionPill := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#38BDF8")).
		Background(lipgloss.Color("#0F172A")).
		Padding(0, 1).
		Render("v1.0.0")

	tagline := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#94A3B8")).
		Render("Cloud Platform-as-a-Service • Nomad & BuildKit")

	divider := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#334155")).
		Render("  ─────────────────────────────────────────────────────────")

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(whiteLogo.Render(line1))
	sb.WriteString("\n\n") // Empty line gap between the two block lines
	sb.WriteString(whiteLogo.Render(line2))
	sb.WriteString("\n\n") // Space before metadata
	sb.WriteString(fmt.Sprintf("  %s %s  %s\n", badgePill, versionPill, tagline))
	sb.WriteString(divider)
	sb.WriteString("\n")

	return sb.String()
}


// VARIATION 3: White Half-Block Logo with space below, followed by badges and divider
//
//   ▄▀█ █▀▄ █▀█ █▀█ █ ▀█▀
//   █▀█ █▄▀ █▀▄ █▄█ █  █ 
//
//    ADROIT   v1.0.0   Cloud Platform-as-a-Service • Nomad & BuildKit
//   ─────────────────────────────────────────────────────────

func bannerWhiteBlockWithBadgesBelow() string {
	line1 := `  ▄▀█ █▀▄ █▀█ █▀█ █ ▀█▀`
	line2 := `  █▀█ █▄▀ █▀▄ █▄█ █  █ `

	whiteLogo := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true)

	badgePill := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#2563EB")).
		Padding(0, 1).
		Render("ADROIT")

	versionPill := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#38BDF8")).
		Background(lipgloss.Color("#0F172A")).
		Padding(0, 1).
		Render("v1.0.0")

	tagline := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#94A3B8")).
		Render("Cloud Platform-as-a-Service • Nomad & BuildKit")

	divider := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#334155")).
		Render("  ─────────────────────────────────────────────────────────")

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(whiteLogo.Render(line1))
	sb.WriteString("\n")
	sb.WriteString(whiteLogo.Render(line2))
	sb.WriteString("\n\n")
	sb.WriteString(fmt.Sprintf("  %s %s  %s\n", badgePill, versionPill, tagline))
	sb.WriteString(divider)
	sb.WriteString("\n")

	return sb.String()
}


// VARIATION 4: Ultra-Minimalist Pure White Half-Block with inline text on line 2
//
//   ▄▀█ █▀▄ █▀█ █▀█ █ ▀█▀
//   █▀█ █▄▀ █▀▄ █▄█ █  █   ADROIT v1.0.0 • Cloud PaaS

func bannerWhiteBlockInline() string {
	line1 := `  ▄▀█ █▀▄ █▀█ █▀█ █ ▀█▀`
	line2 := `  █▀█ █▄▀ █▀▄ █▄█ █  █   ADROIT v1.0.0 • Cloud PaaS`

	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true)

	return fmt.Sprintf("\n%s\n%s\n\n", style.Render(line1), style.Render(line2))
}


// VARIATION 5: Dual-Tone Gradient Half-Block (Logo on left, Badges & Tagline on right)
//
//   ▄▀█ █▀▄ █▀█ █▀█ █ ▀█▀    ADROIT   v1.0.0 
//   █▀█ █▄▀ █▀▄ █▄█ █  █    Cloud Platform-as-a-Service • Nomad & BuildKit
//   ──────────────────────────────────────────────────────────────────

func bannerDualToneSideBySide() string {
	logoLine1 := `  ▄▀█ █▀▄ █▀█ █▀█ █ ▀█▀`
	logoLine2 := `  █▀█ █▄▀ █▀▄ █▄█ █  █ `

	styleLine1 := lipgloss.NewStyle().Foreground(ColorCyanElectric).Bold(true)
	styleLine2 := lipgloss.NewStyle().Foreground(ColorBlueDeep).Bold(true)

	badgePill := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#2563EB")).
		Padding(0, 1).
		Render("ADROIT")

	versionPill := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#38BDF8")).
		Background(lipgloss.Color("#0F172A")).
		Padding(0, 1).
		Render("v1.0.0")

	tagline := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#94A3B8")).
		Render("Cloud Platform-as-a-Service • Nomad & BuildKit")

	divider := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#334155")).
		Render("  ──────────────────────────────────────────────────────────────────")

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("%s   %s %s\n", styleLine1.Render(logoLine1), badgePill, versionPill))
	sb.WriteString(fmt.Sprintf("%s   %s\n", styleLine2.Render(logoLine2), tagline))
	sb.WriteString(divider)
	sb.WriteString("\n")

	return sb.String()
}


// VARIATION 6: Initial Flat-Cyan FIGlet with Underscores
//
//    _____     .___           .__  __   
//   /  _  \  __| _/______  ___|__|/  |_ 
//  /  /_\  \/ __ |\_  __ \/  _ \  \   __\
// /    |    / /_/ | |  | \(  <_> )  ||  |  
// \____|__  \____ | |__|   \____/|__||__|  
//         \/     \/  Cloud PaaS v1.0.0      

func bannerOriginalFiglet() string {
	logo := `
   _____     .___           .__  __   
  /  _  \  __| _/______  ___|__|/  |_ 
 /  /_\  \/ __ |\_  __ \/  _ \  \   __\
/    |    / /_/ | |  | \(  <_> )  ||  |  
\____|__  \____ | |__|   \____/|__||__|  
        \/     \/  Cloud PaaS v1.0.0      
`
	return lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(logo)
}

// VARIATION 7: White Two layer block logo with badges on the right
//
//   ▄▀█ █▀▄ █▀█ █▀█ █ ▀█▀    ADROIT   v1.0.0 
//   █▀█ █▄▀ █▀▄ █▄█ █  █    Cloud Platform-as-a-Service • Nomad & BuildKit
//   ──────────────────────────────────────────────────────────────────

func bannerWhiteTwoLayerBlockLogoWithBadgesOnTheRight() string {
	logoLine1 := `  ▄▀█ █▀▄ █▀█ █▀█ █ ▀█▀`
	logoLine2 := `  █▀█ █▄▀ █▀▄ █▄█ █  █ `

	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true)

	badgePill := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#2563EB")).
		Padding(0, 1).
		Render("ADROIT")

	versionPill := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#38BDF8")).
		Background(lipgloss.Color("#0F172A")).
		Padding(0, 1).
		Render("v1.0.0")

	tagline := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#94A3B8")).
		Render("Cloud Platform-as-a-Service • Nomad & BuildKit")

	divider := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#334155")).
		Render("  ──────────────────────────────────────────────────────────────────")

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("%s   %s %s\n", style.Render(logoLine1), badgePill, versionPill))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("%s   %s\n", style.Render(logoLine2), tagline))
	sb.WriteString(divider)
	sb.WriteString("\n")

	return sb.String()
}


// ============================================================================
// UI HELPERS & BADGES
// ============================================================================

// Success prints a green success message.
func Success(msg string) string {
	return StyleSuccess.Render("✓ ") + msg
}

// Error prints a red error message.
func Error(msg string) string {
	return StyleError.Render("✗ ") + msg
}

// Warning prints a yellow warning message.
func Warning(msg string) string {
	return StyleWarning.Render("⚠ ") + msg
}

// Info prints an info bullet.
func Info(msg string) string {
	return StylePrimary.Render("• ") + msg
}

// StatusBadge renders a colored badge depending on resource state.
func StatusBadge(status string) string {
	switch strings.ToLower(status) {
	case "active", "healthy", "running", "succeeded", "verified":
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(ColorSuccess).
			Padding(0, 1).
			Bold(true).
			Render(" " + strings.ToUpper(status) + " ")
	case "building", "queued", "pending", "deploying":
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Background(ColorWarning).
			Padding(0, 1).
			Bold(true).
			Render(" " + strings.ToUpper(status) + " ")
	case "failed", "error", "unhealthy":
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(ColorError).
			Padding(0, 1).
			Bold(true).
			Render(" " + strings.ToUpper(status) + " ")
	default:
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(ColorMuted).
			Padding(0, 1).
			Render(" " + strings.ToUpper(status) + " ")
	}
}

// KeyValue formats a key-value label pair.
func KeyValue(key string, val any) string {
	return fmt.Sprintf("  %s %s", StyleMuted.Render(fmt.Sprintf("%-16s", key+":")), StyleBold.Render(fmt.Sprintf("%v", val)))
}
