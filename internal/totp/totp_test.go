package totp

import (
	"net/url"
	"testing"
	"time"
)

// RFC 4648 base32 test secret ("Hello!" padded), valid for the library.
const testSecret = "JBSWY3DPEHPK3PXP"

func TestVerifyWithCounterAcceptsCurrentCodeAndReturnsStep(t *testing.T) {
	code, err := Generate(testSecret)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	step, ok := VerifyWithCounter(testSecret, code)
	if !ok {
		t.Fatal("VerifyWithCounter rejected a freshly generated code")
	}
	if want := time.Now().Unix() / period; step != want {
		t.Fatalf("matched step = %d, want current step %d", step, want)
	}
}

func TestVerifyWithCounterRejectsAlteredCode(t *testing.T) {
	code, err := Generate(testSecret)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Change the first digit so the candidate is guaranteed to differ.
	altered := string('0'+(code[0]-'0'+1)%10) + code[1:]
	if _, ok := VerifyWithCounter(testSecret, altered); ok {
		t.Fatalf("VerifyWithCounter accepted an altered code %q (from %q)", altered, code)
	}
}

func TestBuildURIEncodesOtpauthParameters(t *testing.T) {
	uri := BuildURI("Go Spark", "user@example.com", testSecret)

	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatalf("BuildURI produced an unparseable URI %q: %v", uri, err)
	}
	if parsed.Scheme != "otpauth" || parsed.Host != "totp" {
		t.Fatalf("scheme/host = %q/%q, want otpauth/totp", parsed.Scheme, parsed.Host)
	}

	q := parsed.Query()
	for key, want := range map[string]string{
		"secret":    testSecret,
		"issuer":    "Go Spark",
		"algorithm": "SHA1",
		"digits":    "6",
		"period":    "30",
	} {
		if got := q.Get(key); got != want {
			t.Fatalf("query %q = %q, want %q", key, got, want)
		}
	}
}
