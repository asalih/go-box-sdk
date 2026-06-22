package schemas

import (
	"encoding/json"
	"testing"

	"github.com/asalih/go-box-sdk/serialization"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeFileFull(t *testing.T) {
	const body = `{
		"type": "file",
		"id": "123",
		"etag": "1",
		"name": "evidence.bin",
		"sha1": "deadbeef",
		"size": 1024,
		"file_version": {"type": "file_version", "id": "v1", "sha1": "deadbeef"},
		"parent": {"type": "folder", "id": "0", "name": "All Files"},
		"content_created_at": "2024-01-02T03:04:05-08:00",
		"comment_count": 2,
		"tags": ["evidence", "case-1"]
	}`

	f, err := DecodeBytes[*FileFull]([]byte(body))
	require.NoError(t, err)

	// Fields flow through the FileBase -> FileMini -> File -> FileFull embedding.
	assert.Equal(t, "123", f.ID)
	assert.Equal(t, FileBaseType, f.Type)
	require.NotNil(t, f.Etag)
	assert.Equal(t, "1", *f.Etag)
	assert.Equal(t, "evidence.bin", f.Name)
	assert.Equal(t, "deadbeef", f.Sha1)
	require.NotNil(t, f.Size)
	assert.Equal(t, int64(1024), *f.Size)
	require.NotNil(t, f.FileVersion)
	assert.Equal(t, "v1", f.FileVersion.ID)
	require.NotNil(t, f.Parent)
	assert.Equal(t, "0", f.Parent.ID)
	require.NotNil(t, f.ContentCreatedAt)
	assert.Equal(t, "2024-01-02T03:04:05-08:00", *f.ContentCreatedAt)
	require.NotNil(t, f.CommentCount)
	assert.Equal(t, int64(2), *f.CommentCount)
	assert.Equal(t, []string{"evidence", "case-1"}, f.Tags)
}

func TestDecodeFolderFull(t *testing.T) {
	const body = `{
		"type": "folder",
		"id": "555",
		"name": "Case Files",
		"size": 0,
		"path_collection": {"total_count": 1, "entries": [{"type": "folder", "id": "0", "name": "All Files"}]},
		"parent": {"type": "folder", "id": "0", "name": "All Files"}
	}`

	folder, err := DecodeBytes[*FolderFull]([]byte(body))
	require.NoError(t, err)
	assert.Equal(t, "555", folder.ID)
	assert.Equal(t, FolderBaseType, folder.Type)
	assert.Equal(t, "Case Files", folder.Name)
	require.NotNil(t, folder.PathCollection)
	assert.Equal(t, int64(1), folder.PathCollection.TotalCount)
	require.Len(t, folder.PathCollection.Entries, 1)
	assert.Equal(t, "0", folder.PathCollection.Entries[0].ID)
}

func TestDecodeAccessToken(t *testing.T) {
	const body = `{
		"access_token": "tok-abc",
		"expires_in": 3600,
		"token_type": "bearer",
		"restricted_to": [{"scope": "item_upload", "object": {"id": "5", "type": "folder"}}]
	}`

	tok, err := DecodeBytes[*AccessToken]([]byte(body))
	require.NoError(t, err)
	assert.Equal(t, "tok-abc", tok.AccessToken)
	require.NotNil(t, tok.ExpiresIn)
	assert.Equal(t, int64(3600), *tok.ExpiresIn)
	assert.Equal(t, "bearer", tok.TokenType)
	require.Len(t, tok.RestrictedTo, 1)
	assert.Equal(t, "item_upload", tok.RestrictedTo[0].Scope)
	assert.Equal(t, "folder", tok.RestrictedTo[0].Object["type"])
}

func TestDecodeUploadSession(t *testing.T) {
	const body = `{
		"id": "S123",
		"type": "upload_session",
		"part_size": 8388608,
		"total_parts": 3,
		"num_parts_processed": 0,
		"session_endpoints": {
			"upload_part": "https://upload.box.com/api/2.0/files/upload_sessions/S123",
			"commit": "https://upload.box.com/api/2.0/files/upload_sessions/S123/commit",
			"list_parts": "https://upload.box.com/api/2.0/files/upload_sessions/S123/parts"
		}
	}`

	session, err := DecodeBytes[*UploadSession]([]byte(body))
	require.NoError(t, err)
	assert.Equal(t, "S123", session.ID)
	require.NotNil(t, session.PartSize)
	assert.Equal(t, int64(8388608), *session.PartSize)
	require.NotNil(t, session.TotalParts)
	assert.Equal(t, int64(3), *session.TotalParts)
	require.NotNil(t, session.NumPartsProcessed)
	assert.Equal(t, int64(0), *session.NumPartsProcessed)
	require.NotNil(t, session.SessionEndpoints)
	assert.Contains(t, session.SessionEndpoints.Commit, "/commit")
	assert.Contains(t, session.SessionEndpoints.ListParts, "/parts")
}

func TestDecodeUploadedPart(t *testing.T) {
	const body = `{"part": {"part_id": "P1", "offset": 0, "size": 8388608, "sha1": "abc123"}}`

	uploaded, err := DecodeBytes[*UploadedPart]([]byte(body))
	require.NoError(t, err)
	require.NotNil(t, uploaded.Part)
	assert.Equal(t, "P1", uploaded.Part.PartID)
	require.NotNil(t, uploaded.Part.Offset)
	assert.Equal(t, int64(0), *uploaded.Part.Offset)
	require.NotNil(t, uploaded.Part.Size)
	assert.Equal(t, int64(8388608), *uploaded.Part.Size)
	assert.Equal(t, "abc123", uploaded.Part.Sha1)
}

// TestDecodeFromSerializedData covers the generic Decode path used by the
// managers, which feeds already-parsed SerializedData (not raw bytes).
func TestDecodeFromSerializedData(t *testing.T) {
	parsed, err := serialization.JSONToSerializedData(`{"total_count":1,"entries":[{"type":"file","id":"9","name":"a.txt"}]}`)
	require.NoError(t, err)

	files, err := Decode[*Files](parsed)
	require.NoError(t, err)
	require.NotNil(t, files.TotalCount)
	assert.Equal(t, int64(1), *files.TotalCount)
	require.Len(t, files.Entries, 1)
	assert.Equal(t, "9", files.Entries[0].ID)
	assert.Equal(t, "a.txt", files.Entries[0].Name)
}

// TestUploadPartWireFidelity verifies the serialized form uses the exact
// snake_case keys Box expects and omits empty optional fields.
func TestUploadPartWireFidelity(t *testing.T) {
	part := UploadPart{
		UploadPartMini: UploadPartMini{PartID: "P1", Offset: Int64(0), Size: Int64(1024)},
		Sha1:           "abc",
	}
	b, err := json.Marshal(part)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	assert.Equal(t, "P1", m["part_id"])
	assert.Equal(t, float64(0), m["offset"])
	assert.Equal(t, float64(1024), m["size"])
	assert.Equal(t, "abc", m["sha1"])
}

func TestFileMiniOmitsEmptyOptionalFields(t *testing.T) {
	b, err := json.Marshal(FileMini{FileBase: FileBase{ID: "1", Type: FileBaseType}})
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	// id and type are always present; the optional fields are omitted.
	assert.Equal(t, "1", m["id"])
	assert.Equal(t, "file", m["type"])
	_, hasName := m["name"]
	assert.False(t, hasName)
	_, hasSha1 := m["sha1"]
	assert.False(t, hasSha1)
	_, hasFileVersion := m["file_version"]
	assert.False(t, hasFileVersion)
}

func TestHelperPointers(t *testing.T) {
	assert.Equal(t, "x", *String("x"))
	assert.Equal(t, int64(7), *Int64(7))
	assert.True(t, *Bool(true))
}
