package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCloudflareAccessValidatorAcceptsValidToken(t *testing.T) {
	fixture := newAccessFixture(t)
	validator := CloudflareAccessValidator{
		TeamDomain: fixture.teamDomain,
		Audience:   "admin-aud",
		CertsURL:   fixture.server.URL,
		HTTPClient: fixture.server.Client(),
		Now:        fixture.nowFunc(),
	}
	t.Cleanup(fixture.server.Close)

	token := fixture.token(t, map[string]any{
		"iss":   fixture.teamDomain,
		"aud":   []string{"admin-aud"},
		"exp":   fixture.now.Add(time.Hour).Unix(),
		"nbf":   fixture.now.Add(-time.Minute).Unix(),
		"iat":   fixture.now.Add(-time.Minute).Unix(),
		"email": "admin@example.com",
		"sub":   "user-123",
	})

	claims, err := validator.ValidateToken(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Email != "admin@example.com" || claims.Subject != "user-123" {
		t.Fatalf("unexpected claims: %#v", claims)
	}
}

func TestCloudflareAccessValidatorRejectsWrongAudience(t *testing.T) {
	fixture := newAccessFixture(t)
	validator := CloudflareAccessValidator{
		TeamDomain: fixture.teamDomain,
		Audience:   "admin-aud",
		CertsURL:   fixture.server.URL,
		HTTPClient: fixture.server.Client(),
		Now:        fixture.nowFunc(),
	}
	t.Cleanup(fixture.server.Close)

	token := fixture.token(t, map[string]any{
		"iss": fixture.teamDomain,
		"aud": []string{"other-aud"},
		"exp": fixture.now.Add(time.Hour).Unix(),
		"nbf": fixture.now.Add(-time.Minute).Unix(),
		"iat": fixture.now.Add(-time.Minute).Unix(),
	})

	if _, err := validator.ValidateToken(context.Background(), token); err == nil {
		t.Fatal("expected wrong audience to be rejected")
	}
}

func TestCloudflareAccessValidatorRejectsTamperedToken(t *testing.T) {
	fixture := newAccessFixture(t)
	validator := CloudflareAccessValidator{
		TeamDomain: fixture.teamDomain,
		Audience:   "admin-aud",
		CertsURL:   fixture.server.URL,
		HTTPClient: fixture.server.Client(),
		Now:        fixture.nowFunc(),
	}
	t.Cleanup(fixture.server.Close)

	token := fixture.token(t, map[string]any{
		"iss": fixture.teamDomain,
		"aud": []string{"admin-aud"},
		"exp": fixture.now.Add(time.Hour).Unix(),
		"nbf": fixture.now.Add(-time.Minute).Unix(),
		"iat": fixture.now.Add(-time.Minute).Unix(),
	})
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("bad fixture token %q", token)
	}
	parts[1] = encodeJWTPart(map[string]any{
		"iss": fixture.teamDomain,
		"aud": []string{"admin-aud"},
		"exp": fixture.now.Add(2 * time.Hour).Unix(),
		"nbf": fixture.now.Add(-time.Minute).Unix(),
		"iat": fixture.now.Add(-time.Minute).Unix(),
	})

	if _, err := validator.ValidateToken(context.Background(), strings.Join(parts, ".")); err == nil {
		t.Fatal("expected tampered token to be rejected")
	}
}

func TestCloudflareAccessValidatorAuthorizesAdminRequestFromAccessHeader(t *testing.T) {
	fixture := newAccessFixture(t)
	validator := CloudflareAccessValidator{
		TeamDomain: fixture.teamDomain,
		Audience:   "admin-aud",
		CertsURL:   fixture.server.URL,
		HTTPClient: fixture.server.Client(),
		Now:        fixture.nowFunc(),
	}
	t.Cleanup(fixture.server.Close)
	token := fixture.token(t, map[string]any{
		"iss": fixture.teamDomain,
		"aud": []string{"admin-aud"},
		"exp": fixture.now.Add(time.Hour).Unix(),
		"nbf": fixture.now.Add(-time.Minute).Unix(),
		"iat": fixture.now.Add(-time.Minute).Unix(),
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/reviews", nil)
	req.Header.Set("Cf-Access-Jwt-Assertion", token)

	if err := validator.AuthorizeAdmin(context.Background(), req); err != nil {
		t.Fatal(err)
	}
}

func TestCloudflareAccessValidatorRejectsAdminRequestWithoutAccessHeader(t *testing.T) {
	validator := CloudflareAccessValidator{TeamDomain: "https://team.cloudflareaccess.com", Audience: "admin-aud"}
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/reviews", nil)

	if err := validator.AuthorizeAdmin(context.Background(), req); err == nil {
		t.Fatal("expected missing Access header to be rejected")
	}
}

type accessFixture struct {
	key        *rsa.PrivateKey
	kid        string
	teamDomain string
	now        time.Time
	server     *httptest.Server
}

func newAccessFixture(t *testing.T) accessFixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	fixture := accessFixture{
		key:        key,
		kid:        "test-key",
		teamDomain: "https://team.cloudflareaccess.com",
		now:        time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC),
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kty": "RSA",
				"kid": fixture.kid,
				"alg": "RS256",
				"use": "sig",
				"n":   base64.RawURLEncoding.EncodeToString(fixture.key.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(fixture.key.E)).Bytes()),
			}},
		})
	}))
	return fixture
}

func (f accessFixture) nowFunc() func() time.Time {
	return func() time.Time { return f.now }
}

func (f accessFixture) token(t *testing.T, payload map[string]any) string {
	t.Helper()
	header := encodeJWTPart(map[string]any{
		"alg": "RS256",
		"kid": f.kid,
		"typ": "JWT",
	})
	body := encodeJWTPart(payload)
	signingInput := header + "." + body
	sum := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, f.key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func encodeJWTPart(value map[string]any) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(data)
}
