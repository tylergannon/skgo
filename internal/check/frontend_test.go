package check

import (
	"strings"
	"testing"
)

func TestSvelteCompletionRequiresSummaryAndCount(t *testing.T) {
	root := t.TempDir()
	web := root + "/web"
	good := "100 START \"" + web + "\"\n" +
		"101 {\"type\":\"ERROR\",\"filename\":\"src/App.svelte\",\"start\":{\"line\":1,\"character\":6},\"end\":{\"line\":1,\"character\":11},\"message\":\"wrong prop\",\"code\":2322,\"source\":\"ts\"}\n" +
		"102 COMPLETED 2 FILES 1 ERRORS 0 WARNINGS 1 FILES_WITH_PROBLEMS\n"
	for _, tc := range []struct {
		name, input string
		code        int
		status      string
	}{
		{"complete", good, 1, "complete"},
		{"zero_exit_failure", "svelte-check failed\n", 0, "incomplete"},
		{"truncated", strings.TrimSuffix(good, "102 COMPLETED 2 FILES 1 ERRORS 0 WARNINGS 1 FILES_WITH_PROBLEMS\n"), 0, "incomplete"},
		{"count_mismatch", strings.Replace(good, "1 ERRORS", "2 ERRORS", 1), 1, "incomplete"},
		{"zero_files", strings.Replace(good, "2 FILES", "0 FILES", 1), 1, "incomplete"},
		{"zero_exit_errors", good, 0, "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, ds := parseSvelte(root, web, []byte(tc.input), tc.code)
			if c.Status != tc.status {
				t.Fatalf("status=%s, want %s: %s", c.Status, tc.status, c.Message)
			}
			if tc.name == "complete" && (len(ds) != 1 || ds[0].Location == nil || ds[0].Location.File != "web/src/App.svelte" || ds[0].Location.Line != 2) {
				t.Fatalf("diagnostics: %+v", ds)
			}
		})
	}
}
