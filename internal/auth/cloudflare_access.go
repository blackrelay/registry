package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

const CloudflareAccessJWTHeader = "Cf-Access-Jwt-Assertion"

type CloudflareAccessClaims struct {
	Email   string
	Subject string
	Type    string
}

type CloudflareAccessValidator struct {
	TeamDomain string
	Audience   string
	CertsURL   string
	HTTPClient *http.Client
	Now        func() time.Time

	mu   sync.Mutex
	keys map[string]*rsa.PublicKey
}

func (v *CloudflareAccessValidator) ValidateToken(ctx context.Context, token string) (CloudflareAccessClaims, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return CloudflareAccessClaims{}, errors.New("cloudflare access JWT is required")
	}
	teamDomain := strings.TrimRight(strings.TrimSpace(v.TeamDomain), "/")
	if teamDomain == "" {
		return CloudflareAccessClaims{}, errors.New("cloudflare access team domain is required")
	}
	audience := strings.TrimSpace(v.Audience)
	if audience == "" {
		return CloudflareAccessClaims{}, errors.New("cloudflare access audience is required")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return CloudflareAccessClaims{}, errors.New("cloudflare access JWT must have three parts")
	}
	var header accessJWTHeader
	if err := decodeJWTPart(parts[0], &header); err != nil {
		return CloudflareAccessClaims{}, fmt.Errorf("decode cloudflare access JWT header: %w", err)
	}
	if header.Algorithm != "RS256" {
		return CloudflareAccessClaims{}, fmt.Errorf("unsupported cloudflare access JWT algorithm %q", header.Algorithm)
	}
	if strings.TrimSpace(header.KeyID) == "" {
		return CloudflareAccessClaims{}, errors.New("cloudflare access JWT key id is required")
	}
	key, err := v.publicKey(ctx, header.KeyID)
	if err != nil {
		return CloudflareAccessClaims{}, err
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return CloudflareAccessClaims{}, fmt.Errorf("decode cloudflare access JWT signature: %w", err)
	}
	signingInput := parts[0] + "." + parts[1]
	sum := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], signature); err != nil {
		return CloudflareAccessClaims{}, errors.New("cloudflare access JWT signature is invalid")
	}
	var claims accessJWTPayload
	if err := decodeJWTPart(parts[1], &claims); err != nil {
		return CloudflareAccessClaims{}, fmt.Errorf("decode cloudflare access JWT payload: %w", err)
	}
	if strings.TrimRight(claims.Issuer, "/") != teamDomain {
		return CloudflareAccessClaims{}, errors.New("cloudflare access JWT issuer is invalid")
	}
	if !claims.Audience.Contains(audience) {
		return CloudflareAccessClaims{}, errors.New("cloudflare access JWT audience is invalid")
	}
	now := time.Now().UTC()
	if v.Now != nil {
		now = v.Now().UTC()
	}
	if claims.ExpiresAt == 0 || now.After(time.Unix(claims.ExpiresAt, 0)) {
		return CloudflareAccessClaims{}, errors.New("cloudflare access JWT is expired")
	}
	if claims.NotBefore != 0 && now.Before(time.Unix(claims.NotBefore, 0)) {
		return CloudflareAccessClaims{}, errors.New("cloudflare access JWT is not valid yet")
	}
	return CloudflareAccessClaims{
		Email:   claims.Email,
		Subject: claims.Subject,
		Type:    claims.Type,
	}, nil
}

func (v *CloudflareAccessValidator) AuthorizeAdmin(ctx context.Context, r *http.Request) error {
	if r == nil {
		return errors.New("request is required")
	}
	_, err := v.ValidateToken(ctx, r.Header.Get(CloudflareAccessJWTHeader))
	return err
}

func (v *CloudflareAccessValidator) publicKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	key := v.keys[kid]
	v.mu.Unlock()
	if key != nil {
		return key, nil
	}
	keys, err := v.fetchKeys(ctx)
	if err != nil {
		return nil, err
	}
	v.mu.Lock()
	v.keys = keys
	key = v.keys[kid]
	v.mu.Unlock()
	if key == nil {
		return nil, errors.New("cloudflare access JWT key id is unknown")
	}
	return key, nil
}

func (v *CloudflareAccessValidator) fetchKeys(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	certsURL := strings.TrimSpace(v.CertsURL)
	if certsURL == "" {
		teamDomain := strings.TrimRight(strings.TrimSpace(v.TeamDomain), "/")
		certsURL = teamDomain + "/cdn-cgi/access/certs"
	}
	client := v.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, certsURL, nil)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch cloudflare access JWKS failed with status %d", res.StatusCode)
	}
	var set accessJWKS
	if err := json.NewDecoder(res.Body).Decode(&set); err != nil {
		return nil, fmt.Errorf("decode cloudflare access JWKS: %w", err)
	}
	keys := make(map[string]*rsa.PublicKey, len(set.Keys))
	for _, item := range set.Keys {
		key, err := item.publicKey()
		if err != nil {
			return nil, err
		}
		if item.KeyID != "" {
			keys[item.KeyID] = key
		}
	}
	return keys, nil
}

type accessJWTHeader struct {
	Algorithm string `json:"alg"`
	KeyID     string `json:"kid"`
	Type      string `json:"typ"`
}

type accessJWTPayload struct {
	Audience  accessAudience `json:"aud"`
	Issuer    string         `json:"iss"`
	ExpiresAt int64          `json:"exp"`
	NotBefore int64          `json:"nbf"`
	IssuedAt  int64          `json:"iat"`
	Email     string         `json:"email"`
	Subject   string         `json:"sub"`
	Type      string         `json:"type"`
}

type accessAudience []string

func (a accessAudience) Contains(value string) bool {
	for _, item := range a {
		if item == value {
			return true
		}
	}
	return false
}

func (a *accessAudience) UnmarshalJSON(data []byte) error {
	var items []string
	if err := json.Unmarshal(data, &items); err == nil {
		*a = items
		return nil
	}
	var item string
	if err := json.Unmarshal(data, &item); err != nil {
		return err
	}
	*a = []string{item}
	return nil
}

type accessJWKS struct {
	Keys []accessJWK `json:"keys"`
}

type accessJWK struct {
	KeyType   string `json:"kty"`
	KeyID     string `json:"kid"`
	Algorithm string `json:"alg"`
	Use       string `json:"use"`
	Modulus   string `json:"n"`
	Exponent  string `json:"e"`
}

func (k accessJWK) publicKey() (*rsa.PublicKey, error) {
	if k.KeyType != "RSA" {
		return nil, fmt.Errorf("unsupported cloudflare access JWK type %q", k.KeyType)
	}
	modulus, err := base64.RawURLEncoding.DecodeString(k.Modulus)
	if err != nil {
		return nil, fmt.Errorf("decode cloudflare access JWK modulus: %w", err)
	}
	exponentBytes, err := base64.RawURLEncoding.DecodeString(k.Exponent)
	if err != nil {
		return nil, fmt.Errorf("decode cloudflare access JWK exponent: %w", err)
	}
	exponent := new(big.Int).SetBytes(exponentBytes).Int64()
	if exponent <= 0 {
		return nil, errors.New("cloudflare access JWK exponent is invalid")
	}
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(modulus),
		E: int(exponent),
	}, nil
}

func decodeJWTPart(part string, target any) error {
	data, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}
