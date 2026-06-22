package schemas

// ItemsOrder describes the ordering of an items listing.
type ItemsOrder struct {
	By        string `json:"by,omitempty"`
	Direction string `json:"direction,omitempty"`
}

// Item is an entry in a folder listing. The Box API returns a heterogeneous
// union of file, folder, and web_link entries; this flattened representation
// captures the fields common to all three (discriminated by Type), which is
// sufficient for repository-style directory listing.
type Item struct {
	Type        string           `json:"type"`
	ID          string           `json:"id"`
	Etag        *string          `json:"etag,omitempty"`
	SequenceID  string           `json:"sequence_id,omitempty"`
	Name        string           `json:"name,omitempty"`
	Sha1        string           `json:"sha1,omitempty"`
	FileVersion *FileVersionMini `json:"file_version,omitempty"`
	Size        *int64           `json:"size,omitempty"`
}

// Items is a paginated collection of folder entries.
type Items struct {
	Limit      *int64       `json:"limit,omitempty"`
	NextMarker *string      `json:"next_marker,omitempty"`
	PrevMarker *string      `json:"prev_marker,omitempty"`
	TotalCount *int64       `json:"total_count,omitempty"`
	Offset     *int64       `json:"offset,omitempty"`
	Order      []ItemsOrder `json:"order,omitempty"`
	Entries    []Item       `json:"entries,omitempty"`
}
