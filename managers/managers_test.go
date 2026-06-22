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

	"github.com/asalih/go-box-sdk/networking"
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
	data, err := io.ReadAll(stream)
	require.NoError(t, err)
	assert.Equal(t, "binary-bytes", string(data))
	if closer, ok := stream.(io.Closer); ok {
		_ = closer.Close()
	}
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
