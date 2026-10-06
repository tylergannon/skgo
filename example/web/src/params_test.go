package hooks

import "testing"

func TestOrderMatcherBoundaries(t *testing.T) {
	for _, tc := range []struct {
		input    string
		accepted bool
		label    string
	}{
		{"0", true, "Order #0"}, {"+0", true, "Order #0"}, {"00042", true, "Order #42"},
		{"1000000", true, "Order #1000000"}, {"1000001", false, ""},
		{"-1", false, ""}, {"9223372036854775808", false, ""}, {"banana", false, ""},
	} {
		t.Run(tc.input, func(t *testing.T) {
			value, accepted := Order(tc.input)
			if accepted != tc.accepted {
				t.Fatalf("accepted=%t, want %t", accepted, tc.accepted)
			}
			if accepted && value.Label() != tc.label {
				t.Fatalf("label=%q, want %q", value.Label(), tc.label)
			}
		})
	}
}
