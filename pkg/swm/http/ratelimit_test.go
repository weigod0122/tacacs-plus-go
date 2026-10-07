package http

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func resetRateLimitStateForTest() {
	ipBucketsMu.Lock()
	defer ipBucketsMu.Unlock()
	ipBuckets = map[string]*ipBucket{}
}

func TestRequestUsernameRestoresFormBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := "username=alice&password=secret"
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	if got := requestUsername(c); got != "alice" {
		t.Fatalf("username = %q, want alice", got)
	}
	remaining, err := io.ReadAll(c.Request.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(remaining) != body {
		t.Fatalf("body after username inspection = %q, want %q", remaining, body)
	}
}

func TestRequestUsernameDoesNotTruncateLargeBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := "username=alice&notes=" + strings.Repeat("x", 9000)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/create-user", strings.NewReader(body))
	if got := requestUsername(c); got != "" {
		t.Fatalf("large body username = %q, want empty hint", got)
	}
	remaining, err := io.ReadAll(c.Request.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(remaining) != body {
		t.Fatalf("large body was changed by rate limiter: got %d bytes, want %d", len(remaining), len(body))
	}
}

func TestLoginRateLimitUsesAccountScope(t *testing.T) {
	resetRateLimitStateForTest()
	gin.SetMode(gin.TestMode)
	middleware := LoginRateLimit()
	for i := 0; i < loginAttemptMax; i++ {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/create-user", strings.NewReader("username=alice&password=x"))
		middleware(c)
		if recorder.Code != http.StatusOK {
			t.Fatalf("attempt %d status = %d, want 200", i+1, recorder.Code)
		}
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/create-user", strings.NewReader("username=alice&password=x"))
	middleware(c)
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("account-throttled status = %d, want 429", recorder.Code)
	}
}
