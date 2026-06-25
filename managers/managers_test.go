package managers

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	boxerrors "github.com/asalih/go-box-sdk/errors"
	"github.com/asalih/go-box-sdk/networking"
	"github.com/asalih/go-box-sdk/schemas"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testSession returns a NetworkSession whose base, upload, and OAuth2 URLs all
// point at the test server, so manager URL construction targets the mock.
func testSession(serverURL string) *networking.NetworkSession {
	session := networking.NewNetworkSession()
	session.BaseURLs = &networking.BaseURLs{
		BaseURL:   serverURL,
		UploadURL: serverURL,
		OAuth2URL: serverURL,
	}
	return session
}

func TestCreateFolder(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/2.0/folders", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"123","type":"folder","name":"My Folder"}`)
	}))
	defer srv.Close()

	mgr := NewFoldersManager(nil, testSession(srv.URL))
	folder, err := mgr.CreateFolder(context.Background(), &CreateFolderRequestBody{
		Name:   "My Folder",
		Parent: CreateFolderParent{ID: "0"},
	}, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "123", folder.ID)
	assert.Equal(t, "My Folder", folder.Name)
	// The request body must carry the snake_case parent.id.
	assert.Equal(t, "My Folder", got["name"])
	parent, ok := got["parent"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "0", parent["id"])
}

func TestGetFolderItems(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/2.0/folders/0/items", r.URL.Path)
		assert.Equal(t, "100", r.URL.Query().Get("limit"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"total_count":2,"entries":[
			{"type":"folder","id":"1","name":"sub"},
			{"type":"file","id":"2","name":"a.txt","size":10}
		]}`)
	}))
	defer srv.Close()

	mgr := NewFoldersManager(nil, testSession(srv.URL))
	items, err := mgr.GetFolderItems(context.Background(), "0", &GetFolderItemsOptions{Limit: "100"})
	require.NoError(t, err)
	require.NotNil(t, items.TotalCount)
	assert.EqualValues(t, 2, *items.TotalCount)
	require.Len(t, items.Entries, 2)
	assert.Equal(t, "folder", items.Entries[0].Type)
	assert.Equal(t, "file", items.Entries[1].Type)
	assert.Equal(t, "a.txt", items.Entries[1].Name)
}

func TestGetFileByID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/2.0/files/55", r.URL.Path)
		assert.Equal(t, "name,size", r.URL.Query().Get("fields"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"55","type":"file","name":"report.pdf","size":2048}`)
	}))
	defer srv.Close()

	mgr := NewFilesManager(nil, testSession(srv.URL))
	file, err := mgr.GetFileByID(context.Background(), "55", &GetFileByIDOptions{Fields: []string{"name", "size"}})
	require.NoError(t, err)
	assert.Equal(t, "55", file.ID)
	assert.Equal(t, "report.pdf", file.Name)
	require.NotNil(t, file.Size)
	assert.EqualValues(t, 2048, *file.Size)
}

func TestDeleteFolderByID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodDelete, r.Method)
		require.Equal(t, "/2.0/folders/9", r.URL.Path)
		assert.Equal(t, "true", r.URL.Query().Get("recursive"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	mgr := NewFoldersManager(nil, testSession(srv.URL))
	err := mgr.DeleteFolderByID(context.Background(), "9", &DeleteFolderByIDOptions{Recursive: "true"})
	require.NoError(t, err)
}

func TestUploadFileMultipartOrdering(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/2.0/files/content", r.URL.Path)
		require.True(t, strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data"))

		reader, err := r.MultipartReader()
		require.NoError(t, err)

		// The attributes part must come strictly before the file part.
		attrsPart, err := reader.NextPart()
		require.NoError(t, err)
		require.Equal(t, "attributes", attrsPart.FormName())
		var attrs map[string]any
		require.NoError(t, json.NewDecoder(attrsPart).Decode(&attrs))
		assert.Equal(t, "evidence.bin", attrs["name"])

		filePart, err := reader.NextPart()
		require.NoError(t, err)
		require.Equal(t, "file", filePart.FormName())
		content, err := io.ReadAll(filePart)
		require.NoError(t, err)
		assert.Equal(t, "hello world", string(content))

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"total_count":1,"entries":[{"id":"777","type":"file","name":"evidence.bin"}]}`)
	}))
	defer srv.Close()

	mgr := NewUploadsManager(nil, testSession(srv.URL))
	files, err := mgr.UploadFile(context.Background(), &UploadFileRequestBody{
		Attributes: UploadFileAttributes{Name: "evidence.bin", Parent: UploadFileAttributesParent{ID: "0"}},
		File:       strings.NewReader("hello world"),
	}, nil)
	require.NoError(t, err)
	require.Len(t, files.Entries, 1)
	assert.Equal(t, "777", files.Entries[0].ID)
}

func TestDownloadFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/2.0/files/55/content", r.URL.Path)
		_, _ = w.Write([]byte("binary-bytes"))
	}))
	defer srv.Close()

	mgr := NewDownloadsManager(nil, testSession(srv.URL))
	stream, err := mgr.DownloadFile(context.Background(), "55", nil)
	require.NoError(t, err)
	require.NotNil(t, stream)
	defer stream.Close()
	data, err := io.ReadAll(stream)
	require.NoError(t, err)
	assert.Equal(t, "binary-bytes", string(data))
}

func TestGetDownloadFileURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://dl.box.com/file/55")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	mgr := NewDownloadsManager(nil, testSession(srv.URL))
	url, err := mgr.GetDownloadFileURL(context.Background(), "55", nil)
	require.NoError(t, err)
	assert.Equal(t, "https://dl.box.com/file/55", url)
}

// parseContentRangeStart extracts the start byte from a "bytes <start>-<end>/<total>"
// content-range header.
func parseContentRangeStart(t *testing.T, header string) int64 {
	t.Helper()
	header = strings.TrimPrefix(header, "bytes ")
	dash := strings.Index(header, "-")
	require.GreaterOrEqual(t, dash, 0)
	start, err := strconv.ParseInt(header[:dash], 10, 64)
	require.NoError(t, err)
	return start
}

func TestUploadBigFileChunkedFlow(t *testing.T) {
	const partSize = 8
	content := []byte("0123456789abcdefghij") // 20 bytes -> parts of 8, 8, 4
	totalParts := 3

	var srv *httptest.Server
	var committedDigest string
	uploadedParts := 0

	mux := http.NewServeMux()
	mux.HandleFunc("/2.0/files/upload_sessions", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		resp := map[string]any{
			"id":                  "session-1",
			"type":                "upload_session",
			"part_size":           partSize,
			"total_parts":         totalParts,
			"num_parts_processed": 0,
			"session_endpoints": map[string]any{
				"upload_part": srv.URL + "/upload_part",
				"commit":      srv.URL + "/commit",
				"list_parts":  srv.URL + "/list_parts",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/upload_part", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPut, r.Method)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		start := parseContentRangeStart(t, r.Header.Get("content-range"))
		sum := sha1.Sum(body)
		uploadedParts++
		resp := map[string]any{
			"part": map[string]any{
				"part_id": fmt.Sprintf("part-%d", start),
				"offset":  start,
				"size":    len(body),
				"sha1":    hex.EncodeToString(sum[:]),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/list_parts", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fmt.Sprintf(`{"total_count":%d,"entries":[]}`, totalParts))
	})
	mux.HandleFunc("/commit", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		committedDigest = r.Header.Get("digest")
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		parts, ok := body["parts"].([]any)
		require.True(t, ok)
		require.Len(t, parts, totalParts)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"total_count":1,"entries":[{"id":"999","type":"file","name":"big.bin"}]}`)
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()

	mgr := NewChunkedUploadsManager(nil, testSession(srv.URL))
	file, err := mgr.UploadBigFile(context.Background(), strings.NewReader(string(content)), "big.bin", int64(len(content)), "0")
	require.NoError(t, err)
	assert.Equal(t, "999", file.ID)
	assert.Equal(t, totalParts, uploadedParts)

	// The commit digest is the base64 SHA1 of the whole file.
	wholeSum := sha1.Sum(content)
	expectedDigest := "sha=" + base64.StdEncoding.EncodeToString(wholeSum[:])
	assert.Equal(t, expectedDigest, committedDigest)
}

func TestUploadFileVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/2.0/files/55/content", r.URL.Path)
		assert.Equal(t, "etag-1", r.Header.Get("if-match"))

		reader, err := r.MultipartReader()
		require.NoError(t, err)
		attrsPart, err := reader.NextPart()
		require.NoError(t, err)
		require.Equal(t, "attributes", attrsPart.FormName())
		var attrs map[string]any
		require.NoError(t, json.NewDecoder(attrsPart).Decode(&attrs))
		assert.Equal(t, "v2.bin", attrs["name"])

		filePart, err := reader.NextPart()
		require.NoError(t, err)
		require.Equal(t, "file", filePart.FormName())
		content, err := io.ReadAll(filePart)
		require.NoError(t, err)
		assert.Equal(t, "new version", string(content))

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"total_count":1,"entries":[{"id":"55","type":"file","name":"v2.bin"}]}`)
	}))
	defer srv.Close()

	mgr := NewUploadsManager(nil, testSession(srv.URL))
	files, err := mgr.UploadFileVersion(context.Background(), "55", &UploadFileVersionRequestBody{
		Attributes: UploadFileVersionAttributes{Name: "v2.bin"},
		File:       strings.NewReader("new version"),
	}, &UploadFileVersionOptions{IfMatch: "etag-1"})
	require.NoError(t, err)
	require.Len(t, files.Entries, 1)
	assert.Equal(t, "55", files.Entries[0].ID)
}

func TestPreflightFileUploadCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodOptions, r.Method)
		require.Equal(t, "/2.0/files/content", r.URL.Path)
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "evidence.bin", body["name"])

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"upload_url":"https://upload.box.com/api/2.0/files/content?token=abc","upload_token":"abc"}`)
	}))
	defer srv.Close()

	size := int64(2048)
	mgr := NewUploadsManager(nil, testSession(srv.URL))
	res, err := mgr.PreflightFileUploadCheck(context.Background(), &PreflightFileUploadCheckRequestBody{
		Name:   "evidence.bin",
		Size:   &size,
		Parent: &PreflightFileUploadCheckParent{ID: "0"},
	}, nil)
	require.NoError(t, err)
	assert.Contains(t, res.UploadURL, "upload.box.com")
	assert.Equal(t, "abc", res.UploadToken)
}

func TestUploadWithPreflightCheck(t *testing.T) {
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/2.0/files/content", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodOptions, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fmt.Sprintf(`{"upload_url":%q}`, srv.URL+"/upload"))
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.True(t, strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"total_count":1,"entries":[{"id":"888","type":"file","name":"evidence.bin"}]}`)
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()

	mgr := NewUploadsManager(nil, testSession(srv.URL))
	files, err := mgr.UploadWithPreflightCheck(context.Background(), &UploadFileRequestBody{
		Attributes: UploadFileAttributes{Name: "evidence.bin", Parent: UploadFileAttributesParent{ID: "0"}},
		File:       strings.NewReader("hello world"),
	}, nil)
	require.NoError(t, err)
	require.Len(t, files.Entries, 1)
	assert.Equal(t, "888", files.Entries[0].ID)
}

func TestUploadWithPreflightCheckRejectsBadURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"upload_url":""}`)
	}))
	defer srv.Close()

	mgr := NewUploadsManager(nil, testSession(srv.URL))
	_, err := mgr.UploadWithPreflightCheck(context.Background(), &UploadFileRequestBody{
		Attributes: UploadFileAttributes{Name: "x", Parent: UploadFileAttributesParent{ID: "0"}},
		File:       strings.NewReader("x"),
	}, nil)
	require.Error(t, err)
}

func TestGetFileUploadSessionByID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/2.0/files/upload_sessions/S1", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"S1","type":"upload_session","part_size":8388608,"total_parts":3,"num_parts_processed":1}`)
	}))
	defer srv.Close()

	mgr := NewChunkedUploadsManager(nil, testSession(srv.URL))
	session, err := mgr.GetFileUploadSessionByID(context.Background(), "S1", nil)
	require.NoError(t, err)
	assert.Equal(t, "S1", session.ID)
	require.NotNil(t, session.NumPartsProcessed)
	assert.EqualValues(t, 1, *session.NumPartsProcessed)
}

func TestGetFileUploadSessionParts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/2.0/files/upload_sessions/S1/parts", r.URL.Path)
		assert.Equal(t, "0", r.URL.Query().Get("offset"))
		assert.Equal(t, "100", r.URL.Query().Get("limit"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"total_count":1,"entries":[{"part_id":"P1","offset":0,"size":8,"sha1":"abc"}]}`)
	}))
	defer srv.Close()

	mgr := NewChunkedUploadsManager(nil, testSession(srv.URL))
	parts, err := mgr.GetFileUploadSessionParts(context.Background(), "S1", "0", "100", nil)
	require.NoError(t, err)
	require.NotNil(t, parts.TotalCount)
	assert.EqualValues(t, 1, *parts.TotalCount)
	require.Len(t, parts.Entries, 1)
	assert.Equal(t, "P1", parts.Entries[0].PartID)
}

func TestDeleteFileUploadSessionByID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodDelete, r.Method)
		require.Equal(t, "/2.0/files/upload_sessions/S1", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	mgr := NewChunkedUploadsManager(nil, testSession(srv.URL))
	require.NoError(t, mgr.DeleteFileUploadSessionByID(context.Background(), "S1", nil))
}

func TestCreateFileUploadSessionCommit(t *testing.T) {
	t.Run("200_returns_file", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodPost, r.Method)
			require.Equal(t, "/2.0/files/upload_sessions/S1/commit", r.URL.Path)
			assert.Equal(t, "sha=abc", r.Header.Get("digest"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"total_count":1,"entries":[{"id":"999","type":"file","name":"big.bin"}]}`)
		}))
		defer srv.Close()

		mgr := NewChunkedUploadsManager(nil, testSession(srv.URL))
		files, err := mgr.CreateFileUploadSessionCommit(context.Background(), "S1", nil, "sha=abc", "", "", nil)
		require.NoError(t, err)
		require.NotNil(t, files)
		require.Len(t, files.Entries, 1)
		assert.Equal(t, "999", files.Entries[0].ID)
	})

	t.Run("202_returns_nil", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
		}))
		defer srv.Close()

		mgr := NewChunkedUploadsManager(nil, testSession(srv.URL))
		files, err := mgr.CreateFileUploadSessionCommit(context.Background(), "S1", nil, "sha=abc", "", "", nil)
		require.NoError(t, err)
		assert.Nil(t, files)
	})
}

func TestManagerAPIErrorPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"code":"not_found","message":"Not Found","request_id":"req-1"}`)
	}))
	defer srv.Close()

	mgr := NewFilesManager(nil, testSession(srv.URL))
	_, err := mgr.GetFileByID(context.Background(), "missing", nil)
	var apiErr *boxerrors.BoxAPIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusNotFound, apiErr.ResponseInfo.StatusCode)
	assert.Equal(t, "not_found", apiErr.ResponseInfo.Code)
	assert.Equal(t, "req-1", apiErr.ResponseInfo.RequestID)
}

func TestAuthorizationManagerRequestAccessToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/oauth2/token", r.URL.Path)
		require.NoError(t, r.ParseForm())
		assert.Equal(t, schemas.GrantTypeClientCredentials, r.PostForm.Get("grant_type"))
		assert.Equal(t, "client-id", r.PostForm.Get("client_id"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"at-1","token_type":"bearer","expires_in":3600}`)
	}))
	defer srv.Close()

	mgr := NewAuthorizationManager(nil, testSession(srv.URL))
	tok, err := mgr.RequestAccessToken(context.Background(), &schemas.PostOAuth2Token{
		GrantType: schemas.GrantTypeClientCredentials,
		ClientID:  "client-id",
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, "at-1", tok.AccessToken)
}

func TestAuthorizationManagerRefreshAccessToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The #refresh fragment is informational and not sent on the wire.
		require.Equal(t, "/oauth2/token", r.URL.Path)
		require.NoError(t, r.ParseForm())
		assert.Equal(t, schemas.GrantTypeRefreshToken, r.PostForm.Get("grant_type"))
		assert.Equal(t, "rt-old", r.PostForm.Get("refresh_token"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"at-refreshed","token_type":"bearer"}`)
	}))
	defer srv.Close()

	mgr := NewAuthorizationManager(nil, testSession(srv.URL))
	tok, err := mgr.RefreshAccessToken(context.Background(), &schemas.PostOAuth2Token{
		GrantType:    schemas.GrantTypeRefreshToken,
		RefreshToken: "rt-old",
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, "at-refreshed", tok.AccessToken)
}

func TestAuthorizationManagerRevokeAccessToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/oauth2/revoke", r.URL.Path)
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "tok-to-revoke", r.PostForm.Get("token"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	mgr := NewAuthorizationManager(nil, testSession(srv.URL))
	err := mgr.RevokeAccessToken(context.Background(), &schemas.PostOAuth2Revoke{
		ClientID: "c", ClientSecret: "s", Token: "tok-to-revoke",
	}, nil)
	require.NoError(t, err)
}

func TestGetFolderByID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/2.0/folders/42", r.URL.Path)
		assert.Equal(t, "name", r.URL.Query().Get("fields"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"42","type":"folder","name":"Case 42"}`)
	}))
	defer srv.Close()

	mgr := NewFoldersManager(nil, testSession(srv.URL))
	folder, err := mgr.GetFolderByID(context.Background(), "42", &GetFolderByIDOptions{Fields: []string{"name"}})
	require.NoError(t, err)
	assert.Equal(t, "42", folder.ID)
	assert.Equal(t, "Case 42", folder.Name)
}

func TestUploadFilePartByID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPut, r.Method)
		require.Equal(t, "/2.0/files/upload_sessions/S1", r.URL.Path)
		assert.Equal(t, "sha=abc", r.Header.Get("digest"))
		assert.Equal(t, "bytes 0-3/4", r.Header.Get("content-range"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"part":{"part_id":"P1","offset":0,"size":4,"sha1":"abc"}}`)
	}))
	defer srv.Close()

	mgr := NewChunkedUploadsManager(nil, testSession(srv.URL))
	part, err := mgr.UploadFilePart(context.Background(), "S1", strings.NewReader("data"), "sha=abc", "bytes 0-3/4", nil)
	require.NoError(t, err)
	require.NotNil(t, part.Part)
	assert.Equal(t, "P1", part.Part.PartID)
}

func TestDownloadFile202ReturnsNil(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	mgr := NewDownloadsManager(nil, testSession(srv.URL))
	stream, err := mgr.DownloadFile(context.Background(), "55", &DownloadFileOptions{Version: "v2", Range: "bytes=0-10"})
	require.NoError(t, err)
	assert.Nil(t, stream)
}

func TestGetDownloadFileURLNoLocationErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	mgr := NewDownloadsManager(nil, testSession(srv.URL))
	_, err := mgr.GetDownloadFileURL(context.Background(), "55", nil)
	require.Error(t, err)
}

// fastErrSession points at the given server but retries with zero backoff so
// error-path tests against an always-failing server stay fast.
func fastErrSession(serverURL string) *networking.NetworkSession {
	session := testSession(serverURL)
	session.RetryStrategy = &networking.BoxRetryStrategy{MaxAttempts: 2, RetryRandomizationFactor: 0.5, RetryBaseInterval: 0, MaxRetriesOnException: 1}
	return session
}

// TestManagerFetchErrorsPropagate verifies every manager surfaces the transport
// error from m.fetch instead of swallowing it. The server always returns 500.
func TestManagerFetchErrorsPropagate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"code":"internal_server_error","message":"boom"}`)
	}))
	defer srv.Close()

	ctx := context.Background()
	session := fastErrSession(srv.URL)

	cases := map[string]func() error{
		"RequestAccessToken": func() error {
			_, err := NewAuthorizationManager(nil, session).RequestAccessToken(ctx, &schemas.PostOAuth2Token{GrantType: schemas.GrantTypeClientCredentials}, nil)
			return err
		},
		"RefreshAccessToken": func() error {
			_, err := NewAuthorizationManager(nil, session).RefreshAccessToken(ctx, &schemas.PostOAuth2Token{GrantType: schemas.GrantTypeRefreshToken}, nil)
			return err
		},
		"CreateFolder": func() error {
			_, err := NewFoldersManager(nil, session).CreateFolder(ctx, &CreateFolderRequestBody{Name: "n", Parent: CreateFolderParent{ID: "0"}}, nil, nil)
			return err
		},
		"GetFolderByID": func() error {
			_, err := NewFoldersManager(nil, session).GetFolderByID(ctx, "1", nil)
			return err
		},
		"GetFolderItems": func() error {
			_, err := NewFoldersManager(nil, session).GetFolderItems(ctx, "1", nil)
			return err
		},
		"DeleteFolderByID": func() error {
			return NewFoldersManager(nil, session).DeleteFolderByID(ctx, "1", nil)
		},
		"GetFileByID": func() error {
			_, err := NewFilesManager(nil, session).GetFileByID(ctx, "1", nil)
			return err
		},
		"UploadFile": func() error {
			_, err := NewUploadsManager(nil, session).UploadFile(ctx, &UploadFileRequestBody{Attributes: UploadFileAttributes{Name: "n", Parent: UploadFileAttributesParent{ID: "0"}}, File: strings.NewReader("x")}, nil)
			return err
		},
		"UploadFileVersion": func() error {
			_, err := NewUploadsManager(nil, session).UploadFileVersion(ctx, "1", &UploadFileVersionRequestBody{Attributes: UploadFileVersionAttributes{Name: "n"}, File: strings.NewReader("x")}, nil)
			return err
		},
		"PreflightFileUploadCheck": func() error {
			_, err := NewUploadsManager(nil, session).PreflightFileUploadCheck(ctx, &PreflightFileUploadCheckRequestBody{Name: "n"}, nil)
			return err
		},
		"CreateFileUploadSession": func() error {
			_, err := NewChunkedUploadsManager(nil, session).CreateFileUploadSession(ctx, &CreateFileUploadSessionRequestBody{FolderID: "0", FileName: "n", FileSize: 1}, nil)
			return err
		},
		"GetFileUploadSessionByID": func() error {
			_, err := NewChunkedUploadsManager(nil, session).GetFileUploadSessionByID(ctx, "S1", nil)
			return err
		},
		"UploadFilePartByURL": func() error {
			_, err := NewChunkedUploadsManager(nil, session).UploadFilePartByURL(ctx, srv.URL+"/part", strings.NewReader("x"), "sha=x", "bytes 0-0/1", nil)
			return err
		},
		"GetFileUploadSessionPartsByURL": func() error {
			_, err := NewChunkedUploadsManager(nil, session).GetFileUploadSessionPartsByURL(ctx, srv.URL+"/parts", "", "", nil)
			return err
		},
		"CreateFileUploadSessionCommitByURL": func() error {
			_, err := NewChunkedUploadsManager(nil, session).CreateFileUploadSessionCommitByURL(ctx, srv.URL+"/commit", nil, "sha=x", "", "", nil)
			return err
		},
		"DownloadFile": func() error {
			_, err := NewDownloadsManager(nil, session).DownloadFile(ctx, "1", nil)
			return err
		},
		"GetDownloadFileURL": func() error {
			_, err := NewDownloadsManager(nil, session).GetDownloadFileURL(ctx, "1", nil)
			return err
		},
	}

	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			require.Error(t, call())
		})
	}
}

// TestFetchUsesDefaultClientWhenNil covers the fallback in baseManager.fetch
// that constructs a default network client when the session has none.
func TestFetchUsesDefaultClientWhenNil(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"7","type":"file","name":"f"}`)
	}))
	defer srv.Close()

	session := testSession(srv.URL)
	session.NetworkClient = nil // force the fetch fallback path

	file, err := NewFilesManager(nil, session).GetFileByID(context.Background(), "7", nil)
	require.NoError(t, err)
	assert.Equal(t, "7", file.ID)
}
