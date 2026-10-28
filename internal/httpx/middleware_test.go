package httpx

import "testing"

func TestRateLimiterBlocksFlood(t *testing.T) {
	limiter := NewRateLimiter(1, 3)
	for i := 0; i < 3; i++ {
		if !limiter.Allow("client") {
			t.Fatalf("第 %d 次请求应被允许", i+1)
		}
	}
	if limiter.Allow("client") {
		t.Fatal("超出额度后应被拒绝")
	}
	if !limiter.Allow("another-client") {
		t.Fatal("不同客户端应拥有独立的额度")
	}
}

func TestRateLimiterRefillsOverTime(t *testing.T) {
	limiter := NewRateLimiter(100, 1)
	if !limiter.Allow("client") {
		t.Fatal("第一次请求应被允许")
	}
	if limiter.Allow("client") {
		t.Fatal("额度用尽后应被拒绝")
	}
}

func TestPathTokenStripsTraversal(t *testing.T) {
	cases := map[string]string{
		"file.txt":           "file.txt",
		"../../etc/passwd":   "etcpasswd",
		`..\..\windows\.ini`: "windows.ini",
		"a/b/c.png":          "abc.png",
		"  spaced.png  ":     "spaced.png",
	}
	for input, expected := range cases {
		if got := PathToken(input); got != expected {
			t.Fatalf("PathToken(%q) = %q，期望 %q", input, got, expected)
		}
	}
}

func TestParseIntAndClamp(t *testing.T) {
	if ParseInt("", 7) != 7 {
		t.Fatal("空值应返回默认值")
	}
	if ParseInt("abc", 7) != 7 {
		t.Fatal("非法值应返回默认值")
	}
	if ParseInt("42", 7) != 42 {
		t.Fatal("合法值应被解析")
	}
	if Clamp(5, 1, 10) != 5 {
		t.Fatal("区间内的值不应改变")
	}
	if Clamp(0, 1, 10) != 1 {
		t.Fatal("低于下限应被截断")
	}
	if Clamp(99, 1, 10) != 10 {
		t.Fatal("高于上限应被截断")
	}
}
