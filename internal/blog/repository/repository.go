package repository

import (
	"context"
	"errors"
	"fmt"
	"log"
	"maps"
	"strings"
	"time"

	"blog-backend/internal/blog/schema"
	"blog-backend/internal/content"

	"gorm.io/gorm"
)

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

// ──────────────────────────────────────────────
//  Schema migration
// ──────────────────────────────────────────────

// AutoMigrate ensures all table schemas are in sync with Go structs,
// and migrates legacy comma-separated tags to the normalised tags table.
func (r *blogRepository) AutoMigrate() error {
	if err := r.db.AutoMigrate(&schema.Tag{}, &schema.Blog{}); err != nil {
		return fmt.Errorf("auto-migrate: %w", err)
	}
	return r.migrateOldTags()
}

// migrateOldTags converts legacy comma-separated Blog.tags strings into
// normalised Tag rows and blog_tags associations.  It is idempotent —
// once the old column data is cleared, subsequent calls are no-ops.
func (r *blogRepository) migrateOldTags() error {
	// The old schema stored tags as a VARCHAR column "tags" in blogs.
	// After the struct change (Tags []Tag many2many), the column still
	// exists in SQLite but is no longer mapped by GORM.  Read via raw SQL.
	type oldRow struct {
		ID   int64
		Tags string
	}
	var rows []oldRow
	if err := r.db.Raw(
		"SELECT id, tags FROM blogs WHERE tags IS NOT NULL AND tags != ''",
	).Scan(&rows).Error; err != nil {
		// Column may not exist (fresh DB) — nothing to migrate.
		return nil
	}
	if len(rows) == 0 {
		return nil
	}

	log.Printf("🔄 Migrating %d blog(s) from comma-separated tags to normalised tags table…", len(rows))

	return r.db.Transaction(func(tx *gorm.DB) error {
		txRepo := &blogRepository{db: tx, repoDir: r.repoDir}
		for _, row := range rows {
			tags, err := txRepo.findOrCreateTags(context.Background(), splitCSV(row.Tags))
			if err != nil {
				return err
			}
			var blog schema.Blog
			blog.ID = row.ID
			if err := tx.Model(&blog).Association("Tags").Replace(tags); err != nil {
				return fmt.Errorf("associate tags for blog %d: %w", row.ID, err)
			}
		}
		// Clear the legacy column so migration doesn't run again.
		return tx.Exec("UPDATE blogs SET tags = ''").Error
	})
}

// ──────────────────────────────────────────────
//  Tag helpers
// ──────────────────────────────────────────────

// splitCSV splits a comma-separated string into trimmed, non-empty parts.
func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// findOrCreateTags returns Tag records for the given names, creating any
// that do not yet exist.
func (r *blogRepository) findOrCreateTags(ctx context.Context, names []string) ([]schema.Tag, error) {
	tags := make([]schema.Tag, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		var tag schema.Tag
		if err := r.db.WithContext(ctx).
			Where("name = ?", name).
			FirstOrCreate(&tag, schema.Tag{Name: name}).Error; err != nil {
			return nil, fmt.Errorf("find or create tag %q: %w", name, err)
		}
		tags = append(tags, tag)
	}
	return tags, nil
}

// ──────────────────────────────────────────────
//  Query scoping
// ──────────────────────────────────────────────

// Older rows may only have an authored date. Apply the same fallback before pagination.
const effectivePostTime = "COALESCE(NULLIF(blogs.updated_at,0), NULLIF(blogs.created_at,0), unixepoch(blogs.date), 0)"
const recentOrder = effectivePostTime + " DESC, blogs.id DESC"

// applySearchParams returns a GORM scope that applies filter & order options.
func applySearchParams(params []SearchParam) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if len(params) == 0 {
			return db.Order(recentOrder)
		}
		p := params[0]
		if p.Filter != nil {
			if p.Filter.Title != "" {
				db = db.Where("title LIKE ?", "%"+p.Filter.Title+"%")
			}
			if p.Filter.Tag != "" {
				db = db.
					Joins("JOIN blog_tags ON blog_tags.blog_id = blogs.id").
					Joins("JOIN tags ON tags.id = blog_tags.tag_id").
					Where("tags.name = ?", p.Filter.Tag)
			}
		}
		if p.Order != nil {
			dir := "ASC"
			if p.Order.Desc {
				dir = "DESC"
			}
			db = db.Order(fmt.Sprintf("%s %s", p.Order.Field, dir))
		} else {
			db = db.Order(recentOrder)
		}
		return db
	}
}

// ──────────────────────────────────────────────
//  Blog CRUD
// ──────────────────────────────────────────────

// ListBlogs returns paginated blog summaries with optional filter & order.
func (r *blogRepository) ListBlogs(
	ctx context.Context,
	limit, offset int,
	params ...SearchParam,
) ([]BlogItem, int64, error) {
	scope := applySearchParams(params)

	var total int64
	if err := r.db.WithContext(ctx).Model(&schema.Blog{}).Scopes(scope).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count blogs: %w", err)
	}

	var blogs []schema.Blog
	if err := r.db.WithContext(ctx).
		Preload("Tags").
		Scopes(scope).
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
	if err := r.db.WithContext(ctx).Preload("Tags").First(&blog, id).Error; err != nil {
		return nil, fmt.Errorf("get blog %d: %w", id, err)
	}

	// Read Markdown from: repoDir / path / index.md
	raw, err := content.ReadBody(r.repoDir, "posts", blog.Path, "index.md")
	if err != nil {
		return nil, err
	}
	body := content.RenderBody(raw, blog.Path)

	return &BlogDetail{
		BlogItem: ToBlogItem(&blog),
		Content:  body,
	}, nil
}

// CreateBlog inserts a new blog record and associates the given tags.
func (r *blogRepository) CreateBlog(ctx context.Context, blog *schema.Blog, tagNames []string) error {
	hash, err := content.BodyHash(r.repoDir, "posts", blog.Path, "index.md")
	if err != nil {
		return err
	}
	blog.ContentHash = hash
	blog.UpdatedAt = time.Now().Unix()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepo := &blogRepository{db: tx, repoDir: r.repoDir}
		tags, err := txRepo.findOrCreateTags(ctx, tagNames)
		if err != nil {
			return err
		}
		blog.Tags = tags
		return tx.Create(blog).Error
	})
}

// UpdateBlog updates specific fields of an existing blog and optionally
// replaces its tag associations.
func (r *blogRepository) UpdateBlog(
	ctx context.Context,
	id int64,
	updates map[string]interface{},
	tagNames *[]string,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var blog schema.Blog
		if err := tx.Preload("Tags").First(&blog, id).Error; err != nil {
			return err
		}
		dir := blog.Path
		if next, ok := updates["path"].(string); ok {
			dir = next
		}
		hash, err := content.BodyHash(r.repoDir, "posts", dir, "index.md")
		if err != nil {
			return err
		}
		current := map[string]string{"title": blog.Title, "date": blog.Date, "path": blog.Path, "summary": blog.Summary, "cover": blog.Cover, "author": blog.Author}
		fields := map[string]any{}
		for key, v := range updates {
			if v != current[key] {
				fields[key] = v
			}
		}
		changed := len(fields) > 0 || (blog.ContentHash != "" && hash != blog.ContentHash)
		if hash != blog.ContentHash {
			fields["content_hash"] = hash
		}
		if tagNames != nil {
			txRepo := &blogRepository{db: tx, repoDir: r.repoDir}
			tags, err := txRepo.findOrCreateTags(ctx, *tagNames)
			if err != nil {
				return err
			}
			oldSet := map[int64]bool{}
			newSet := map[int64]bool{}
			for _, t := range blog.Tags {
				oldSet[t.ID] = true
			}
			for _, t := range tags {
				newSet[t.ID] = true
			}
			if !maps.Equal(oldSet, newSet) {
				if err := tx.Model(&blog).Association("Tags").Replace(tags); err != nil {
					return err
				}
				changed = true
			}
		}
		if changed {
			fields["updated_at"] = max(time.Now().Unix(), blog.UpdatedAt+1)
		}
		if len(fields) == 0 {
			return nil
		}
		return tx.Model(&blog).UpdateColumns(fields).Error
	})
}

// DeleteBlog removes a blog record and its tag associations.
func (r *blogRepository) DeleteBlog(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Remove tag associations first.
		var blog schema.Blog
		blog.ID = id
		if err := tx.Model(&blog).Association("Tags").Clear(); err != nil {
			return fmt.Errorf("clear tags for blog %d: %w", id, err)
		}
		// Delete the blog record.
		result := tx.Delete(&schema.Blog{}, id)
		if result.Error != nil {
			return fmt.Errorf("delete blog %d: %w", id, result.Error)
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("blog %d not found", id)
		}

		// 自动删除未被关联的标签
		if err := tx.Exec(`DELETE FROM tags
			WHERE id NOT IN (SELECT tag_id FROM blog_tags)
			  AND id NOT IN (SELECT tag_id FROM column_tags)`).Error; err != nil {
			return fmt.Errorf("cleanup orphan tags: %w", err)
		}

		return nil
	})
}

// ──────────────────────────────────────────────
//  Tags
// ──────────────────────────────────────────────

// ListTags returns all tags along with their associated post counts.
func (r *blogRepository) ListTags(ctx context.Context) ([]TagItem, error) {
	var items []TagItem
	if err := r.db.WithContext(ctx).
		Model(&schema.Tag{}).
		Select("tags.id, tags.name, COUNT(blog_tags.blog_id) AS post_count").
		Joins("LEFT JOIN blog_tags ON blog_tags.tag_id = tags.id").
		Group("tags.id").
		Order("tags.name ASC").
		Scan(&items).Error; err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	return items, nil
}

// ──────────────────────────────────────────────
//  Site statistics
// ──────────────────────────────────────────────

// GetStats returns aggregate site statistics.
func (r *blogRepository) GetStats(ctx context.Context) (*SiteStats, error) {
	// Both aggregates return a row even when their table is empty.
	const query = `SELECT post_count, column_count, (SELECT COUNT(*) FROM tags) AS tag_count,
		MAX(COALESCE(latest_post, 0), COALESCE(latest_column, 0)) AS last_updated_at
		FROM (SELECT COUNT(*) AS post_count, MAX(` + effectivePostTime + `) AS latest_post FROM blogs)
		CROSS JOIN (SELECT COUNT(*) AS column_count, MAX(updated_at) AS latest_column FROM columns)`
	var stats SiteStats
	if err := r.db.WithContext(ctx).Raw(query).Scan(&stats).Error; err != nil {
		return nil, fmt.Errorf("get site stats: %w", err)
	}
	return &stats, nil
}

func (r *blogRepository) SeedContentHashes(ctx context.Context) error {
	db := r.db.WithContext(ctx)
	var blogs []schema.Blog
	if err := db.Where("COALESCE(content_hash, '') = ''").Find(&blogs).Error; err != nil {
		return err
	}
	var failures []error
	for _, blog := range blogs {
		hash, err := content.BodyHash(r.repoDir, "posts", blog.Path, "index.md")
		if err != nil {
			failures = append(failures, err)
			continue
		}
		err = db.Model(&blog).Where("path = ? AND COALESCE(content_hash, '') = ''", blog.Path).UpdateColumn("content_hash", hash).Error
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (r *blogRepository) HasImage(ctx context.Context, dir, asset string) (bool, error) {
	var blog schema.Blog
	err := r.db.WithContext(ctx).Where("path IN ?", content.StoredPaths(dir)).Take(&blog).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if content.AssetRef(blog.Cover, dir) == asset {
		return true, nil
	}
	raw, err := content.ReadBody(r.repoDir, "posts", blog.Path, "index.md")
	if err != nil {
		return false, err
	}
	return content.ReferencesImage(raw, dir, asset), nil
}
