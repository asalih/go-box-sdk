package schemas

// FileBaseType is the constant type value for files.
const FileBaseType = "file"

// FileVersionBaseType is the constant type value for file versions.
const FileVersionBaseType = "file_version"

// FileBase is the minimal file reference (id, etag, type).
type FileBase struct {
	ID   string  `json:"id"`
	Etag *string `json:"etag,omitempty"`
	Type string  `json:"type"`
}

// FileVersionBase is the minimal file-version reference.
type FileVersionBase struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// FileVersionMini extends FileVersionBase with the version's SHA1.
type FileVersionMini struct {
	FileVersionBase
	Sha1 string `json:"sha1,omitempty"`
}

// FileMini is the standard mini representation of a file.
type FileMini struct {
	FileBase
	SequenceID  string           `json:"sequence_id,omitempty"`
	Name        string           `json:"name,omitempty"`
	Sha1        string           `json:"sha1,omitempty"`
	FileVersion *FileVersionMini `json:"file_version,omitempty"`
}

// FilePathCollection lists the parent folders for a file.
type FilePathCollection struct {
	TotalCount int64        `json:"total_count"`
	Entries    []FolderMini `json:"entries"`
}

// File is the standard representation of a file returned by the API.
type File struct {
	FileMini
	Description       *string             `json:"description,omitempty"`
	Size              *int64              `json:"size,omitempty"`
	PathCollection    *FilePathCollection `json:"path_collection,omitempty"`
	CreatedAt         *string             `json:"created_at,omitempty"`
	ModifiedAt        *string             `json:"modified_at,omitempty"`
	TrashedAt         *string             `json:"trashed_at,omitempty"`
	PurgedAt          *string             `json:"purged_at,omitempty"`
	ContentCreatedAt  *string             `json:"content_created_at,omitempty"`
	ContentModifiedAt *string             `json:"content_modified_at,omitempty"`
	CreatedBy         *UserMini           `json:"created_by,omitempty"`
	ModifiedBy        *UserMini           `json:"modified_by,omitempty"`
	OwnedBy           *UserMini           `json:"owned_by,omitempty"`
	Parent            *FolderMini         `json:"parent,omitempty"`
	ItemStatus        *string             `json:"item_status,omitempty"`
}

// FileFull is the full representation of a file. The fields beyond File that
// the focused managers do not consume are retained verbatim in Metadata/Tags or
// ignored on decode; nothing on the wire is altered.
type FileFull struct {
	File
	CommentCount *int64         `json:"comment_count,omitempty"`
	Tags         []string       `json:"tags,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// Files is a collection of files, as returned by the upload endpoints.
type Files struct {
	TotalCount *int64     `json:"total_count,omitempty"`
	Entries    []FileFull `json:"entries,omitempty"`
}
