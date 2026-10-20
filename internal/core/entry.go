package core

import "strings"

// Source says where an entry came from. When two sources describe the same
// application, the lower value wins.
type Source uint8

const (
	SourceStartMenu Source = iota
	SourceCustomFolder
	SourceAppPaths
	SourcePath
)

// Priority is the deduplication order. Start Menu entries carry the name and
// icon users recognize, so they beat anything discovered later.
func (s Source) Priority() int { return int(s) }

// Kind is how an entry is launched. It is derived from the entry, never stored,
// so an entry cannot claim to be two kinds at once.
type Kind uint8

const (
	KindExecutable Kind = iota
	KindShortcut
	KindUWP
)

// Entry is one launchable application.
type Entry struct {
	Key      string `json:"key"`               // canonical identity, also the history key
	Name     string `json:"name"`              // display name
	Target   string `json:"target,omitempty"`  // .lnk or .exe path
	AUMID    string `json:"aumid,omitempty"`   // AppUserModelID for Store apps
	Args     string `json:"args,omitempty"`    // forwarded arguments
	WorkDir  string `json:"workdir,omitempty"` // working directory
	IconPath string `json:"icon,omitempty"`    // icon source, may be the target
	Source   Source `json:"source"`
}

// Kind reports how the entry must be launched.
func (e Entry) Kind() Kind {
	switch {
	case e.AUMID != "":
		return KindUWP
	case strings.HasSuffix(strings.ToLower(e.Target), ".lnk"):
		return KindShortcut
	default:
		return KindExecutable
	}
}
