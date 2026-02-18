package repository

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"blog-backend/internal/blog/schema"

	"gorm.io/gorm"
)

// relImageRe matches Markdown image references with relative paths like
// ![alt](./foo.jpg) or ![](./sub/bar.png)
var relImageRe = regexp.MustCompile(`(!\[[^\]]*\])\(\./([^)]+)\)`)

// blogRepository is the concrete GORM-backed implementation of IBlogRepository.
type blogRepository struct {
	db      *gorm.DB
	repoDir string // absolute path to blog content repo root
}

// NewBlogRepository creates a new IBlogRepository.
// repoDir is the absolute path to the blog content repository.
func NewBlogRepository(db *gorm.DB, repoDir string) IBlogRepository {
	return &blogRepository{db: db, repoDir: repoDir}
}

// AutoMigrate ensures the blogs table schema is in sync with the Go struct.
func (r *blogRepository) AutoMigrate() error {
	return r.db.AutoMigrate(&schema.Blog{})
}

// ListBlogs returns paginated blog summaries with optional filter & order.
func (r *blogRepository) ListBlogs(
	ctx context.Context,
	limit, offset int,
	params ...SearchParam,
) ([]BlogItem, int64, error) {
	query := r.db.WithContext(ctx).Model(&schema.Blog{})

	if len(params) > 0 {
		p := params[0]
		if p.Filter != nil && p.Filter.Title != "" {
			query = query.Where("title LIKE ?", "%"+p.Filter.Title+"%")
		}
		if p.Order != nil {
			dir := "ASC"
			if p.Order.Desc {
				dir = "DESC"
			}
			query = query.Order(fmt.Sprintf("%s %s", p.Order.Field, dir))
		} else {
			query = query.Order("date DESC")
		}
	} else {
		query = query.Order("date DESC")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count blogs: %w", err)
	}

	var blogs []schema.Blog
	if err := query.
		Select("id, title, summary, date, tags, cover, author").
		Limit(limit).Offset(offset).
		Find(&blogs).Error; err != nil {
		return nil, 0, fmt.Errorf("list blogs: %w", err)
	}

	items := make([]BlogItem, 0, len(blogs))
	for i := range blogs {
		items = append(items, ToBlogItem(&blogs[i]))
	}
	return items, total, nil
}

// GetBlogByID returns the full blog detail, reading Markdown content from disk.
func (r *blogRepository) GetBlogByID(ctx context.Context, id int64) (*BlogDetail, error) {
	var blog schema.Blog
	if err := r.db.WithContext(ctx).First(&blog, id).Error; err != nil {
		return nil, fmt.Errorf("get blog %d: %w", id, err)
	}

	// Read Markdown from: repoDir / path / index.md
	mdPath := filepath.Join(r.repoDir, blog.Path, "index.md")
	raw, err := os.ReadFile(mdPath)
	if err != nil {
		return nil, fmt.Errorf("read markdown %s: %w", mdPath, err)
	}

	// Replace relative image paths (./xxx) with absolute paths (/static/{path}/xxx)
	// so the browser can fetch images via the static file server.
	content := relImageRe.ReplaceAllString(string(raw), "${1}(/static/"+blog.Path+"/${2})")

	return &BlogDetail{
		BlogItem: ToBlogItem(&blog),
		Content:  content,
	}, nil
}

// CreateBlog inserts a new blog record.
func (r *blogRepository) CreateBlog(ctx context.Context, blog *schema.Blog) error {
	if err := r.db.WithContext(ctx).Create(blog).Error; err != nil {
		return fmt.Errorf("create blog: %w", err)
	}
	return nil
}

// UpdateBlog updates specific fields of an existing blog.
func (r *blogRepository) UpdateBlog(ctx context.Context, id int64, updates map[string]interface{}) error {
	result := r.db.WithContext(ctx).Model(&schema.Blog{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("update blog %d: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("blog %d not found", id)
	}
	return nil
}

// DeleteBlog removes a blog record by ID.
func (r *blogRepository) DeleteBlog(ctx context.Context, id int64) error {
	result := r.db.WithContext(ctx).Delete(&schema.Blog{}, id)
	if result.Error != nil {
		return fmt.Errorf("delete blog %d: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("blog %d not found", id)
	}
	return nil
}

// GetStats returns aggregate site statistics.
func (r *blogRepository) GetStats(ctx context.Context) (*SiteStats, error) {
	var postCount int64
	if err := r.db.WithContext(ctx).Model(&schema.Blog{}).Count(&postCount).Error; err != nil {
		return nil, fmt.Errorf("count posts: %w", err)
	}

	// Count unique tags: each blog stores comma-separated tags.
	var tagValues []string
	if err := r.db.WithContext(ctx).
		Model(&schema.Blog{}).
		Where("tags != ''").
		Pluck("tags", &tagValues).Error; err != nil {
		return nil, fmt.Errorf("pluck tags: %w", err)
	}

	uniqueTagSet := make(map[string]struct{})
	for _, raw := range tagValues {
		for _, t := range strings.Split(raw, ",") {
			tag := strings.TrimSpace(t)
			if tag != "" {
				uniqueTagSet[tag] = struct{}{}
			}
		}
	}

	return &SiteStats{
		PostCount: postCount,
		TagCount:  int64(len(uniqueTagSet)),
	}, nil
}
