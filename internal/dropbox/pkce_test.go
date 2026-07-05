package dropbox

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
)

func TestGenerateCodeVerifierIsURLSafeAndUnique(t *testing.T) {
	a, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatalf("GenerateCodeVerifier: %v", err)
	}
	b, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatalf("GenerateCodeVerifier: %v", err)
	}
	if a == b {
		t.Fatal("expected two calls to produce different verifiers")
	}
	if strings.ContainsAny(a, "+/=") {
		t.Fatalf("verifier contains non-URL-safe characters: %q", a)
	}
}

func TestCodeChallengeMatchesS256Spec(t *testing.T) {
	verifier := "test-verifier-value"
	got := CodeChallenge(verifier)

	sum := sha256.Sum256([]byte(verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])

	if got != want {
		t.Fatalf("CodeChallenge(%q) = %q, want %q", verifier, got, want)
	}
}
