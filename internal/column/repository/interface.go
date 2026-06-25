package repository

import (
	"context"

	"blog-backend/internal/column/schema"
)

// IColumnRepository defines the data-access interface for columns (专栏).
type IColumnRepository interface {
	// ListColumns returns a page of column summaries.
	ListColumns(ctx context.Context, limit, offset int, params ...SearchParam) ([]ColumnItem, int64, error)

	// GetColumnBySlug returns a column with its chapter list (no bodies).
	GetColumnBySlug(ctx context.Context, slug string) (*ColumnDetail, error)

	// GetChapter returns a single chapter view: column meta, the full chapter
	// list (for sidebar nav), the current chapter's Markdown body, and the
	// prev/next chapter slugs.
	GetChapter(ctx context.Context, columnSlug, chapterSlug string) (*ChapterView, error)

	// CreateColumn inserts a column with its tags and chapters.
	CreateColumn(ctx context.Context, col *schema.Column, tagNames []string, chapters []schema.Chapter) error

	// UpdateColumn replaces an existing column's metadata, tags and chapters.
	UpdateColumn(ctx context.Context, slug string, updates map[string]interface{}, tagNames *[]string, chapters *[]schema.Chapter) error

	// DeleteColumn removes a column, its chapters and tag associations.
	DeleteColumn(ctx context.Context, slug string) error

	// ListTags returns all column tags with their associated column counts.
	ListTags(ctx context.Context) ([]TagItem, error)

	// AutoMigrate ensures the column tables exist.
	AutoMigrate() error
}

// ──────────────────────────────────────────────
//  Query options (mirror the blog repository)
// ──────────────────────────────────────────────

// SearchParam carries optional filter & order options.
type SearchParam struct {
	Filter *ColumnFilter
	Order  *Order
}

// ColumnFilter holds optional query filters.
type ColumnFilter struct {
	Title string // LIKE search on title
	Tag   string // exact match on tag name
}

// Order specifies the sort column and direction.
type Order struct {
	Field string
	Desc  bool
}

// ──────────────────────────────────────────────
//  DTOs
// ──────────────────────────────────────────────

// ColumnItem is the summary projection returned by list queries.
type ColumnItem struct {
	ID           int64    `json:"id"`
	Slug         string   `json:"slug"`
	Title        string   `json:"title"`
	Summary      string   `json:"summary"`
	Author       string   `json:"author,omitempty"`
	Tags         []string `json:"tags"`
	ChapterCount int      `json:"chapter_count"`
	UpdatedAt    int64    `json:"updated_at"` // last publish/update (unix seconds)
}

// ChapterItem is a chapter entry in a column's table of contents.
type ChapterItem struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
	Sort  int    `json:"sort"`
}

// ColumnDetail is a column with its ordered chapter list (no bodies).
type ColumnDetail struct {
	ColumnItem
	Chapters []ChapterItem `json:"chapters"`
}

// ChapterContent is a single chapter's body.
type ChapterContent struct {
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	Content string `json:"text"` // Markdown body read from filesystem
}

// ChapterView powers the chapter reading page in one request.
type ChapterView struct {
	Column   ColumnItem     `json:"column"`
	Chapters []ChapterItem  `json:"chapters"`
	Current  ChapterContent `json:"current"`
	PrevSlug string         `json:"prev_slug,omitempty"`
	NextSlug string         `json:"next_slug,omitempty"`
}

// TagItem is the projection returned by tag list queries.
type TagItem struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	ColumnCount int64  `json:"column_count"`
}

// ToColumnItem converts a schema.Column (with Tags & Chapters preloaded) to a
// ColumnItem summary.
func ToColumnItem(c *schema.Column) ColumnItem {
	tagNames := make([]string, 0, len(c.Tags))
	for _, t := range c.Tags {
		tagNames = append(tagNames, t.Name)
	}
	return ColumnItem{
		ID:           c.ID,
		Slug:         c.Slug,
		Title:        c.Title,
		Summary:      c.Summary,
		Author:       c.Author,
		Tags:         tagNames,
		ChapterCount: len(c.Chapters),
		UpdatedAt:    c.UpdatedAt,
	}
}
