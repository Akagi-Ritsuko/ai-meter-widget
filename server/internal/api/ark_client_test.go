package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	arkTestAK      = "test-ak-1234567890"
	arkTestSK      = "test-sk-0987654321"
	arkTestRegion  = "cn-beijing"
	arkTestPayload = "{}"
)

var arkFixedNow = time.Date(2026, 8, 27, 10, 30, 0, 0, time.UTC)

type arkReqCapture struct {
	Method  string
	Path    string
	Host    string
	Body    string
	Headers http.Header
}

func arkTestServer(t *testing.T, status int, body string) (*httptest.Server, *arkReqCapture) {
	t.Helper()
	var mu sync.Mutex
	var cap arkReqCapture
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		cap = arkReqCapture{
			Method:  r.Method,
			Path:    r.URL.RequestURI(),
			Host:    r.Host,
			Body:    string(b),
			Headers: r.Header.Clone(),
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(ts.Close)
	return ts, &cap
}

func newArkTestClient(t *testing.T, baseURL string) (*ArkClient, *bytes.Buffer) {
	t.Helper()
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	c := NewArkClient(arkTestAK, arkTestSK, logger,
		WithArkBaseURL(baseURL),
		WithArkClock(func() time.Time { return arkFixedNow }),
	)
	return c, &logBuf
}

// verifyArkV4Signature independently recomputes the Volcengine V4 signature
// from the captured request and compares it against the Authorization header.
func verifyArkV4Signature(cap *arkReqCapture, accessKey, secretKey, region string, now time.Time) error {
	amzDate := now.UTC().Format("20060102T150405Z")
	dateStamp := now.UTC().Format("20060102")

	if cap.Method != http.MethodPost {
		return fmt.Errorf("method = %s, want POST", cap.Method)
	}
	if !strings.HasPrefix(cap.Path, "/?Action=GetAFPUsage&Version=2024-01-01") {
		return fmt.Errorf("path/query = %q", cap.Path)
	}
	if got := cap.Headers.Get("X-Date"); got != amzDate {
		return fmt.Errorf("X-Date = %q, want %q", got, amzDate)
	}

	host := cap.Host
	if h, _, err := net.SplitHostPort(cap.Host); err == nil {
		host = h
	}

	payloadHash := hexSHA256([]byte(cap.Body))
	if got := cap.Headers.Get("X-Content-Sha256"); got != payloadHash {
		return fmt.Errorf("X-Content-Sha256 = %q, want %q", got, payloadHash)
	}

	var cr strings.Builder
	cr.WriteString("POST\n")
	cr.WriteString("/\n")
	cr.WriteString("Action=GetAFPUsage&Version=2024-01-01\n")
	cr.WriteString("content-type:application/json\n")
	cr.WriteString("host:" + host + "\n")
	cr.WriteString("x-content-sha256:" + payloadHash + "\n")
	cr.WriteString("x-date:" + amzDate + "\n")
	cr.WriteString("\n")
	cr.WriteString("content-type;host;x-content-sha256;x-date\n")
	cr.WriteString(payloadHash)

	sts := "HMAC-SHA256\n" + amzDate + "\n" + dateStamp + "/" + region + "/ark/request\n" + hexSHA256([]byte(cr.String()))

	kDate := hmacSHA256Sum([]byte("AWS4"+secretKey), []byte(dateStamp))
	kRegion := hmacSHA256Sum(kDate, []byte(region))
	kService := hmacSHA256Sum(kRegion, []byte("ark"))
	kSigning := hmacSHA256Sum(kService, []byte("request"))
	wantSig := hex.EncodeToString(hmacSHA256Sum(kSigning, []byte(sts)))

	auth := cap.Headers.Get("Authorization")
	if !strings.HasPrefix(auth, "HMAC-SHA256 Credential="+accessKey+"/"+dateStamp+"/"+region+"/ark/request,") {
		return fmt.Errorf("Authorization credential mismatch: %s", auth)
	}
	if !strings.Contains(auth, "SignedHeaders=content-type;host;x-content-sha256;x-date") {
		return fmt.Errorf("Authorization signed headers mismatch: %s", auth)
	}
	idx := strings.LastIndex(auth, "Signature=")
	if idx < 0 {
		return fmt.Errorf("Authorization missing Signature: %s", auth)
	}
	gotSig := strings.TrimSpace(auth[idx+len("Signature="):])
	if gotSig != wantSig {
		return fmt.Errorf("Signature = %q, want %q", gotSig, wantSig)
	}
	return nil
}

func hmacSHA256Sum(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func hexSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func arkFetchAndVerify(t *testing.T, status int, body string, fn func(snap *ArkSnapshot, err error)) {
	t.Helper()
	ts, cap := arkTestServer(t, status, body)
	c, _ := newArkTestClient(t, ts.URL)
	snap, err := c.FetchUsage(t.Context())
	if err == nil {
		if verr := verifyArkV4Signature(cap, arkTestAK, arkTestSK, arkTestRegion, arkFixedNow); verr != nil {
			t.Errorf("signature verification failed: %v", verr)
		}
	}
	fn(snap, err)
}

func TestArkFetchUsageOKStringQuota(t *testing.T) {
	body := `{"ResponseMetadata":{"RequestId":"r1"},"Result":{"PlanType":"agent",` +
		`"AFPFiveHour":{"Quota":"500","Used":"120","SubscribeTime":1720000000000,"ResetTime":1720080000000},` +
		`"AFPDaily":{"Quota":"1000","Used":"400","SubscribeTime":1720000000000,"ResetTime":1720080000000},` +
		`"AFPWeekly":{"Quota":"7000","Used":"2100","SubscribeTime":1720000000000,"ResetTime":1720080000000},` +
		`"AFPMonthly":{"Quota":"30000","Used":"9000","SubscribeTime":1720000000000,"ResetTime":1720080000000}}}`

	arkFetchAndVerify(t, http.StatusOK, body, func(snap *ArkSnapshot, err error) {
		if err != nil {
			t.Fatalf("FetchUsage: %v", err)
		}
		if snap.PlanType != "agent" {
			t.Errorf("PlanType = %q", snap.PlanType)
		}
		if len(snap.Windows) != 4 {
			t.Fatalf("windows = %d, want 4", len(snap.Windows))
		}
		if snap.Windows[0].Name != "five_hour" || snap.Windows[0].Quota != 500 || snap.Windows[0].Used != 120 {
			t.Errorf("five_hour = %+v", snap.Windows[0])
		}
		if snap.RawJSON == "" {
			t.Error("RawJSON must be populated")
		}
	})
}

func TestArkFetchUsageOKNumberQuota(t *testing.T) {
	body := `{"ResponseMetadata":{},"Result":{"PlanType":"agent",` +
		`"AFPDaily":{"Quota":1000,"Used":250,"SubscribeTime":1720000000000,"ResetTime":1720080000000},` +
		`"AFPFiveHour":{"Quota":500,"Used":50,"SubscribeTime":1720000000000,"ResetTime":1720080000000},` +
		`"AFPWeekly":{"Quota":7000,"Used":0,"SubscribeTime":1720000000000,"ResetTime":1720080000000},` +
		`"AFPMonthly":{"Quota":30000,"Used":15000,"SubscribeTime":1720000000000,"ResetTime":1720080000000}}}`

	arkFetchAndVerify(t, http.StatusOK, body, func(snap *ArkSnapshot, err error) {
		if err != nil {
			t.Fatalf("FetchUsage: %v", err)
		}
		daily := snap.Windows[1]
		if daily.Name != "daily" || daily.Quota != 1000 || daily.Used != 250 || daily.Percent != 25 {
			t.Errorf("daily = %+v", daily)
		}
	})
}

func TestArkFetchUsageResponseMetadataError(t *testing.T) {
	body := `{"ResponseMetadata":{"RequestId":"x","Error":{"Code":"SignatureDoesNotMatch","Message":"signature mismatch"}},"Result":{}}`

	arkFetchAndVerify(t, http.StatusOK, body, func(_ *ArkSnapshot, err error) {
		if !errors.Is(err, ErrArkUnauthorized) {
			t.Fatalf("err = %v, want ErrArkUnauthorized", err)
		}
	})
}

func TestArkFetchUsageResponseMetadataAPIError(t *testing.T) {
	body := `{"ResponseMetadata":{"RequestId":"x","Error":{"Code":"UnknownError","Message":"boom"}},"Result":{}}`

	arkFetchAndVerify(t, http.StatusOK, body, func(_ *ArkSnapshot, err error) {
		if !errors.Is(err, ErrArkAPIError) {
			t.Fatalf("err = %v, want ErrArkAPIError", err)
		}
	})
}

func TestArkFetchUsageHTTPStatusErrors(t *testing.T) {
	cases := []struct {
		status int
		want   error
	}{
		{http.StatusUnauthorized, ErrArkUnauthorized},
		{http.StatusForbidden, ErrArkUnauthorized},
		{http.StatusTooManyRequests, ErrArkRateLimited},
		{http.StatusInternalServerError, ErrArkServerError},
		{http.StatusBadGateway, ErrArkServerError},
		{http.StatusTeapot, ErrArkInvalidResponse},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("status_%d", tc.status), func(t *testing.T) {
			arkFetchAndVerify(t, tc.status, `{"error":"x"}`, func(_ *ArkSnapshot, err error) {
				if !errors.Is(err, tc.want) {
					t.Fatalf("err = %v, want %v", err, tc.want)
				}
			})
		})
	}
}

func TestArkFetchUsageMalformedJSON(t *testing.T) {
	arkFetchAndVerify(t, http.StatusOK, "{not-json", func(_ *ArkSnapshot, err error) {
		if !errors.Is(err, ErrArkInvalidResponse) {
			t.Fatalf("err = %v, want ErrArkInvalidResponse", err)
		}
	})
}

func TestArkFetchUsageEmptyBody(t *testing.T) {
	arkFetchAndVerify(t, http.StatusOK, "", func(_ *ArkSnapshot, err error) {
		if !errors.Is(err, ErrArkInvalidResponse) {
			t.Fatalf("err = %v, want ErrArkInvalidResponse", err)
		}
	})
}

func TestArkFetchUsageBodyOverCap(t *testing.T) {
	oversized := strings.Repeat("a", 1<<20+1)
	arkFetchAndVerify(t, http.StatusOK, oversized, func(_ *ArkSnapshot, err error) {
		if !errors.Is(err, ErrArkInvalidResponse) {
			t.Fatalf("err = %v, want ErrArkInvalidResponse", err)
		}
	})
}

func TestArkFetchUsageNetworkError(t *testing.T) {
	ts, _ := arkTestServer(t, http.StatusOK, "{}")
	url := ts.URL
	ts.Close() // force network error

	c, _ := newArkTestClient(t, url)
	_, err := c.FetchUsage(t.Context())
	if !errors.Is(err, ErrArkNetworkError) {
		t.Fatalf("err = %v, want network error", err)
	}
}

func TestArkClientLogsRedactedAccessKey(t *testing.T) {
	body := `{"ResponseMetadata":{},"Result":{"PlanType":"agent",` +
		`"AFPDaily":{"Quota":1000,"Used":0},"AFPFiveHour":{"Quota":500,"Used":0},` +
		`"AFPWeekly":{"Quota":7000,"Used":0},"AFPMonthly":{"Quota":30000,"Used":0}}}`

	ts, _ := arkTestServer(t, http.StatusOK, body)
	c, logBuf := newArkTestClient(t, ts.URL)
	if _, err := c.FetchUsage(t.Context()); err != nil {
		t.Fatalf("FetchUsage: %v", err)
	}
	logged := logBuf.String()
	if strings.Contains(logged, arkTestAK) {
		t.Fatalf("log leaks plaintext access key")
	}
	if !strings.Contains(logged, "test***...***890") {
		t.Fatalf("log missing redacted access key, got: %s", logged)
	}
	if strings.Contains(logged, arkTestSK) {
		t.Fatalf("log leaks secret key")
	}
}