package win

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

// ExecutionAlias returns the command name of a packaged app's execution alias
// (for example "wt.exe"), or "" when the package declares none.
//
// A packaged app cannot be activated elevated through its AppUserModelID:
// IApplicationActivationManager has no notion of elevation. Its execution alias
// can, because the alias is an ordinary command that ShellExecuteEx can run
// with the "runas" verb.
//
// The alias files in WindowsApps are zero bytes, so they cannot be read. Windows
// registers every alias under App Paths instead, and that value points into the
// package folder, which carries the package name and the publisher id.
//
// The family name and the folder name are not directly comparable: the family
// is "Name_PublisherId" and the folder is "Name_Version_Arch__PublisherId", so
// the family is split and both halves are looked for separately.
func ExecutionAlias(aumid string) string {
	family, _, found := strings.Cut(aumid, "!")
	if !found {
		return ""
	}

	cut := strings.LastIndex(family, "_")
	if cut <= 0 {
		return ""
	}
	pkgName := strings.ToLower(family[:cut])
	publisher := strings.ToLower(family[cut+1:])

	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		key, err := registry.OpenKey(root, `SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths`, registry.READ)
		if err != nil {
			continue
		}
		names, err := key.ReadSubKeyNames(-1)
		key.Close()
		if err != nil {
			continue
		}

		for _, name := range names {
			sub, err := registry.OpenKey(root, `SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\`+name, registry.READ)
			if err != nil {
				continue
			}
			target, _, valueErr := sub.GetStringValue("")
			sub.Close()
			if valueErr != nil {
				continue
			}

			lower := strings.ToLower(target)
			if strings.Contains(lower, pkgName) && strings.Contains(lower, publisher) {
				return name
			}
		}
	}
	return ""
}
