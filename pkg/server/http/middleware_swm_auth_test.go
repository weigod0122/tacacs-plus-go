package http

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestVerifySwmSignatureConsumesNonceOnlyAfterValidMAC(t *testing.T) {
	nonceStore = sync.Map{}
	nonceCount.Store(0)
	secret := "test-secret"
	nonce := "nonce-valid"
	r := signedRequestForTest(t, secret, nonce, []byte(`{"ok":true}`), true)
	if err := verifySwmSignature(r, secret, 300); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if nonceCount.Load() != 1 {
		t.Fatalf("nonce count = %d, want 1", nonceCount.Load())
	}

	replay := signedRequestForTest(t, secret, nonce, []byte(`{"ok":true}`), true)
	if err := verifySwmSignature(replay, secret, 300); err != errReplayedNonce {
		t.Fatalf("replay error = %v, want %v", err, errReplayedNonce)
	}

	bad := signedRequestForTest(t, secret, "nonce-invalid", []byte(`{"ok":true}`), false)
	if err := verifySwmSignature(bad, secret, 300); err != errBadSignature {
		t.Fatalf("invalid signature error = %v, want %v", err, errBadSignature)
	}
	if nonceCount.Load() != 1 {
		t.Fatalf("invalid signature consumed nonce, count = %d", nonceCount.Load())
	}
}

func TestVerifySwmSignatureRejectsOversizedNonce(t *testing.T) {
	nonceStore = sync.Map{}
	nonceCount.Store(0)
	r := signedRequestForTest(t, "test-secret", strings.Repeat("n", maxNonceLength+1), nil, true)
	if err := verifySwmSignature(r, "test-secret", 300); err != errBadHeaderFormat {
		t.Fatalf("oversized nonce error = %v, want %v", err, errBadHeaderFormat)
	}
	if nonceCount.Load() != 0 {
		t.Fatalf("oversized nonce changed store count: %d", nonceCount.Load())
	}
}

func TestNonceTTLTracksConfiguredClockSkew(t *testing.T) {
	if got := nonceTTLForSkew(300); got != nonceTTL {
		t.Fatalf("default nonce TTL = %v, want %v", got, nonceTTL)
	}
	if got := nonceTTLForSkew(900); got <= 900*time.Second {
		t.Fatalf("nonce TTL = %v, want more than configured skew", got)
	}
}

func signedRequestForTest(t *testing.T, secret, nonce string, body []byte, valid bool) *http.Request {
	t.Helper()
	ts := fmt.Sprint(time.Now().Unix())
	r := httptest.NewRequest(http.MethodPost, "/tacacs/approval/create", bytes.NewReader(body))
	r.Header.Set("X-SwM-User", "alice")
	r.Header.Set("X-SwM-Is-Admin", "0")
	hash := sha256.Sum256(body)
	canonical := strings.Join([]string{r.Method, r.URL.Path, ts, nonce, "alice", "0", hex.EncodeToString(hash[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	sig := mac.Sum(nil)
	if !valid {
		sig[0] ^= 0xff
	}
	r.Header.Set("X-SwM-Signature", "t="+ts+",n="+nonce+",v1="+base64.StdEncoding.EncodeToString(sig))
	return r
}
