// Package networking is the Go port of src/networking from the Box SDK. It
// provides the HTTP transport (BoxNetworkClient), request/response models,
// retry strategy, network session configuration, and the Authentication
// interface that the auth layer implements.
package networking

// Default base URLs for the Box API. They mirror src/networking/baseUrls.ts.
const (
	DefaultBaseURL   = "https://api.box.com"
	DefaultUploadURL = "https://upload.box.com/api"
	DefaultOAuth2URL = "https://account.box.com/api/oauth2"
)

// BaseURLs holds the API, upload, and OAuth2 base URLs used for requests.
type BaseURLs struct {
	BaseURL   string
	UploadURL string
	OAuth2URL string
}

// NewBaseURLs returns BaseURLs seeded with the Box defaults.
func NewBaseURLs() *BaseURLs {
	return &BaseURLs{
		BaseURL:   DefaultBaseURL,
		UploadURL: DefaultUploadURL,
		OAuth2URL: DefaultOAuth2URL,
	}
}
