package releaseversion

import "testing"

func TestAnalyzeConventionalCommits(t *testing.T) {
	for _, tc := range []struct {
		name     string
		messages []string
		want     Level
	}{
		{"feature", []string{"feat(scaffold): add browser tests"}, Minor},
		{"fix", []string{"fix: keep the visible status fresh"}, Patch},
		{"performance", []string{"perf(ssr): reuse a buffer"}, Patch},
		{"breaking marker", []string{"feat(api)!: remove the old constructor"}, Major},
		{"breaking footer", []string{"fix: correct transport\n\nBREAKING CHANGE: old payloads are rejected"}, Major},
		{"largest wins", []string{"fix: one", "feat: two"}, Minor},
		{"non-release type", []string{"docs: explain the origin"}, None},
		{"historical subject", []string{"Fix the thing (#12)", "feat: new behavior"}, Minor},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Analyze(tc.messages); got != tc.want {
				t.Fatalf("Analyze() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNext(t *testing.T) {
	for _, tc := range []struct {
		level Level
		want  string
	}{{Patch, "v0.2.10"}, {Minor, "v0.3.0"}, {Major, "v1.0.0"}, {None, ""}} {
		got, err := Next("v0.2.9", tc.level)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Fatalf("Next(v0.2.9, %v) = %q, want %q", tc.level, got, tc.want)
		}
	}
}

func TestValidateSubject(t *testing.T) {
	for _, subject := range []string{"feat: add it", "fix(remote)!: change it", "ci(release): gate it"} {
		if err := ValidateSubject(subject); err != nil {
			t.Errorf("ValidateSubject(%q): %v", subject, err)
		}
	}
	for _, subject := range []string{"Add it", "feat add it", "feat: "} {
		if err := ValidateSubject(subject); err == nil {
			t.Errorf("ValidateSubject(%q) succeeded", subject)
		}
	}
}
