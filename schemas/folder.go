package schemas

// FolderBaseType is the constant type value for folders.
const FolderBaseType = "folder"

// FolderBase is the minimal folder reference (id, etag, type).
type FolderBase struct {
	ID   string  `json:"id"`
	Etag *string `json:"etag,omitempty"`
	Type string  `json:"type"`
}

// FolderMini is the standard mini representation of a folder.
type FolderMini struct {
	FolderBase
	SequenceID string `json:"sequence_id,omitempty"`
	Name       string `json:"name,omitempty"`
}

// FolderPathCollection lists the parent folders for a folder.
type FolderPathCollection struct {
	TotalCount int64        `json:"total_count"`
	Entries    []FolderMini `json:"entries"`
}

// Folder is the standard representation of a folder returned by the API.
type Folder struct {
	FolderMini
	Description       *string               `json:"description,omitempty"`
	Size              *int64                `json:"size,omitempty"`
	PathCollection    *FolderPathCollection `json:"path_collection,omitempty"`
	CreatedAt         *string               `json:"created_at,omitempty"`
	ModifiedAt        *string               `json:"modified_at,omitempty"`
	TrashedAt         *string               `json:"trashed_at,omitempty"`
	PurgedAt          *string               `json:"purged_at,omitempty"`
	ContentCreatedAt  *string               `json:"content_created_at,omitempty"`
	ContentModifiedAt *string               `json:"content_modified_at,omitempty"`
	CreatedBy         *UserMini             `json:"created_by,omitempty"`
	ModifiedBy        *UserMini             `json:"modified_by,omitempty"`
	OwnedBy           *UserMini             `json:"owned_by,omitempty"`
	Parent            *FolderMini           `json:"parent,omitempty"`
	ItemStatus        *string               `json:"item_status,omitempty"`
}

// FolderFull is the full representation of a folder. Full-only fields not
// consumed by the focused managers are retained in Metadata or ignored on
// decode; nothing on the wire is altered.
type FolderFull struct {
	Folder
	Metadata map[string]any `json:"metadata,omitempty"`
}
