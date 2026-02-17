package repository

import (
	"context"

	"blog-backend/internal/blog/schema"
)

// IBlogRepository defines the data-access interface for blog posts.
// Every method accepts context for cancellation / timeout propagation.
type IBlogRepository interface {
	// ListBlogs returns a page of blog summaries (without content body).
	// limit/offset implement pagination; searchParam carries optional filters & ordering.
	ListBlogs(ctx context.Context, limit, offset int, params ...SearchParam) ([]BlogItem, int64, error)

	// GetBlogByID returns the full blog (including content) for a single ID.
	GetBlogByID(ctx context.Context, id int64) (*BlogDetail, error)

	// AutoMigrate ensures the table schema is up-to-date.
	AutoMigrate() error
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
	Content string `json:"text"` // "text" keeps backward compat with old frontend key
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

// ToBlogDetail converts a schema.Blog to a BlogDetail (full).
func ToBlogDetail(b *schema.Blog) *BlogDetail {
	return &BlogDetail{
		BlogItem: ToBlogItem(b),
		Content:  b.Content,
	}
}
