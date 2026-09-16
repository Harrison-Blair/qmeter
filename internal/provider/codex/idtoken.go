package codex

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// openAIAuthClaim is the namespace OpenAI nests the chatgpt_* claims under.
const openAIAuthClaim = "https://api.openai.com/auth"

// idTokenClaims are the only claims qmeter reads out of a Codex id_token.
type idTokenClaims struct {
	PlanType  string
	AccountID string
}

// decodeIDToken reads the claims qmeter needs out of a Codex id_token.
//
// The token is a JWT, but qmeter is a read-only usage reader, not a resource
// server: it does not verify the signature, and it deliberately ignores exp.
// The id_token lives about an hour while the access token lives about ten
// days, so by the time usage is worth looking at the id_token is usually
// expired — yet chatgpt_plan_type and chatgpt_account_id are still exactly
// right, and they are all this decoder is for. Treating an expired token as
// undecodable would throw away the plan name for no benefit.
//
// Both the flat claim names (chatgpt_plan_type) and the namespaced form
// OpenAI uses ({"https://api.openai.com/auth": {...}}) are read, in
// snake_case or camelCase.
func decodeIDToken(token string) (idTokenClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 || parts[1] == "" {
		return idTokenClaims{}, errors.New("id_token: not a JWT")
	}
	payload, err := decodeSegment(parts[1])
	if err != nil {
		return idTokenClaims{}, fmt.Errorf("id_token payload: %w", err)
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(payload, &obj); err != nil {
		return idTokenClaims{}, fmt.Errorf("id_token claims: %w", err)
	}

	claims := claimsFrom(obj)
	if claims.PlanType != "" && claims.AccountID != "" {
		return claims, nil
	}
	// Fall back to the claim objects these are nested inside: OpenAI's own
	// namespace first, then any other object claim in sorted order. Map
	// iteration order must not decide which of two candidates wins.
	for _, key := range nestedClaimKeys(obj) {
		var nested map[string]json.RawMessage
		if json.Unmarshal(obj[key], &nested) != nil {
			continue
		}
		inner := claimsFrom(nested)
		if claims.PlanType == "" {
			claims.PlanType = inner.PlanType
		}
		if claims.AccountID == "" {
			claims.AccountID = inner.AccountID
		}
		if claims.PlanType != "" && claims.AccountID != "" {
			break
		}
	}
	return claims, nil
}

// nestedClaimKeys returns the claim names to search for nested chatgpt_*
// claims, in a deterministic order: the OpenAI namespace, then the rest
// sorted.
func nestedClaimKeys(obj map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(obj))
	for key := range obj {
		if key != openAIAuthClaim {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	if _, ok := obj[openAIAuthClaim]; ok {
		keys = append([]string{openAIAuthClaim}, keys...)
	}
	return keys
}

// claimsFrom reads the two claims out of one decoded JSON object, matching
// key names in either snake_case or camelCase.
func claimsFrom(obj map[string]json.RawMessage) idTokenClaims {
	var claims idTokenClaims
	forEachField(obj, func(key string, raw json.RawMessage) {
		switch key {
		case "chatgptplantype":
			claims.PlanType = decodeString(raw)
		case "chatgptaccountid":
			claims.AccountID = decodeString(raw)
		}
	})
	return claims
}

// decodeSegment decodes one base64url JWT segment, tolerating the padded
// spelling as well as the unpadded one the spec calls for.
func decodeSegment(seg string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(seg, "="))
}
