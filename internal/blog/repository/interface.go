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

	// CreateBlog inserts a new blog record and associates the given tags.
	CreateBlog(ctx context.Context, blog *schema.Blog, tagNames []string) error

	// UpdateBlog updates an existing blog's metadata and optionally its tags.
	// Pass a non-nil tagNames to replace tags; nil means no tag changes.
	UpdateBlog(ctx context.Context, id int64, updates map[string]interface{}, tagNames *[]string) error

	// DeleteBlog removes a blog record and its tag associations.
	DeleteBlog(ctx context.Context, id int64) error

	// ListTags returns all tags with their associated post counts.
	ListTags(ctx context.Context) ([]TagItem, error)

	// GetStats returns site-wide statistics (total posts, total unique tags).
	GetStats(ctx context.Context) (*SiteStats, error)

	// AutoMigrate ensures the table schema is up-to-date.
	AutoMigrate() error
}

// ──────────────────────────────────────────────
//  Query options
// ──────────────────────────────────────────────

// SearchParam carries optional filter & order options.
type SearchParam struct {
	Filter *BlogFilter
	Order  *Order
}

// BlogFilter holds optional query filters.
type BlogFilter struct {
	Title string // LIKE search on title
	Tag   string // exact match on tag name
}

// Order specifies the sort column and direction.
type Order struct {
	Field string
	Desc  bool
}

// ──────────────────────────────────────────────
//  DTOs (Data Transfer Objects)
// ──────────────────────────────────────────────

// SiteStats holds aggregate site statistics.
type SiteStats struct {
	PostCount int64 `json:"post_count"`
	TagCount  int64 `json:"tag_count"`
}

// BlogItem is the summary projection returned by list queries.
type BlogItem struct {
	ID        int64    `json:"id"`
	Title     string   `json:"title"`
	Summary   string   `json:"summary"`
	Date      string   `json:"date"`
	Tags      []string `json:"tags"`
	Cover     string   `json:"cover,omitempty"`
	Author    string   `json:"author,omitempty"`
	CreatedAt int64    `json:"created_at"` // first published to DB (unix seconds)
	UpdatedAt int64    `json:"updated_at"` // last metadata update (unix seconds)
}

// BlogDetail is the full projection returned by detail queries.
type BlogDetail struct {
	BlogItem
	Content string `json:"text"` // Markdown body read from filesystem
}

// TagItem is the projection returned by tag list queries.
type TagItem struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	PostCount int64  `json:"post_count"`
}

// ToBlogItem converts a schema.Blog to a BlogItem (summary only).
// The Blog must have Tags preloaded.
func ToBlogItem(b *schema.Blog) BlogItem {
	tagNames := make([]string, 0, len(b.Tags))
	for _, t := range b.Tags {
		tagNames = append(tagNames, t.Name)
	}
	return BlogItem{
		ID:        b.ID,
		Title:     b.Title,
		Summary:   b.Summary,
		Date:      b.Date,
		Tags:      tagNames,
		Cover:     b.Cover,
		Author:    b.Author,
		CreatedAt: b.CreatedAt,
		UpdatedAt: b.UpdatedAt,
	}
}
