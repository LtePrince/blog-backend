package repository

import (
	"context"
	"fmt"

	"blog-backend/internal/blog/schema"

	"gorm.io/gorm"
)

// blogRepository is the concrete GORM-backed implementation of IBlogRepository.
type blogRepository struct {
	db *gorm.DB
}

// NewBlogRepository creates a new IBlogRepository backed by GORM.
func NewBlogRepository(db *gorm.DB) IBlogRepository {
	return &blogRepository{db: db}
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

	// Apply filter & order when provided.
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

// GetBlogByID returns the full blog detail for a given ID.
func (r *blogRepository) GetBlogByID(ctx context.Context, id int64) (*BlogDetail, error) {
	var blog schema.Blog
	if err := r.db.WithContext(ctx).First(&blog, id).Error; err != nil {
		return nil, fmt.Errorf("get blog %d: %w", id, err)
	}
	return ToBlogDetail(&blog), nil
}
