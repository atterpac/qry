package view

import (
	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/validators"

	"github.com/atterpac/qry/internal/config"
)

// showProfileForm shows a form for creating or editing a connection profile.
func (a *App) showProfileForm(name string, cfg config.ConnectionConfig, isEdit bool) {
	builder := components.NewFormBuilder()

	if isEdit {
		builder.Text("name", "Profile Name").
			Value(name).
			Done()
	} else {
		builder.Text("name", "Profile Name").
			Placeholder("Enter profile name").
			Validate(validators.Required()).
			Done()
	}

	engines := []string{"sqlite", "postgres", "mysql"}
	engineDefault := string(cfg.Engine)
	if engineDefault == "" {
		engineDefault = "sqlite"
	}
	builder.Select("engine", "Engine", engines).
		Default(engineDefault).
		Done()

	builder.Text("dsn", "DSN").
		Value(cfg.DSN).
		Placeholder("postgres://user:pass@host:5432/db").
		Done()

	builder.Text("path", "Path").
		Value(cfg.Path).
		Placeholder("path/to/database.db").
		Done()

	builder.Text("url", "URL").
		Value(cfg.URL).
		Done()

	builder.Text("database", "Database").
		Value(cfg.Database).
		Done()

	builder.Text("namespace", "Namespace").
		Value(cfg.Namespace).
		Done()

	builder.Text("username", "Username").
		Value(cfg.Auth.Username).
		Done()

	builder.Text("password", "Password").
		Value(cfg.Auth.Password).
		Done()

	builder.OnSubmit(func(values map[string]any) {
		profileName := name
		if !isEdit {
			profileName = values["name"].(string)
		}
		if profileName == "" {
			return
		}

		newCfg := config.ConnectionConfig{
			Engine:    config.EngineType(values["engine"].(string)),
			DSN:       values["dsn"].(string),
			Path:      values["path"].(string),
			URL:       values["url"].(string),
			Database:  values["database"].(string),
			Namespace: values["namespace"].(string),
			Auth: config.AuthConfig{
				Username: values["username"].(string),
				Password: values["password"].(string),
			},
			SavedQueries: cfg.SavedQueries,
			Commands:     cfg.Commands,
		}

		a.Config().SaveProfile(profileName, newCfg)
		go a.Config().Save()
		a.app.Pages().Pop()
		a.ShowSuccess("Profile saved: " + profileName)
	})

	builder.OnCancel(func() {
		a.app.Pages().Pop()
	})

	form := builder.Build()

	title := "New Profile"
	if isEdit {
		title = "Edit Profile: " + name
	}

	modal := components.NewModal(components.ModalConfig{
		Title:    title,
		Width:    60,
		Height:   22,
		Backdrop: true,
	})
	modal.SetContent(form)
	modal.SetHints([]components.KeyHint{
		{Key: "Tab", Description: "Next field"},
		{Key: "Ctrl+S", Description: "Save"},
		{Key: "Esc", Description: "Cancel"},
	})

	a.app.Pages().Push(modal)
	a.app.SetFocus(form)
}
