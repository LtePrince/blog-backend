package schema

// Tag represents the tags table in the database.
// Tags are shared across blog posts via a many-to-many relationship (blog_tags).
type Tag struct {
	ID        int64  `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	Name      string `gorm:"type:varchar(100);uniqueIndex;not null;column:name" json:"name"`
	CreatedAt int64  `gorm:"autoCreateTime;column:created_at" json:"created_at"`
}

func (*Tag) TableName() string {
	return "tags"
}
