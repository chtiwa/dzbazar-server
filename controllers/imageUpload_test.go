package controllers

import "testing"

func TestSniffImageType(t *testing.T) {
	pad := make([]byte, 20)
	cases := []struct {
		name string
		in   []byte
		want string
		ok   bool
	}{
		{"png", append([]byte("\x89PNG\r\n\x1a\n"), pad...), "image/png", true},
		{"jpeg", append([]byte("\xff\xd8\xff\xe0"), pad...), "image/jpeg", true},
		{"html", []byte("<html><body>x</body></html>"), "", false},
		{"svg", []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>"), "", false},
	}
	for _, c := range cases {
		got, ok := sniffImageType(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: got (%q,%v) want (%q,%v)", c.name, got, ok, c.want, c.ok)
		}
	}
}
