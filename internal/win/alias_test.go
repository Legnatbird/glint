package win

import "testing"

func TestExecutionAliasForTerminal(t *testing.T) {
	if alias := ExecutionAlias("Microsoft.WindowsTerminal_8wekyb3d8bbwe!App"); alias != "wt.exe" {
		t.Fatalf("alias = %q, want wt.exe", alias)
	}
}
