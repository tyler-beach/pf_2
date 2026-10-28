package prscan

import "testing"

func TestParsePRURL(t *testing.T) {
	cases := []struct {
		in      string
		owner   string
		repo    string
		number  int
		wantErr bool
	}{
		{"https://github.com/acme/widgets/pull/42", "acme", "widgets", 42, false},
		{"https://github.com/acme/widgets/pull/42/", "acme", "widgets", 42, false},
		{"github.com/acme/widgets/pull/7", "acme", "widgets", 7, false},
		{"https://api.github.com/repos/acme/widgets/pulls/9", "acme", "widgets", 9, false},
		{"acme/widgets#11", "acme", "widgets", 11, false},
		{"https://github.com/acme/widgets/issues/1", "", "", 0, true},
		{"", "", "", 0, true},
		{"not a url", "", "", 0, true},
		{"https://github.com/acme/wid gets/pull/1", "", "", 0, true},
		{"https://github.com/./widgets/pull/1", "", "", 0, true},
		{"https://github.com/acme/../evil/pull/1", "", "", 0, true},
	}
	for _, tc := range cases {
		got, err := ParsePRURL(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("%q: expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		if got.Owner != tc.owner || got.Repo != tc.repo || got.Number != tc.number {
			t.Fatalf("%q: got %+v", tc.in, got)
		}
		if got.HTMLURL() == "" || got.FullName() != tc.owner+"/"+tc.repo {
			t.Fatalf("%q: helpers failed %+v", tc.in, got)
		}
	}
}
