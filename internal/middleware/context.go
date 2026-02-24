package middleware

// Context keys and lightweight data structures used by middleware to
// share information across the request lifecycle.

// apiKeyInfo holds essential information about the authenticated API key
// that is used by subsequent middleware.
type apiKeyInfo struct {
    ID            int
    UserID        int
    KeyHash       string
    QuotaTokens   int
    UsedTokens    int
    RateLimitRPM  int
}

// apiKeyContextKey is the context key under which the apiKeyInfo is stored
// in the request's context. Using a distinct type avoids collisions with
// other context values.
type apiKeyContextKey string

const (
    // apiKeyContextKeyValue is the key used to store apiKeyInfo in the context.
    apiKeyContextKeyValue apiKeyContextKey = "api_key_info"
)
