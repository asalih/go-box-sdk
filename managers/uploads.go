package managers

import (
	"context"
	"io"
	"net/http"
	"strings"

	boxerrors "github.com/asalih/go-box-sdk/errors"
	"github.com/asalih/go-box-sdk/networking"
	"github.com/asalih/go-box-sdk/schemas"
)

// UploadsManager exposes the small-file upload endpoints: preflight check,
// upload a new file, upload a new version, and upload with a preflight check.
// It mirrors the focused slice of src/managers/uploads.ts.
type UploadsManager struct {
	baseManager
}

// NewUploadsManager returns an UploadsManager bound to the given auth and session.
func NewUploadsManager(auth networking.Authentication, session *networking.NetworkSession) *UploadsManager {
	return &UploadsManager{baseManager{Auth: auth, NetworkSession: session}}
}

// UploadFileAttributesParent identifies the destination folder of an upload.
type UploadFileAttributesParent struct {
	ID string `json:"id"`
}

// UploadFileAttributes is the JSON "attributes" part of a multipart upload. The
// attributes part must be sent before the file part. It mirrors
// UploadFileRequestBodyAttributesField.
type UploadFileAttributes struct {
	Name              string                     `json:"name"`
	Parent            UploadFileAttributesParent `json:"parent"`
	ContentCreatedAt  string                     `json:"content_created_at,omitempty"`
	ContentModifiedAt string                     `json:"content_modified_at,omitempty"`
}

// UploadFileRequestBody is the body of UploadFile: the attributes plus the file
// content stream and optional part metadata.
type UploadFileRequestBody struct {
	Attributes      UploadFileAttributes
	File            io.Reader
	FileFileName    string
	FileContentType string
}

// UploadFileOptions carries the optional parameters of UploadFile.
type UploadFileOptions struct {
	Fields       []string
	ContentMD5   string
	ExtraHeaders map[string]string
}

// UploadFile uploads a small file to Box. For files over 50MB use the chunked
// upload API. It mirrors uploadFile. The attributes part is written before the
// file part to satisfy the Box multipart ordering requirement.
func (m *UploadsManager) UploadFile(ctx context.Context, requestBody *UploadFileRequestBody, opts *UploadFileOptions) (*schemas.Files, error) {
	if opts == nil {
		opts = &UploadFileOptions{}
	}
	params := prepareParams(map[string]string{"fields": joinFields(opts.Fields)})
	headers := mergeExtraHeaders(map[string]string{"content-md5": opts.ContentMD5}, opts.ExtraHeaders)
	resp, err := m.fetch(ctx, &networking.FetchOptions{
		URL:     m.NetworkSession.BaseURLs.UploadURL + "/2.0/files/content",
		Method:  http.MethodPost,
		Params:  params,
		Headers: headers,
		MultipartData: []networking.MultipartItem{
			{PartName: "attributes", Data: requestBody.Attributes},
			{PartName: "file", FileStream: requestBody.File, FileName: requestBody.FileFileName, ContentType: requestBody.FileContentType},
		},
		ContentType:    networking.ContentTypeMultipartForm,
		ResponseFormat: networking.ResponseFormatJSON,
	})
	if err != nil {
		return nil, err
	}
	return schemas.Decode[*schemas.Files](resp.Data)
}

// UploadFileVersionAttributes is the JSON "attributes" part of a new-version
// upload. It mirrors UploadFileVersionRequestBodyAttributesField.
type UploadFileVersionAttributes struct {
	Name              string `json:"name"`
	ContentModifiedAt string `json:"content_modified_at,omitempty"`
}

// UploadFileVersionRequestBody is the body of UploadFileVersion.
type UploadFileVersionRequestBody struct {
	Attributes      UploadFileVersionAttributes
	File            io.Reader
	FileFileName    string
	FileContentType string
}

// UploadFileVersionOptions carries the optional parameters of UploadFileVersion.
type UploadFileVersionOptions struct {
	Fields       []string
	IfMatch      string
	ContentMD5   string
	ExtraHeaders map[string]string
}

// UploadFileVersion uploads a new version of an existing file. It mirrors
// uploadFileVersion.
func (m *UploadsManager) UploadFileVersion(ctx context.Context, fileID string, requestBody *UploadFileVersionRequestBody, opts *UploadFileVersionOptions) (*schemas.Files, error) {
	if opts == nil {
		opts = &UploadFileVersionOptions{}
	}
	params := prepareParams(map[string]string{"fields": joinFields(opts.Fields)})
	headers := mergeExtraHeaders(map[string]string{
		"if-match":    opts.IfMatch,
		"content-md5": opts.ContentMD5,
	}, opts.ExtraHeaders)
	resp, err := m.fetch(ctx, &networking.FetchOptions{
		URL:     m.NetworkSession.BaseURLs.UploadURL + "/2.0/files/" + fileID + "/content",
		Method:  http.MethodPost,
		Params:  params,
		Headers: headers,
		MultipartData: []networking.MultipartItem{
			{PartName: "attributes", Data: requestBody.Attributes},
			{PartName: "file", FileStream: requestBody.File, FileName: requestBody.FileFileName, ContentType: requestBody.FileContentType},
		},
		ContentType:    networking.ContentTypeMultipartForm,
		ResponseFormat: networking.ResponseFormatJSON,
	})
	if err != nil {
		return nil, err
	}
	return schemas.Decode[*schemas.Files](resp.Data)
}

// PreflightFileUploadCheckParent identifies the parent folder for a preflight check.
type PreflightFileUploadCheckParent struct {
	ID string `json:"id,omitempty"`
}

// PreflightFileUploadCheckRequestBody is the body of PreflightFileUploadCheck.
type PreflightFileUploadCheckRequestBody struct {
	Name   string                          `json:"name,omitempty"`
	Size   *int64                          `json:"size,omitempty"`
	Parent *PreflightFileUploadCheckParent `json:"parent,omitempty"`
}

// PreflightFileUploadCheck verifies that a file will be accepted by Box before
// uploading the whole file. It mirrors preflightFileUploadCheck.
func (m *UploadsManager) PreflightFileUploadCheck(ctx context.Context, requestBody *PreflightFileUploadCheckRequestBody, extraHeaders map[string]string) (*schemas.UploadURL, error) {
	if requestBody == nil {
		requestBody = &PreflightFileUploadCheckRequestBody{}
	}
	resp, err := m.fetch(ctx, &networking.FetchOptions{
		URL:            m.NetworkSession.BaseURLs.BaseURL + "/2.0/files/content",
		Method:         http.MethodOptions,
		Headers:        prepareParams(extraHeaders),
		Data:           requestBody,
		ContentType:    networking.ContentTypeJSON,
		ResponseFormat: networking.ResponseFormatJSON,
	})
	if err != nil {
		return nil, err
	}
	return schemas.Decode[*schemas.UploadURL](resp.Data)
}

// UploadWithPreflightCheck runs a preflight check and then uploads the file to
// the returned upload URL. It mirrors uploadWithPreflightCheck.
func (m *UploadsManager) UploadWithPreflightCheck(ctx context.Context, requestBody *UploadFileRequestBody, opts *UploadFileOptions) (*schemas.Files, error) {
	if opts == nil {
		opts = &UploadFileOptions{}
	}
	params := prepareParams(map[string]string{"fields": joinFields(opts.Fields)})
	headers := mergeExtraHeaders(map[string]string{"content-md5": opts.ContentMD5}, opts.ExtraHeaders)

	preflight, err := m.PreflightFileUploadCheck(ctx, &PreflightFileUploadCheckRequestBody{
		Name:   requestBody.Attributes.Name,
		Parent: &PreflightFileUploadCheckParent{ID: requestBody.Attributes.Parent.ID},
	}, opts.ExtraHeaders)
	if err != nil {
		return nil, err
	}
	if preflight.UploadURL == "" || !strings.Contains(preflight.UploadURL, "http") {
		return nil, boxerrors.NewBoxSDKError("Unable to get preflight upload URL")
	}
	resp, err := m.fetch(ctx, &networking.FetchOptions{
		URL:     preflight.UploadURL,
		Method:  http.MethodPost,
		Params:  params,
		Headers: headers,
		MultipartData: []networking.MultipartItem{
			{PartName: "attributes", Data: requestBody.Attributes},
			{PartName: "file", FileStream: requestBody.File, FileName: requestBody.FileFileName, ContentType: requestBody.FileContentType},
		},
		ContentType:    networking.ContentTypeMultipartForm,
		ResponseFormat: networking.ResponseFormatJSON,
	})
	if err != nil {
		return nil, err
	}
	return schemas.Decode[*schemas.Files](resp.Data)
}
