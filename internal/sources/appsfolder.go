package sources

import (
	"strings"

	"applauncher/internal/core"
	"applauncher/internal/win"
)

// scanAppsFolder returns Store, MSIX and other packaged applications.
//
// They are the ones that look "missing" from a naive Start Menu scan: Edge,
// Windows Terminal, Settings, Calculator and anything installed from the Store.
func scanAppsFolder(rep *Report) []core.Entry {
	apps, err := win.EnumerateAppsFolder()
	if err != nil {
		rep.warn("apps folder: " + err.Error())
		return nil
	}

	out := make([]core.Entry, 0, len(apps))
	for _, app := range apps {
		out = append(out, core.Entry{
			Key:   "uwp:" + strings.ToLower(app.AUMID),
			Name:  app.Name,
			AUMID: app.AUMID,
			// The shell resolves an icon for this parsing name, which is how a
			// packaged app gets its real icon without a COM image factory.
			IconPath: "aumid:" + strings.ToLower(app.AUMID),
			Source:   core.SourceStartMenu,
		})
	}
	return out
}
