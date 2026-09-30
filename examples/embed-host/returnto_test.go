package main

import "testing"

func TestReturnToStaysUnderTheBasePath(t *testing.T) {
	home := basePath + "/"
	cases := map[string]string{
		"":                                  home,
		basePath:                            basePath,
		basePath + "/admin/llms?tab=x#frag": basePath + "/admin/llms?tab=x#frag",
		basePath + "/":                      home,
		"/elsewhere":                        home,
		"//evil.example":                    home,
		"//evil.example" + basePath + "/x":  home,
		"https://evil.example" + basePath:   home,
		basePath + "/../evil":               home,
		basePath + "/..//evil.example":      home,
		basePath + "-other/x":               home,
		"/\\evil.example":                   home,
		basePath + "/x\r\nSet-Cookie: a=b":  home,
		"relative/path":                     home,
	}
	for next, want := range cases {
		if got := returnTo(next); got != want {
			t.Errorf("returnTo(%q) = %q, want %q", next, got, want)
		}
	}
}
