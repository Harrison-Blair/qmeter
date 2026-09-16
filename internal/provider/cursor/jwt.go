package cursor

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// userIDFromJWT extracts the Cursor user id from an access token.
//
// Route A identifies the account by the user id carried in the token's "sub"
// claim, which looks like "google-oauth2|<id>" or "auth0|<id>"; the user id is
// the part after the LAST "|" (a subject with no "|" is used whole).
//
// The token's payload is base64url-decoded and read as JSON. The signature is
// NOT verified and the expiry is NOT checked: qmeter only needs a claim to
// build the request cookie with, and the vendor decides whether the token is
// still good (a rejected token comes back as 401/403, which httpx maps to
// provider.ErrTokenExpired). Decoding an expired token still works, which is
// deliberate — Detect must report a store with an expired credential as
// present.
//
// This decoder is intentionally private to this package: per the plan it must
// neither import nor be imported by the Codex id_token decoder. If the two
// ever converge, the shared logic moves to a new internal/lib/jwt package.
func userIDFromJWT(token string) (string, error) {
	sub, err := jwtSubject(token)
	if err != nil {
		return "", err
	}
	id := sub[strings.LastIndex(sub, "|")+1:]
	if id == "" {
		return "", fmt.Errorf("JWT sub claim %q has no user id after the last %q", sub, "|")
	}
	return id, nil
}

// jwtSubject returns the "sub" claim of a JWT's payload.
func jwtSubject(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("not a JWT: want 3 dot-separated segments, got %d", len(parts))
	}
	payload, err := decodeSegment(parts[1])
	if err != nil {
		return "", fmt.Errorf("not a JWT: decode payload: %w", err)
	}
	var claims struct {
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("not a JWT: parse payload claims: %w", err)
	}
	if claims.Sub == "" {
		return "", fmt.Errorf("JWT payload has no %q claim", "sub")
	}
	return claims.Sub, nil
}

// decodeSegment base64url-decodes one JWT segment. JWTs are specified as
// unpadded base64url, but a padded segment is accepted too rather than
// rejecting a token the vendor would have honoured.
func decodeSegment(seg string) ([]byte, error) {
	if strings.HasSuffix(seg, "=") {
		return base64.URLEncoding.DecodeString(seg)
	}
	return base64.RawURLEncoding.DecodeString(seg)
}
