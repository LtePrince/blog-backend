package schema

// Blog represents the blogs table in the database.
// The Markdown body is NOT stored in the DB — it is read from the filesystem
// at request time using repo_dir + Path.
type Blog struct {
	ID        int64  `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	Title     string `gorm:"type:varchar(255);not null;column:title" json:"title"`
	Summary   string `gorm:"type:text;column:summary" json:"summary"`
	Path      string `gorm:"type:varchar(500);not null;column:path;uniqueIndex" json:"path"` // relative dir within repo (e.g. "my-first-post")
	Date      string `gorm:"type:varchar(50);not null;column:date" json:"date"`
	Tags      string `gorm:"type:varchar(500);column:tags" json:"tags"`   // comma-separated
	Cover     string `gorm:"type:varchar(500);column:cover" json:"cover"` // relative path or URL
	Author    string `gorm:"type:varchar(100);column:author" json:"author"`
	CreatedAt int64  `gorm:"autoCreateTime;column:created_at" json:"created_at"`
	UpdatedAt int64  `gorm:"autoUpdateTime;column:updated_at" json:"updated_at"`
}

func (*Blog) TableName() string {
	return "blogs"
}
