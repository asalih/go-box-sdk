package managers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	boxerrors "github.com/asalih/go-box-sdk/errors"
	"github.com/asalih/go-box-sdk/internal/utils"
	"github.com/asalih/go-box-sdk/networking"
	"github.com/asalih/go-box-sdk/schemas"
)

// ChunkedUploadsManager exposes the chunked (multipart session) upload API and
// the high-level UploadBigFile helper. It mirrors src/managers/chunkedUploads.ts.
type ChunkedUploadsManager struct {
	baseManager
}

// NewChunkedUploadsManager returns a ChunkedUploadsManager bound to the given
// auth and session.
func NewChunkedUploadsManager(auth networking.Authentication, session *networking.NetworkSession) *ChunkedUploadsManager {
	return &ChunkedUploadsManager{baseManager{Auth: auth, NetworkSession: session}}
}

// CreateFileUploadSessionRequestBody is the body of CreateFileUploadSession.
type CreateFileUploadSessionRequestBody struct {
	FolderID string `json:"folder_id"`
	FileSize int64  `json:"file_size"`
	FileName string `json:"file_name"`
}

// CreateFileUploadSession creates an upload session for a new file. It mirrors
// createFileUploadSession.
func (m *ChunkedUploadsManager) CreateFileUploadSession(ctx context.Context, requestBody *CreateFileUploadSessionRequestBody, extraHeaders map[string]string) (*schemas.UploadSession, error) {
	resp, err := m.fetch(ctx, &networking.FetchOptions{
		URL:            m.NetworkSession.BaseURLs.UploadURL + "/2.0/files/upload_sessions",
		Method:         http.MethodPost,
		Headers:        prepareParams(extraHeaders),
		Data:           requestBody,
		ContentType:    networking.ContentTypeJSON,
		ResponseFormat: networking.ResponseFormatJSON,
	})
	if err != nil {
		return nil, err
	}
	return schemas.Decode[*schemas.UploadSession](resp.Data)
}

// GetFileUploadSessionByID returns information about an upload session. It
// mirrors getFileUploadSessionById.
func (m *ChunkedUploadsManager) GetFileUploadSessionByID(ctx context.Context, uploadSessionID string, extraHeaders map[string]string) (*schemas.UploadSession, error) {
	resp, err := m.fetch(ctx, &networking.FetchOptions{
		URL:            m.NetworkSession.BaseURLs.UploadURL + "/2.0/files/upload_sessions/" + uploadSessionID,
		Method:         http.MethodGet,
		Headers:        prepareParams(extraHeaders),
		ResponseFormat: networking.ResponseFormatJSON,
	})
	if err != nil {
		return nil, err
	}
	return schemas.Decode[*schemas.UploadSession](resp.Data)
}

// UploadFilePartByURL uploads a chunk to the session's per-part endpoint. The
// digest must be "sha=<base64 sha1>" and contentRange the chunk's byte range.
// It mirrors uploadFilePartByUrl.
func (m *ChunkedUploadsManager) UploadFilePartByURL(ctx context.Context, url string, body io.Reader, digest, contentRange string, extraHeaders map[string]string) (*schemas.UploadedPart, error) {
	headers := mergeExtraHeaders(map[string]string{
		"digest":        digest,
		"content-range": contentRange,
	}, extraHeaders)
	resp, err := m.fetch(ctx, &networking.FetchOptions{
		URL:            url,
		Method:         http.MethodPut,
		Headers:        headers,
		FileStream:     body,
		ContentType:    networking.ContentTypeOctetStream,
		ResponseFormat: networking.ResponseFormatJSON,
	})
	if err != nil {
		return nil, err
	}
	return schemas.Decode[*schemas.UploadedPart](resp.Data)
}

// UploadFilePart uploads a chunk by upload session ID. It mirrors uploadFilePart.
func (m *ChunkedUploadsManager) UploadFilePart(ctx context.Context, uploadSessionID string, body io.Reader, digest, contentRange string, extraHeaders map[string]string) (*schemas.UploadedPart, error) {
	url := m.NetworkSession.BaseURLs.UploadURL + "/2.0/files/upload_sessions/" + uploadSessionID
	return m.UploadFilePartByURL(ctx, url, body, digest, contentRange, extraHeaders)
}

// DeleteFileUploadSessionByID aborts an upload session and discards uploaded
// data. It mirrors deleteFileUploadSessionById.
func (m *ChunkedUploadsManager) DeleteFileUploadSessionByID(ctx context.Context, uploadSessionID string, extraHeaders map[string]string) error {
	_, err := m.fetch(ctx, &networking.FetchOptions{
		URL:            m.NetworkSession.BaseURLs.UploadURL + "/2.0/files/upload_sessions/" + uploadSessionID,
		Method:         http.MethodDelete,
		Headers:        prepareParams(extraHeaders),
		ResponseFormat: networking.ResponseFormatNoContent,
	})
	return err
}

// GetFileUploadSessionPartsByURL lists the chunks uploaded so far via the
// session's list-parts endpoint. It mirrors getFileUploadSessionPartsByUrl.
func (m *ChunkedUploadsManager) GetFileUploadSessionPartsByURL(ctx context.Context, url, offset, limit string, extraHeaders map[string]string) (*schemas.UploadParts, error) {
	params := prepareParams(map[string]string{"offset": offset, "limit": limit})
	resp, err := m.fetch(ctx, &networking.FetchOptions{
		URL:            url,
		Method:         http.MethodGet,
		Params:         params,
		Headers:        prepareParams(extraHeaders),
		ResponseFormat: networking.ResponseFormatJSON,
	})
	if err != nil {
		return nil, err
	}
	return schemas.Decode[*schemas.UploadParts](resp.Data)
}

// GetFileUploadSessionParts lists the chunks uploaded so far by session ID. It
// mirrors getFileUploadSessionParts.
func (m *ChunkedUploadsManager) GetFileUploadSessionParts(ctx context.Context, uploadSessionID, offset, limit string, extraHeaders map[string]string) (*schemas.UploadParts, error) {
	url := m.NetworkSession.BaseURLs.UploadURL + "/2.0/files/upload_sessions/" + uploadSessionID + "/parts"
	return m.GetFileUploadSessionPartsByURL(ctx, url, offset, limit, extraHeaders)
}

// commitRequestBody is the JSON body of a commit request: the uploaded parts.
type commitRequestBody struct {
	Parts []schemas.UploadPart `json:"parts"`
}

// CreateFileUploadSessionCommitByURL closes an upload session and assembles the
// file from the uploaded parts via the session's commit endpoint. A nil result
// is returned when the server responds 202 (still processing). It mirrors
// createFileUploadSessionCommitByUrl.
func (m *ChunkedUploadsManager) CreateFileUploadSessionCommitByURL(ctx context.Context, url string, parts []schemas.UploadPart, digest, ifMatch, ifNoneMatch string, extraHeaders map[string]string) (*schemas.Files, error) {
	headers := mergeExtraHeaders(map[string]string{
		"digest":        digest,
		"if-match":      ifMatch,
		"if-none-match": ifNoneMatch,
	}, extraHeaders)
	resp, err := m.fetch(ctx, &networking.FetchOptions{
		URL:            url,
		Method:         http.MethodPost,
		Headers:        headers,
		Data:           &commitRequestBody{Parts: parts},
		ContentType:    networking.ContentTypeJSON,
		ResponseFormat: networking.ResponseFormatJSON,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status == 202 {
		return nil, nil
	}
	return schemas.Decode[*schemas.Files](resp.Data)
}

// CreateFileUploadSessionCommit closes an upload session by session ID. It
// mirrors createFileUploadSessionCommit.
func (m *ChunkedUploadsManager) CreateFileUploadSessionCommit(ctx context.Context, uploadSessionID string, parts []schemas.UploadPart, digest, ifMatch, ifNoneMatch string, extraHeaders map[string]string) (*schemas.Files, error) {
	url := m.NetworkSession.BaseURLs.UploadURL + "/2.0/files/upload_sessions/" + uploadSessionID + "/commit"
	return m.CreateFileUploadSessionCommitByURL(ctx, url, parts, digest, ifMatch, ifNoneMatch, extraHeaders)
}

// UploadBigFile chunk-uploads a large file and returns the resulting file. It
// mirrors uploadBigFile: create a session, upload each fixed-size chunk
// sequentially while verifying the per-part SHA1, size and offset, fold each
// chunk into the whole-file SHA1, confirm the part count, then commit with the
// whole-file digest. The chunk loop is the Go equivalent of the sequential
// reduceIterator in the source.
func (m *ChunkedUploadsManager) UploadBigFile(ctx context.Context, file io.Reader, fileName string, fileSize int64, parentFolderID string) (*schemas.FileFull, error) {
	session, err := m.CreateFileUploadSession(ctx, &CreateFileUploadSessionRequestBody{
		FolderID: parentFolderID,
		FileSize: fileSize,
		FileName: fileName,
	}, nil)
	if err != nil {
		return nil, err
	}
	if session.SessionEndpoints == nil {
		return nil, boxerrors.NewBoxSDKError("upload session is missing session endpoints")
	}
	uploadPartURL := session.SessionEndpoints.UploadPart
	commitURL := session.SessionEndpoints.Commit
	listPartsURL := session.SessionEndpoints.ListParts
	if session.PartSize == nil || session.TotalParts == nil {
		return nil, boxerrors.NewBoxSDKError("upload session is missing part size or total parts")
	}
	partSize := *session.PartSize
	totalParts := *session.TotalParts
	if partSize*totalParts < fileSize {
		return nil, boxerrors.NewBoxSDKError("Assertion failed")
	}
	if session.NumPartsProcessed != nil && *session.NumPartsProcessed != 0 {
		return nil, boxerrors.NewBoxSDKError("Assertion failed")
	}

	fileHash := utils.NewHash()
	parts := make([]schemas.UploadPart, 0, totalParts)
	lastIndex := int64(-1)

	iterator := utils.IterateChunks(file, partSize, fileSize)
	for {
		chunk, ok, err := iterator.Next()
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}

		chunkHash := utils.NewHash()
		chunkHash.Update(chunk)
		sha1 := chunkHash.DigestBase64()
		digest := "sha=" + sha1
		chunkSize := int64(len(chunk))
		bytesStart := lastIndex + 1
		bytesEnd := lastIndex + chunkSize
		contentRange := fmt.Sprintf("bytes %d-%d/%d", bytesStart, bytesEnd, fileSize)

		uploaded, err := m.UploadFilePartByURL(ctx, uploadPartURL, bytes.NewReader(chunk), digest, contentRange, nil)
		if err != nil {
			return nil, err
		}
		if uploaded.Part == nil {
			return nil, boxerrors.NewBoxSDKError("uploaded part response is missing the part")
		}
		part := uploaded.Part
		partSha1, err := utils.HexToBase64(part.Sha1)
		if err != nil {
			return nil, err
		}
		if partSha1 != sha1 {
			return nil, boxerrors.NewBoxSDKError("Assertion failed")
		}
		if part.Size == nil || *part.Size != chunkSize {
			return nil, boxerrors.NewBoxSDKError("Assertion failed")
		}
		if part.Offset == nil || *part.Offset != bytesStart {
			return nil, boxerrors.NewBoxSDKError("Assertion failed")
		}
		fileHash.Update(chunk)
		parts = append(parts, *part)
		lastIndex = bytesEnd
	}

	processedSessionParts, err := m.GetFileUploadSessionPartsByURL(ctx, listPartsURL, "", "", nil)
	if err != nil {
		return nil, err
	}
	if processedSessionParts.TotalCount == nil || *processedSessionParts.TotalCount != totalParts {
		return nil, boxerrors.NewBoxSDKError("Assertion failed")
	}

	digest := "sha=" + fileHash.DigestBase64()
	committed, err := m.CreateFileUploadSessionCommitByURL(ctx, commitURL, parts, digest, "", "", nil)
	if err != nil {
		return nil, err
	}
	if committed == nil || len(committed.Entries) == 0 {
		return nil, boxerrors.NewBoxSDKError("commit did not return a file")
	}
	return &committed.Entries[0], nil
}
