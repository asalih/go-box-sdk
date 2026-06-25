//go:build integration

// Package box integration tests exercise the SDK against the live Box API. They
// are gated behind the `integration` build tag and skip automatically when the
// required credentials are not present in the environment. Run with:
//
//	CLIENT_ID=... CLIENT_SECRET=... ENTERPRISE_ID=... \
//	    go test -tags integration ./box/...
//
// or provide JWT_CONFIG_BASE_64 (a base64-encoded Box JWT config JSON) to use
// JWT auth, mirroring src/test/commons.ts.
package box

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"
	"os"
	"testing"

	"github.com/asalih/go-box-sdk/managers"
	"github.com/asalih/go-box-sdk/networking"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getEnvVar returns the named environment variable or skips the test when it is
// not set, mirroring the credential gating of the source integration suite.
func getEnvVar(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Skipf("skipping integration test: %s is not set", name)
	}
	return value
}

// getCcgAuth builds a CCG auth from the CLIENT_ID/CLIENT_SECRET/ENTERPRISE_ID
// environment variables. It mirrors commons.getCcgAuth.
func getCcgAuth(t *testing.T) *BoxCcgAuth {
	return NewBoxCcgAuth(CcgConfig{
		ClientID:     getEnvVar(t, "CLIENT_ID"),
		ClientSecret: getEnvVar(t, "CLIENT_SECRET"),
		EnterpriseID: getEnvVar(t, "ENTERPRISE_ID"),
	})
}

// getDefaultClient builds a client using JWT auth when JWT_CONFIG_BASE_64 is
// present, otherwise CCG auth. It mirrors commons.getDefaultClient (non-browser
// path prefers JWT).
func getDefaultClient(t *testing.T) *BoxClient {
	if encoded := os.Getenv("JWT_CONFIG_BASE_64"); encoded != "" {
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		require.NoError(t, err)
		config, err := JwtConfigFromConfigJSONString(string(decoded), nil, nil)
		require.NoError(t, err)
		return NewBoxClient(NewBoxJwtAuth(*config), nil)
	}
	return NewBoxClient(getCcgAuth(t), nil)
}

func TestIntegrationCcgAuthRetrievesToken(t *testing.T) {
	auth := getCcgAuth(t)
	header, err := auth.RetrieveAuthorizationHeader(context.Background(), networking.NewNetworkSession())
	require.NoError(t, err)
	assert.Contains(t, header, "Bearer ")
}

func TestIntegrationFolderCreateListDelete(t *testing.T) {
	client := getDefaultClient(t)
	ctx := context.Background()
	name := uuid.NewString()

	folder, err := client.Folders.CreateFolder(ctx, &managers.CreateFolderRequestBody{
		Name:   name,
		Parent: managers.CreateFolderParent{ID: "0"},
	}, nil, nil)
	require.NoError(t, err)
	require.NotEmpty(t, folder.ID)
	assert.Equal(t, name, folder.Name)

	fetched, err := client.Folders.GetFolderByID(ctx, folder.ID, nil)
	require.NoError(t, err)
	assert.Equal(t, folder.ID, fetched.ID)

	items, err := client.Folders.GetFolderItems(ctx, "0", &managers.GetFolderItemsOptions{Limit: "1000"})
	require.NoError(t, err)
	found := false
	for _, item := range items.Entries {
		if item.ID == folder.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "created folder should appear in root listing")

	require.NoError(t, client.Folders.DeleteFolderByID(ctx, folder.ID, &managers.DeleteFolderByIDOptions{Recursive: "true"}))
}

func TestIntegrationUploadDownload(t *testing.T) {
	client := getDefaultClient(t)
	ctx := context.Background()

	folder, err := client.Folders.CreateFolder(ctx, &managers.CreateFolderRequestBody{
		Name:   uuid.NewString(),
		Parent: managers.CreateFolderParent{ID: "0"},
	}, nil, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = client.Folders.DeleteFolderByID(context.Background(), folder.ID, &managers.DeleteFolderByIDOptions{Recursive: "true"})
	})

	content := make([]byte, 1024)
	_, err = rand.Read(content)
	require.NoError(t, err)
	fileName := uuid.NewString() + ".bin"

	files, err := client.Uploads.UploadFile(ctx, &managers.UploadFileRequestBody{
		Attributes: managers.UploadFileAttributes{Name: fileName, Parent: managers.UploadFileAttributesParent{ID: folder.ID}},
		File:       bytes.NewReader(content),
	}, nil)
	require.NoError(t, err)
	require.Len(t, files.Entries, 1)
	fileID := files.Entries[0].ID

	stream, err := client.Downloads.DownloadFile(ctx, fileID, nil)
	require.NoError(t, err)
	require.NotNil(t, stream)
	defer stream.Close()
	downloaded, err := io.ReadAll(stream)
	require.NoError(t, err)
	assert.Equal(t, content, downloaded)
}

func TestIntegrationChunkedUpload(t *testing.T) {
	client := getDefaultClient(t)
	ctx := context.Background()

	folder, err := client.Folders.CreateFolder(ctx, &managers.CreateFolderRequestBody{
		Name:   uuid.NewString(),
		Parent: managers.CreateFolderParent{ID: "0"},
	}, nil, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = client.Folders.DeleteFolderByID(context.Background(), folder.ID, &managers.DeleteFolderByIDOptions{Recursive: "true"})
	})

	// Box requires at least 20MB for an upload session.
	const fileSize = 20 * 1024 * 1024
	content := make([]byte, fileSize)
	_, err = rand.Read(content)
	require.NoError(t, err)
	fileName := uuid.NewString() + ".bin"

	file, err := client.ChunkedUploads.UploadBigFile(ctx, bytes.NewReader(content), fileName, int64(fileSize), folder.ID)
	require.NoError(t, err)
	require.NotNil(t, file)
	assert.Equal(t, fileName, file.Name)
}
