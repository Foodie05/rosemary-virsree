package service

import "testing"

func TestNormalizeEndpointAcceptsCommonHumanInput(t *testing.T) {
	tests := map[string]string{
		".s3.bitiful.net":         "https://s3.bitiful.net",
		"s3.bitiful.net/":         "https://s3.bitiful.net",
		"//s3.bitiful.net":        "https://s3.bitiful.net",
		"https//s3.bitiful.net":   "https://s3.bitiful.net",
		"https:/s3.bitiful.net/":  "https://s3.bitiful.net",
		"https://.s3.bitiful.net": "https://s3.bitiful.net",
		" http://localhost:9000 ": "http://localhost:9000",
	}
	for input, want := range tests {
		got, err := normalizeEndpoint(input, false)
		if err != nil || got != want {
			t.Errorf("normalizeEndpoint(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
}

func TestNormalizeEndpointRejectsUnsafeOrAmbiguousInput(t *testing.T) {
	for _, input := range []string{"", "ftp://example.com", "https://user:pass@example.com", "https://example.com?secret=value"} {
		if _, err := normalizeEndpoint(input, false); err == nil {
			t.Errorf("normalizeEndpoint(%q) unexpectedly succeeded", input)
		}
	}
	if got, err := normalizeEndpoint("", true); err != nil || got != "" {
		t.Fatalf("optional empty endpoint = %q, %v", got, err)
	}
}
