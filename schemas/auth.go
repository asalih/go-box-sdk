package schemas

// ResourceScope describes a single resource and the scopes permitted on it,
// as returned in a downscoped access token's restricted_to list.
type ResourceScope struct {
	Scope  string         `json:"scope,omitempty"`
	Object map[string]any `json:"object,omitempty"`
}

// AccessToken is the OAuth2 token response returned by the Box token endpoint.
type AccessToken struct {
	AccessToken     string          `json:"access_token,omitempty"`
	ExpiresIn       *int64          `json:"expires_in,omitempty"`
	TokenType       string          `json:"token_type,omitempty"`
	RestrictedTo    []ResourceScope `json:"restricted_to,omitempty"`
	RefreshToken    string          `json:"refresh_token,omitempty"`
	IssuedTokenType string          `json:"issued_token_type,omitempty"`
}

// OAuth2 grant type constants used in token requests.
const (
	GrantTypeAuthorizationCode = "authorization_code"
	GrantTypeRefreshToken      = "refresh_token"
	GrantTypeClientCredentials = "client_credentials"
	GrantTypeJWTBearer         = "urn:ietf:params:oauth:grant-type:jwt-bearer"
	GrantTypeTokenExchange     = "urn:ietf:params:oauth:grant-type:token-exchange"
)

// Subject token type constants used in token-exchange requests.
const (
	SubjectTokenTypeAccessToken = "urn:ietf:params:oauth:token-type:access_token"
)

// Box subject type constants used in client-credentials requests.
const (
	BoxSubjectTypeEnterprise = "enterprise"
	BoxSubjectTypeUser       = "user"
)

// PostOAuth2Token is the request body for the Box token endpoint.
type PostOAuth2Token struct {
	GrantType        string `json:"grant_type"`
	ClientID         string `json:"client_id,omitempty"`
	ClientSecret     string `json:"client_secret,omitempty"`
	Code             string `json:"code,omitempty"`
	RefreshToken     string `json:"refresh_token,omitempty"`
	Assertion        string `json:"assertion,omitempty"`
	SubjectToken     string `json:"subject_token,omitempty"`
	SubjectTokenType string `json:"subject_token_type,omitempty"`
	ActorToken       string `json:"actor_token,omitempty"`
	ActorTokenType   string `json:"actor_token_type,omitempty"`
	Scope            string `json:"scope,omitempty"`
	Resource         string `json:"resource,omitempty"`
	BoxSubjectType   string `json:"box_subject_type,omitempty"`
	BoxSubjectID     string `json:"box_subject_id,omitempty"`
	BoxSharedLink    string `json:"box_shared_link,omitempty"`
}

// PostOAuth2Revoke is the request body for the Box token revocation endpoint.
type PostOAuth2Revoke struct {
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
	Token        string `json:"token,omitempty"`
}

// OAuth2Error is the error body returned by the OAuth2 endpoints.
type OAuth2Error struct {
	Error            string `json:"error,omitempty"`
	ErrorDescription string `json:"error_description,omitempty"`
}
