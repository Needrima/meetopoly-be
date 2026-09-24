package auth

import "testing"

func TestCapitalizeUsername(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"ademola", "Ademola"},
		{"Ademola", "Ademola"},
		{"ADEMOLA", "Ademola"},
		{"john_doe", "John_Doe"},
		{"", ""},
	}
	for _, c := range cases {
		if got := capitalizeUsername(c.in); got != c.want {
			t.Fatalf("%q → %q want %q", c.in, got, c.want)
		}
	}
}
