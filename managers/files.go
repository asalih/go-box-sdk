package managers

import (
	"context"
	"net/http"

	"github.com/asalih/go-box-sdk/networking"
	"github.com/asalih/go-box-sdk/schemas"
)

// FilesManager exposes the file metadata endpoints. It mirrors the focused
// slice of src/managers/files.ts used by the repository: retrieving file info.
type FilesManager struct {
	baseManager
}

// NewFilesManager returns a FilesManager bound to the given auth and session.
func NewFilesManager(auth networking.Authentication, session *networking.NetworkSession) *FilesManager {
	return &FilesManager{baseManager{Auth: auth, NetworkSession: session}}
}

// GetFileByIDOptions carries the optional query parameters and headers of
// GetFileByID. A nil pointer selects the defaults.
type GetFileByIDOptions struct {
	Fields       []string
	IfNoneMatch  string
	Boxapi       string
	XRepHints    string
	ExtraHeaders map[string]string
}

// GetFileByID retrieves the metadata about a file. It mirrors getFileById.
func (m *FilesManager) GetFileByID(ctx context.Context, fileID string, opts *GetFileByIDOptions) (*schemas.FileFull, error) {
	if opts == nil {
		opts = &GetFileByIDOptions{}
	}
	params := prepareParams(map[string]string{"fields": joinFields(opts.Fields)})
	headers := mergeExtraHeaders(map[string]string{
		"if-none-match": opts.IfNoneMatch,
		"boxapi":        opts.Boxapi,
		"x-rep-hints":   opts.XRepHints,
	}, opts.ExtraHeaders)
	resp, err := m.fetch(ctx, &networking.FetchOptions{
		URL:            m.NetworkSession.BaseURLs.BaseURL + "/2.0/files/" + fileID,
		Method:         http.MethodGet,
		Params:         params,
		Headers:        headers,
		ResponseFormat: networking.ResponseFormatJSON,
	})
	if err != nil {
		return nil, err
	}
	return schemas.Decode[*schemas.FileFull](resp.Data)
}
