package repository

import (
	"context"

	"blog-backend/internal/blog/schema"
)

// IBlogRepository defines the data-access interface for blog posts.
type IBlogRepository interface {
	// ListBlogs returns a page of blog summaries.
	ListBlogs(ctx context.Context, limit, offset int, params ...SearchParam) ([]BlogItem, int64, error)

	// GetBlogByID returns the full blog (content read from disk).
	GetBlogByID(ctx context.Context, id int64) (*BlogDetail, error)

	// CreateBlog inserts a new blog record.
	CreateBlog(ctx context.Context, blog *schema.Blog) error

	// UpdateBlog updates an existing blog's metadata.
	UpdateBlog(ctx context.Context, id int64, updates map[string]interface{}) error

	// DeleteBlog removes a blog record.
	DeleteBlog(ctx context.Context, id int64) error

	// GetStats returns site-wide statistics (total posts, total unique tags).
	GetStats(ctx context.Context) (*SiteStats, error)

	// AutoMigrate ensures the table schema is up-to-date.
	AutoMigrate() error
}

// SiteStats holds aggregate site statistics.
type SiteStats struct {
	PostCount int64 `json:"post_count"`
	TagCount  int64 `json:"tag_count"`
}

// SearchParam carries optional filter & order options.
type SearchParam struct {
	Filter *BlogFilter
	Order  *Order
}

// BlogFilter holds optional query filters.
type BlogFilter struct {
	Title string // LIKE search on title
}

// Order specifies the sort column and direction.
type Order struct {
	Field string
	Desc  bool
}

// BlogItem is the summary projection returned by list queries.
type BlogItem struct {
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Date    string `json:"date"`
	Tags    string `json:"tags,omitempty"`
	Cover   string `json:"cover,omitempty"`
	Author  string `json:"author,omitempty"`
}

// BlogDetail is the full projection returned by detail queries.
type BlogDetail struct {
	BlogItem
	Content string `json:"text"` // Markdown body read from filesystem
}

// ToBlogItem converts a schema.Blog to a BlogItem (summary only).
func ToBlogItem(b *schema.Blog) BlogItem {
	return BlogItem{
		ID:      b.ID,
		Title:   b.Title,
		Summary: b.Summary,
		Date:    b.Date,
		Tags:    b.Tags,
		Cover:   b.Cover,
		Author:  b.Author,
	}
}
