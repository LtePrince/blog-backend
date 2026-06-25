package repository

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	blogschema "blog-backend/internal/blog/schema"
	"blog-backend/internal/column/schema"

	"gorm.io/gorm"
)

// relImageRe matches Markdown image references with relative paths like
// ![alt](./foo.jpg) — same rewrite rule as the blog repository.
var relImageRe = regexp.MustCompile(`(!\[[^\]]*\])\(\./([^)]+)\)`)

// columnRepository is the GORM-backed implementation of IColumnRepository.
type columnRepository struct {
	db      *gorm.DB
	repoDir string
}

// NewColumnRepository creates a new IColumnRepository.
func NewColumnRepository(db *gorm.DB, repoDir string) IColumnRepository {
	return &columnRepository{db: db, repoDir: repoDir}
}

// AutoMigrate ensures the column tables exist. The tags table itself is owned
// by the blog module; here we only add columns, column_chapters and the
// column_tags junction.
func (r *columnRepository) AutoMigrate() error {
	if err := r.db.AutoMigrate(&schema.Column{}, &schema.Chapter{}); err != nil {
		return fmt.Errorf("auto-migrate columns: %w", err)
	}
	return nil
}

// ──────────────────────────────────────────────
//  Tag helpers (tags table shared with blog posts)
// ──────────────────────────────────────────────

func (r *columnRepository) findOrCreateTags(ctx context.Context, names []string) ([]blogschema.Tag, error) {
	tags := make([]blogschema.Tag, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		var tag blogschema.Tag
		if err := r.db.WithContext(ctx).
			Where("name = ?", name).
			FirstOrCreate(&tag, blogschema.Tag{Name: name}).Error; err != nil {
			return nil, fmt.Errorf("find or create tag %q: %w", name, err)
		}
		tags = append(tags, tag)
	}
	return tags, nil
}

// cleanupOrphanTags removes tags no longer referenced by any blog or column.
func cleanupOrphanTags(tx *gorm.DB) error {
	return tx.Exec(`DELETE FROM tags
		WHERE id NOT IN (SELECT tag_id FROM blog_tags)
		  AND id NOT IN (SELECT tag_id FROM column_tags)`).Error
}

// ──────────────────────────────────────────────
//  Query scoping
// ──────────────────────────────────────────────

func applySearchParams(params []SearchParam) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if len(params) == 0 {
			return db.Order("updated_at DESC")
		}
		p := params[0]
		if p.Filter != nil {
			if p.Filter.Title != "" {
				db = db.Where("title LIKE ?", "%"+p.Filter.Title+"%")
			}
			if p.Filter.Tag != "" {
				db = db.
					Joins("JOIN column_tags ON column_tags.column_id = columns.id").
					Joins("JOIN tags ON tags.id = column_tags.tag_id").
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
			db = db.Order("updated_at DESC")
		}
		return db
	}
}

// ──────────────────────────────────────────────
//  Column CRUD
// ──────────────────────────────────────────────

func (r *columnRepository) ListColumns(
	ctx context.Context,
	limit, offset int,
	params ...SearchParam,
) ([]ColumnItem, int64, error) {
	scope := applySearchParams(params)

	var total int64
	if err := r.db.WithContext(ctx).Model(&schema.Column{}).Scopes(scope).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count columns: %w", err)
	}

	var cols []schema.Column
	if err := r.db.WithContext(ctx).
		Preload("Tags").
		Preload("Chapters").
		Scopes(scope).
		Limit(limit).Offset(offset).
		Find(&cols).Error; err != nil {
		return nil, 0, fmt.Errorf("list columns: %w", err)
	}

	items := make([]ColumnItem, 0, len(cols))
	for i := range cols {
		items = append(items, ToColumnItem(&cols[i]))
	}
	return items, total, nil
}

func (r *columnRepository) loadColumn(ctx context.Context, slug string) (*schema.Column, error) {
	var col schema.Column
	if err := r.db.WithContext(ctx).
		Preload("Tags").
		Preload("Chapters", func(db *gorm.DB) *gorm.DB { return db.Order("sort ASC") }).
		Where("slug = ?", slug).
		First(&col).Error; err != nil {
		return nil, fmt.Errorf("get column %q: %w", slug, err)
	}
	return &col, nil
}

func toChapterItems(chs []schema.Chapter) []ChapterItem {
	items := make([]ChapterItem, 0, len(chs))
	for _, c := range chs {
		items = append(items, ChapterItem{Slug: c.Slug, Title: c.Title, Sort: c.Sort})
	}
	return items
}

func (r *columnRepository) GetColumnBySlug(ctx context.Context, slug string) (*ColumnDetail, error) {
	col, err := r.loadColumn(ctx, slug)
	if err != nil {
		return nil, err
	}
	return &ColumnDetail{
		ColumnItem: ToColumnItem(col),
		Chapters:   toChapterItems(col.Chapters),
	}, nil
}

func (r *columnRepository) GetChapter(ctx context.Context, columnSlug, chapterSlug string) (*ChapterView, error) {
	col, err := r.loadColumn(ctx, columnSlug)
	if err != nil {
		return nil, err
	}

	// Chapters are preloaded ordered by sort ASC.
	idx := -1
	for i := range col.Chapters {
		if col.Chapters[i].Slug == chapterSlug {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("chapter %q not found in column %q", chapterSlug, columnSlug)
	}
	current := col.Chapters[idx]

	mdPath := filepath.Join(r.repoDir, col.Path, current.File)
	raw, err := os.ReadFile(mdPath)
	if err != nil {
		return nil, fmt.Errorf("read chapter markdown %s: %w", mdPath, err)
	}
	content := relImageRe.ReplaceAllString(string(raw), "${1}(/static/"+col.Path+"/${2})")

	view := &ChapterView{
		Column:   ToColumnItem(col),
		Chapters: toChapterItems(col.Chapters),
		Current: ChapterContent{
			Slug:    current.Slug,
			Title:   current.Title,
			Content: content,
		},
	}
	if idx > 0 {
		view.PrevSlug = col.Chapters[idx-1].Slug
	}
	if idx < len(col.Chapters)-1 {
		view.NextSlug = col.Chapters[idx+1].Slug
	}
	return view, nil
}

func (r *columnRepository) CreateColumn(
	ctx context.Context,
	col *schema.Column,
	tagNames []string,
	chapters []schema.Chapter,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepo := &columnRepository{db: tx, repoDir: r.repoDir}
		tags, err := txRepo.findOrCreateTags(ctx, tagNames)
		if err != nil {
			return err
		}
		col.Tags = tags
		normalizeChapterSort(chapters)
		col.Chapters = chapters

		if err := tx.Create(col).Error; err != nil {
			return fmt.Errorf("create column: %w", err)
		}
		return nil
	})
}

func (r *columnRepository) UpdateColumn(
	ctx context.Context,
	slug string,
	updates map[string]interface{},
	tagNames *[]string,
	chapters *[]schema.Chapter,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var col schema.Column
		if err := tx.Where("slug = ?", slug).First(&col).Error; err != nil {
			return fmt.Errorf("column %q not found: %w", slug, err)
		}

		if len(updates) > 0 {
			if err := tx.Model(&schema.Column{}).Where("id = ?", col.ID).Updates(updates).Error; err != nil {
				return fmt.Errorf("update column %q: %w", slug, err)
			}
		}

		if tagNames != nil {
			txRepo := &columnRepository{db: tx, repoDir: r.repoDir}
			tags, err := txRepo.findOrCreateTags(ctx, *tagNames)
			if err != nil {
				return err
			}
			if err := tx.Model(&col).Association("Tags").Replace(tags); err != nil {
				return fmt.Errorf("replace tags for column %q: %w", slug, err)
			}
		}

		if chapters != nil {
			// Replace the whole chapter set.
			if err := tx.Where("column_id = ?", col.ID).Delete(&schema.Chapter{}).Error; err != nil {
				return fmt.Errorf("clear chapters for column %q: %w", slug, err)
			}
			newChapters := *chapters
			normalizeChapterSort(newChapters)
			for i := range newChapters {
				newChapters[i].ID = 0
				newChapters[i].ColumnID = col.ID
			}
			if len(newChapters) > 0 {
				if err := tx.Create(&newChapters).Error; err != nil {
					return fmt.Errorf("recreate chapters for column %q: %w", slug, err)
				}
			}
		}

		return cleanupOrphanTags(tx)
	})
}

func (r *columnRepository) DeleteColumn(ctx context.Context, slug string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var col schema.Column
		if err := tx.Where("slug = ?", slug).First(&col).Error; err != nil {
			return fmt.Errorf("column %q not found: %w", slug, err)
		}
		if err := tx.Model(&col).Association("Tags").Clear(); err != nil {
			return fmt.Errorf("clear tags for column %q: %w", slug, err)
		}
		if err := tx.Where("column_id = ?", col.ID).Delete(&schema.Chapter{}).Error; err != nil {
			return fmt.Errorf("delete chapters for column %q: %w", slug, err)
		}
		if err := tx.Delete(&schema.Column{}, col.ID).Error; err != nil {
			return fmt.Errorf("delete column %q: %w", slug, err)
		}
		return cleanupOrphanTags(tx)
	})
}

// ──────────────────────────────────────────────
//  Tags
// ──────────────────────────────────────────────

func (r *columnRepository) ListTags(ctx context.Context) ([]TagItem, error) {
	var items []TagItem
	if err := r.db.WithContext(ctx).
		Table("tags").
		Select("tags.id, tags.name, COUNT(column_tags.column_id) AS column_count").
		Joins("JOIN column_tags ON column_tags.tag_id = tags.id").
		Group("tags.id").
		Order("tags.name ASC").
		Scan(&items).Error; err != nil {
		return nil, fmt.Errorf("list column tags: %w", err)
	}
	if items == nil {
		items = []TagItem{}
	}
	return items, nil
}

// normalizeChapterSort assigns an ascending Sort based on slice order so the
// meta.yaml chapter order stays authoritative.
func normalizeChapterSort(chapters []schema.Chapter) {
	for i := range chapters {
		chapters[i].Sort = i
	}
}
