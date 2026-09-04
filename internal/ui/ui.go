package ui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/legibet/sbxctl/internal/config"
	"github.com/legibet/sbxctl/internal/sbx"
)

func Run(ep sbx.Endpoint, name string, file *config.File, noColor bool) error {
	var session *sbx.Session
	var err error
	if ep.URL != "" {
		session, err = sbx.NewSession(ep)
		if err != nil {
			return err
		}
	}
	model := newApp(ep, name, file, session)
	options := []tea.ProgramOption{}
	if noColor {
		options = append(options, tea.WithColorProfile(colorprofile.Ascii))
	}
	finalModel, err := tea.NewProgram(model, options...).Run()
	if final, ok := finalModel.(app); ok && final.session != nil {
		final.session.Close()
	} else if session != nil {
		session.Close()
	}
	return err
}
