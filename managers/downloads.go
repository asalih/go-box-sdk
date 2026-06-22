package managers

import (
	"context"
	"io"
	"net/http"

	boxerrors "github.com/asalih/go-box-sdk/errors"
	"github.com/asalih/go-box-sdk/networking"
)

// DownloadsManager exposes file content download. It mirrors the focused slice
// of src/managers/downloads.ts.
type DownloadsManager struct {
	baseManager
}

// NewDownloadsManager returns a DownloadsManager bound to the given auth and session.
func NewDownloadsManager(auth networking.Authentication, session *networking.NetworkSession) *DownloadsManager {
	return &DownloadsManager{baseManager{Auth: auth, NetworkSession: session}}
}

// DownloadFileOptions carries the optional parameters of DownloadFile.
type DownloadFileOptions struct {
	Version      string
	AccessToken  string
	Range        string
	Boxapi       string
	ExtraHeaders map[string]string
}

func (o *DownloadFileOptions) params() map[string]string {
	if o == nil {
		return nil
	}
	return prepareParams(map[string]string{
		"version":      o.Version,
		"access_token": o.AccessToken,
	})
}

func (o *DownloadFileOptions) headers() map[string]string {
	if o == nil {
		return nil
	}
	return mergeExtraHeaders(map[string]string{
		"range":  o.Range,
		"boxapi": o.Boxapi,
	}, o.ExtraHeaders)
}

// DownloadFile returns the contents of a file as a stream. A nil reader is
// returned when the server responds 202 (content not yet ready). It mirrors
// downloadFile. The caller is responsible for closing the returned stream when
// it implements io.Closer.
func (m *DownloadsManager) DownloadFile(ctx context.Context, fileID string, opts *DownloadFileOptions) (io.Reader, error) {
	resp, err := m.fetch(ctx, &networking.FetchOptions{
		URL:            m.NetworkSession.BaseURLs.BaseURL + "/2.0/files/" + fileID + "/content",
		Method:         http.MethodGet,
		Params:         opts.params(),
		Headers:        opts.headers(),
		ResponseFormat: networking.ResponseFormatBinary,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status == 202 {
		return nil, nil
	}
	return resp.Content, nil
}

// GetDownloadFileURL returns the redirect location for a file's content without
// downloading it. It issues a non-redirect-following request and reads the
// Location header. It mirrors getDownloadFileUrl (non-browser path).
func (m *DownloadsManager) GetDownloadFileURL(ctx context.Context, fileID string, opts *DownloadFileOptions) (string, error) {
	followRedirects := false
	resp, err := m.fetch(ctx, &networking.FetchOptions{
		URL:             m.NetworkSession.BaseURLs.BaseURL + "/2.0/files/" + fileID + "/content",
		Method:          http.MethodGet,
		Params:          opts.params(),
		Headers:         opts.headers(),
		ResponseFormat:  networking.ResponseFormatNoContent,
		FollowRedirects: &followRedirects,
	})
	if err != nil {
		return "", err
	}
	if location, ok := resp.Headers["location"]; ok {
		return location, nil
	}
	if location, ok := resp.Headers["Location"]; ok {
		return location, nil
	}
	return "", boxerrors.NewBoxSDKError("No location header in response")
}
