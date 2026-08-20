package verdict

import "testing"

func TestWorse(t *testing.T) {
	cases := []struct{ a, b, want string }{
		{"", "", ""},
		{"", Passed, Passed},
		{Passed, "", Passed},
		{Passed, Warning, Warning},
		{Warning, Passed, Warning},
		{Warning, Failed, Failed},
		{Failed, Passed, Failed},
	}
	for _, c := range cases {
		if got := Worse(c.a, c.b); got != c.want {
			t.Errorf("Worse(%q,%q) = %q, want %q", c.a, c.b, got, c.want)
		}
	}
}

func TestToStatus(t *testing.T) {
	cases := []struct{ v, want string }{
		{Passed, StatusPassed}, {Warning, StatusWarning}, {Failed, StatusFailed},
		{"", StatusRunning}, {"bogus", StatusRunning},
	}
	for _, c := range cases {
		if got := ToStatus(c.v); got != c.want {
			t.Errorf("ToStatus(%q) = %q, want %q", c.v, got, c.want)
		}
	}
}
