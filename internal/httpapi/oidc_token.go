package httpapi

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"
)

type oidcClaims struct {
	Issuer    string          `json:"iss"`
	Subject   string          `json:"sub"`
	Audience  json.RawMessage `json:"aud"`
	Expires   int64           `json:"exp"`
	IssuedAt  int64           `json:"iat"`
	NotBefore int64           `json:"nbf"`
	Nonce     string          `json:"nonce"`
}

func verifyIDToken(ctx context.Context, client *http.Client, raw, configuredIssuer, clientID, expectedNonce string) (string, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return "", errors.New("ID Token format is invalid")
	}
	decode := func(value string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(value) }
	headerBytes, err := decode(parts[0])
	if err != nil {
		return "", errors.New("ID Token header is invalid")
	}
	var header struct {
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
	}
	if json.Unmarshal(headerBytes, &header) != nil || header.Algorithm != "RS256" || header.KeyID == "" {
		return "", errors.New("ID Token must use RS256 with a key ID")
	}

	issuer := strings.TrimRight(configuredIssuer, "/")
	metadataURL := issuer + "/.well-known/openid-configuration"
	metadataReq, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return "", err
	}
	metadataResp, err := client.Do(metadataReq)
	if err != nil {
		return "", fmt.Errorf("load OIDC discovery: %w", err)
	}
	defer metadataResp.Body.Close()
	var metadata struct {
		Issuer  string `json:"issuer"`
		JWKSURI string `json:"jwks_uri"`
	}
	if metadataResp.StatusCode >= 300 || json.NewDecoder(io.LimitReader(metadataResp.Body, 1<<20)).Decode(&metadata) != nil {
		return "", errors.New("OIDC discovery response is invalid")
	}
	if strings.TrimRight(metadata.Issuer, "/") != issuer || metadata.JWKSURI == "" {
		return "", errors.New("OIDC discovery issuer or JWKS URI is invalid")
	}

	jwksReq, err := http.NewRequestWithContext(ctx, http.MethodGet, metadata.JWKSURI, nil)
	if err != nil {
		return "", err
	}
	jwksResp, err := client.Do(jwksReq)
	if err != nil {
		return "", fmt.Errorf("load OIDC signing keys: %w", err)
	}
	defer jwksResp.Body.Close()
	var keySet struct {
		Keys []struct {
			KeyType   string `json:"kty"`
			KeyID     string `json:"kid"`
			Algorithm string `json:"alg"`
			Use       string `json:"use"`
			Modulus   string `json:"n"`
			Exponent  string `json:"e"`
		} `json:"keys"`
	}
	if jwksResp.StatusCode >= 300 || json.NewDecoder(io.LimitReader(jwksResp.Body, 2<<20)).Decode(&keySet) != nil {
		return "", errors.New("OIDC JWKS response is invalid")
	}
	var publicKey *rsa.PublicKey
	for _, key := range keySet.Keys {
		if key.KeyID != header.KeyID || key.KeyType != "RSA" || (key.Algorithm != "" && key.Algorithm != "RS256") || (key.Use != "" && key.Use != "sig") {
			continue
		}
		n, nErr := decode(key.Modulus)
		e, eErr := decode(key.Exponent)
		if nErr != nil || eErr != nil || len(n) == 0 || len(e) == 0 || len(e) > 4 {
			continue
		}
		exponent := 0
		for _, value := range e {
			exponent = exponent<<8 | int(value)
		}
		if exponent < 3 {
			continue
		}
		candidate := &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: exponent}
		if candidate.N.BitLen() < 2048 {
			continue
		}
		publicKey = candidate
		break
	}
	if publicKey == nil {
		return "", errors.New("ID Token signing key was not found")
	}
	signature, err := decode(parts[2])
	if err != nil {
		return "", errors.New("ID Token signature is invalid")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err = rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, digest[:], signature); err != nil {
		return "", errors.New("ID Token signature verification failed")
	}

	claimsBytes, err := decode(parts[1])
	if err != nil {
		return "", errors.New("ID Token claims are invalid")
	}
	var claims oidcClaims
	if json.Unmarshal(claimsBytes, &claims) != nil {
		return "", errors.New("ID Token claims are invalid")
	}
	if strings.TrimRight(claims.Issuer, "/") != issuer || claims.Subject == "" || claims.Nonce != expectedNonce || !audienceContains(claims.Audience, clientID) {
		return "", errors.New("ID Token issuer, subject, audience, or nonce is invalid")
	}
	now := time.Now()
	skew := time.Minute
	if claims.Expires == 0 || claims.IssuedAt == 0 || now.After(time.Unix(claims.Expires, 0).Add(skew)) || time.Unix(claims.IssuedAt, 0).After(now.Add(skew)) || (claims.NotBefore != 0 && time.Unix(claims.NotBefore, 0).After(now.Add(skew))) {
		return "", errors.New("ID Token time claims are invalid")
	}
	return claims.Subject, nil
}

func audienceContains(raw json.RawMessage, clientID string) bool {
	var single string
	if json.Unmarshal(raw, &single) == nil {
		return single == clientID
	}
	var multiple []string
	if json.Unmarshal(raw, &multiple) != nil {
		return false
	}
	for _, audience := range multiple {
		if audience == clientID {
			return true
		}
	}
	return false
}
