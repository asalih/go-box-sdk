package managers

import (
	"context"
	"net/http"

	"github.com/asalih/go-box-sdk/networking"
	"github.com/asalih/go-box-sdk/schemas"
)

// FoldersManager exposes the folder endpoints used by the repository: create a
// folder, get folder info, list folder items, and delete a folder. It mirrors
// the focused slice of src/managers/folders.ts.
type FoldersManager struct {
	baseManager
}

// NewFoldersManager returns a FoldersManager bound to the given auth and session.
func NewFoldersManager(auth networking.Authentication, session *networking.NetworkSession) *FoldersManager {
	return &FoldersManager{baseManager{Auth: auth, NetworkSession: session}}
}

// CreateFolderParent identifies the parent folder a new folder is created in.
type CreateFolderParent struct {
	ID string `json:"id"`
}

// CreateFolderRequestBody is the body of CreateFolder. It mirrors
// CreateFolderRequestBody; only the name and parent are required.
type CreateFolderRequestBody struct {
	Name   string             `json:"name"`
	Parent CreateFolderParent `json:"parent"`
}

// CreateFolder creates a new empty folder within the specified parent folder.
func (m *FoldersManager) CreateFolder(ctx context.Context, requestBody *CreateFolderRequestBody, fields []string, extraHeaders map[string]string) (*schemas.FolderFull, error) {
	params := prepareParams(map[string]string{"fields": joinFields(fields)})
	resp, err := m.fetch(ctx, &networking.FetchOptions{
		URL:            m.NetworkSession.BaseURLs.BaseURL + "/2.0/folders",
		Method:         http.MethodPost,
		Params:         params,
		Headers:        prepareParams(extraHeaders),
		Data:           requestBody,
		ContentType:    networking.ContentTypeJSON,
		ResponseFormat: networking.ResponseFormatJSON,
	})
	if err != nil {
		return nil, err
	}
	return schemas.Decode[*schemas.FolderFull](resp.Data)
}

// GetFolderByIDOptions carries the optional parameters of GetFolderByID.
type GetFolderByIDOptions struct {
	Fields       []string
	Sort         string
	Direction    string
	Offset       string
	Limit        string
	IfNoneMatch  string
	Boxapi       string
	ExtraHeaders map[string]string
}

// GetFolderByID retrieves details about a folder, including the first page of
// its items if requested. It mirrors getFolderById.
func (m *FoldersManager) GetFolderByID(ctx context.Context, folderID string, opts *GetFolderByIDOptions) (*schemas.FolderFull, error) {
	if opts == nil {
		opts = &GetFolderByIDOptions{}
	}
	params := prepareParams(map[string]string{
		"fields":    joinFields(opts.Fields),
		"sort":      opts.Sort,
		"direction": opts.Direction,
		"offset":    opts.Offset,
		"limit":     opts.Limit,
	})
	headers := mergeExtraHeaders(map[string]string{
		"if-none-match": opts.IfNoneMatch,
		"boxapi":        opts.Boxapi,
	}, opts.ExtraHeaders)
	resp, err := m.fetch(ctx, &networking.FetchOptions{
		URL:            m.NetworkSession.BaseURLs.BaseURL + "/2.0/folders/" + folderID,
		Method:         http.MethodGet,
		Params:         params,
		Headers:        headers,
		ResponseFormat: networking.ResponseFormatJSON,
	})
	if err != nil {
		return nil, err
	}
	return schemas.Decode[*schemas.FolderFull](resp.Data)
}

// GetFolderItemsOptions carries the optional parameters of GetFolderItems.
type GetFolderItemsOptions struct {
	Fields       []string
	UseMarker    string
	Marker       string
	Offset       string
	Limit        string
	Sort         string
	Direction    string
	Boxapi       string
	ExtraHeaders map[string]string
}

// GetFolderItems retrieves a page of items (files, folders, web links) in a
// folder. It mirrors getFolderItems.
func (m *FoldersManager) GetFolderItems(ctx context.Context, folderID string, opts *GetFolderItemsOptions) (*schemas.Items, error) {
	if opts == nil {
		opts = &GetFolderItemsOptions{}
	}
	params := prepareParams(map[string]string{
		"fields":    joinFields(opts.Fields),
		"usemarker": opts.UseMarker,
		"marker":    opts.Marker,
		"offset":    opts.Offset,
		"limit":     opts.Limit,
		"sort":      opts.Sort,
		"direction": opts.Direction,
	})
	headers := mergeExtraHeaders(map[string]string{"boxapi": opts.Boxapi}, opts.ExtraHeaders)
	resp, err := m.fetch(ctx, &networking.FetchOptions{
		URL:            m.NetworkSession.BaseURLs.BaseURL + "/2.0/folders/" + folderID + "/items",
		Method:         http.MethodGet,
		Params:         params,
		Headers:        headers,
		ResponseFormat: networking.ResponseFormatJSON,
	})
	if err != nil {
		return nil, err
	}
	return schemas.Decode[*schemas.Items](resp.Data)
}

// DeleteFolderByIDOptions carries the optional parameters of DeleteFolderByID.
type DeleteFolderByIDOptions struct {
	Recursive    string
	IfMatch      string
	ExtraHeaders map[string]string
}

// DeleteFolderByID deletes a folder, either permanently or by moving it to the
// trash. It mirrors deleteFolderById.
func (m *FoldersManager) DeleteFolderByID(ctx context.Context, folderID string, opts *DeleteFolderByIDOptions) error {
	if opts == nil {
		opts = &DeleteFolderByIDOptions{}
	}
	params := prepareParams(map[string]string{"recursive": opts.Recursive})
	headers := mergeExtraHeaders(map[string]string{"if-match": opts.IfMatch}, opts.ExtraHeaders)
	_, err := m.fetch(ctx, &networking.FetchOptions{
		URL:            m.NetworkSession.BaseURLs.BaseURL + "/2.0/folders/" + folderID,
		Method:         http.MethodDelete,
		Params:         params,
		Headers:        headers,
		ResponseFormat: networking.ResponseFormatNoContent,
	})
	return err
}
