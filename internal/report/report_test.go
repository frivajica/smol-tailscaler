package report

import "testing"

func TestYesNo(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "no"},
		{"  ", "no"},
		{"false", "no"},
		{"False", "no"},
		{"true", "yes"},
		{"9/15/2026", "yes"},
	}
	for _, tc := range cases {
		if got := yesNo(tc.in); got != tc.want {
			t.Errorf("yesNo(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
