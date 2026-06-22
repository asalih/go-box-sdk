package schemas

// UploadURL is returned by the preflight upload check.
type UploadURL struct {
	UploadURL   string `json:"upload_url,omitempty"`
	UploadToken string `json:"upload_token,omitempty"`
}

// UploadSessionSessionEndpoints holds the per-session endpoint URLs.
type UploadSessionSessionEndpoints struct {
	UploadPart string `json:"upload_part,omitempty"`
	Commit     string `json:"commit,omitempty"`
	Abort      string `json:"abort,omitempty"`
	ListParts  string `json:"list_parts,omitempty"`
	Status     string `json:"status,omitempty"`
	LogEvent   string `json:"log_event,omitempty"`
}

// UploadSession represents a chunked upload session.
type UploadSession struct {
	ID                string                         `json:"id,omitempty"`
	Type              string                         `json:"type,omitempty"`
	SessionExpiresAt  string                         `json:"session_expires_at,omitempty"`
	PartSize          *int64                         `json:"part_size,omitempty"`
	TotalParts        *int64                         `json:"total_parts,omitempty"`
	NumPartsProcessed *int64                         `json:"num_parts_processed,omitempty"`
	SessionEndpoints  *UploadSessionSessionEndpoints `json:"session_endpoints,omitempty"`
}

// UploadPartMini holds the minimal representation of an uploaded chunk.
type UploadPartMini struct {
	PartID string `json:"part_id,omitempty"`
	Offset *int64 `json:"offset,omitempty"`
	Size   *int64 `json:"size,omitempty"`
}

// UploadPart extends UploadPartMini with the chunk's SHA1 hash.
type UploadPart struct {
	UploadPartMini
	Sha1 string `json:"sha1,omitempty"`
}

// UploadedPart wraps the part returned after uploading a chunk.
type UploadedPart struct {
	Part *UploadPart `json:"part,omitempty"`
}

// UploadPartsOrder describes the ordering of a parts listing.
type UploadPartsOrder struct {
	By        string `json:"by,omitempty"`
	Direction string `json:"direction,omitempty"`
}

// UploadParts is a paginated listing of uploaded chunks.
type UploadParts struct {
	TotalCount *int64             `json:"total_count,omitempty"`
	Limit      *int64             `json:"limit,omitempty"`
	Offset     *int64             `json:"offset,omitempty"`
	Order      []UploadPartsOrder `json:"order,omitempty"`
	Entries    []UploadPart       `json:"entries,omitempty"`
}
