package llmadmit

import "testing"

func TestPrimaryDomain(t *testing.T) {
	cases := map[string]string{
		"https://www.yropc.com/v1":            "yropc.com",
		"https://api.modelverse.cn/v1":        "modelverse.cn",
		"https://cn.liuneng.cn/v1":            "liuneng.cn",
		"https://www.joyhill-ai.com/v1":       "joyhill-ai.com",
		"https://api.edgecloudapp.com/v2/llm": "edgecloudapp.com",
		"http://conreix.com/v1":               "conreix.com",
		"http://180.178.51.102:3000/v1":       "180.178.51.102:3000",
		"http://localhost:8080/v1":            "localhost:8080",
		"not-a-url":                           "not-a-url",
	}
	for in, want := range cases {
		if got := primaryDomain(in); got != want {
			t.Errorf("primaryDomain(%q)=%q want %q", in, got, want)
		}
	}
}
