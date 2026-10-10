package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
)

// PromptLogin runs an interactive login form asking for email and password.
func PromptLogin() (string, string, error) {
	var email string
	var password string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Work Email").
				Placeholder("user@company.com").
				Value(&email).
				Validate(func(s string) error {
					if !strings.Contains(s, "@") || !strings.Contains(s, ".") {
						return fmt.Errorf("please enter a valid email address")
					}
					return nil
				}),
			huh.NewInput().
				Title("Password").
				EchoMode(huh.EchoModePassword).
				Value(&password).
				Validate(func(s string) error {
					if len(s) < 6 {
						return fmt.Errorf("password must be at least 6 characters")
					}
					return nil
				}),
		),
	)

	if err := form.Run(); err != nil {
		return "", "", err
	}

	return strings.TrimSpace(email), strings.TrimSpace(password), nil
}

// PromptInput requests a single string input with validation.
func PromptInput(title, placeholder, defaultValue string) (string, error) {
	val := defaultValue
	err := huh.NewInput().
		Title(title).
		Placeholder(placeholder).
		Value(&val).
		Run()
	return strings.TrimSpace(val), err
}

// PromptPassword requests a masked password or secret.
func PromptPassword(title string) (string, error) {
	var val string
	err := huh.NewInput().
		Title(title).
		EchoMode(huh.EchoModePassword).
		Value(&val).
		Run()
	return strings.TrimSpace(val), err
}

// PromptConfirm asks a yes/no question.
func PromptConfirm(title string, defaultYes bool) (bool, error) {
	confirmed := defaultYes
	err := huh.NewConfirm().
		Title(title).
		Value(&confirmed).
		Run()
	return confirmed, err
}

// PromptSelect asks user to pick an option from a list.
func PromptSelect(title string, options []string) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("no options available")
	}

	var selected string
	huhOptions := make([]huh.Option[string], len(options))
	for i, opt := range options {
		huhOptions[i] = huh.NewOption(opt, opt)
	}

	err := huh.NewSelect[string]().
		Title(title).
		Options(huhOptions...).
		Value(&selected).
		Run()

	return selected, err
}
