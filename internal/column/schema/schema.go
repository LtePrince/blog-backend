package schema

import blogschema "blog-backend/internal/blog/schema"

// Column represents a multi-chapter series ("专栏") in the columns table.
// Like blogs, only metadata lives in the DB; chapter Markdown bodies are read
// from the filesystem at request time using repo_dir + Path + Chapter.File.
type Column struct {
	ID      int64  `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	Slug    string `gorm:"type:varchar(200);uniqueIndex;not null;column:slug" json:"slug"` // URL slug, e.g. "zk-snark"
	Title   string `gorm:"type:varchar(255);not null;column:title" json:"title"`
	Summary string `gorm:"type:text;column:summary" json:"summary"`
	Author  string `gorm:"type:varchar(100);column:author" json:"author"`
	Path    string `gorm:"type:varchar(500);not null;column:path" json:"path"` // relative dir within repo, e.g. "columns/zk-snark"

	// Tags are shared with blog posts via the tags table, but associated
	// through a separate column_tags junction so post counts stay independent.
	Tags []blogschema.Tag `gorm:"many2many:column_tags" json:"tags"`

	// Chapters belong to the column and carry their own ordering.
	Chapters []Chapter `gorm:"foreignKey:ColumnID;constraint:OnDelete:CASCADE" json:"chapters"`

	CreatedAt int64 `gorm:"autoCreateTime;column:created_at" json:"created_at"`
	UpdatedAt int64 `gorm:"autoUpdateTime;column:updated_at" json:"updated_at"`
}

func (*Column) TableName() string {
	return "columns"
}

// Chapter is a single Markdown chapter within a column.
type Chapter struct {
	ID       int64  `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	ColumnID int64  `gorm:"index;not null;column:column_id" json:"column_id"`
	Slug     string `gorm:"type:varchar(200);not null;column:slug" json:"slug"`  // URL slug within the column, e.g. "intro"
	Title    string `gorm:"type:varchar(255);not null;column:title" json:"title"`
	File     string `gorm:"type:varchar(300);not null;column:file" json:"file"` // markdown filename within the column dir
	Sort     int    `gorm:"column:sort" json:"sort"`                            // display order (ascending)

	CreatedAt int64 `gorm:"autoCreateTime;column:created_at" json:"created_at"`
	UpdatedAt int64 `gorm:"autoUpdateTime;column:updated_at" json:"updated_at"`
}

func (*Chapter) TableName() string {
	return "column_chapters"
}
