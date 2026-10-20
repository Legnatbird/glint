package sources

import (
	"strings"

	"applauncher/internal/core"
)

// scanManual turns the hand-written entries of config.json into applications.
//
// It exists for everything no source can discover: a game owned by a third
// party launcher, a portable tool, a script. The user provides the name and the
// target, so there is nothing to guess.
func scanManual(rep *Report, apps []core.ManualApp) []core.Entry {
	var out []core.Entry

	for _, app := range apps {
		if !fileExists(app.Target) && !strings.Contains(app.Target, "://") {
			rep.warn("manual entry not found: " + app.Target)
			continue
		}

		icon := app.Icon
		if icon == "" {
			icon = app.Target
		}

		out = append(out, core.Entry{
			Key:      "manual:" + strings.ToLower(app.Target),
			Name:     app.Name,
			Target:   app.Target,
			Resolved: app.Target,
			Args:     app.Args,
			IconPath: icon,
			Source:   core.SourceCustomFolder,
		})
	}
	return out
}
