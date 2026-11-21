package repository

import (
	"context"
	"errors"
	"fmt"

	"strings"
	"time"

	blogschema "blog-backend/internal/blog/schema"
	"blog-backend/internal/column/schema"
	"blog-backend/internal/content"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

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
		Preload("Chapters", func(db *gorm.DB) *gorm.DB { return db.Select("id", "column_id") }).
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
		Preload("Chapters", func(db *gorm.DB) *gorm.DB { return db.Omit("published_markdown").Order("sort ASC, id ASC") }).
		Where("slug = ?", slug).
		First(&col).Error; err != nil {
		return nil, fmt.Errorf("get column %q: %w", slug, err)
	}
	return &col, nil
}

func toChapterItems(chs []schema.Chapter) []ChapterItem {
	items := make([]ChapterItem, 0, len(chs))
	for _, c := range chs {
		items = append(items, ChapterItem{File: c.File, ContentHash: c.ContentHash, Slug: c.Slug, Title: c.Title, Sort: c.Sort, UpdatedAt: c.UpdatedAt})
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

	var published schema.Chapter
	if err := r.db.WithContext(ctx).Select("slug", "title", "published_markdown").Where("published_markdown IS NOT NULL").First(&published, current.ID).Error; err != nil {
		return nil, err
	}
	body := content.RenderBody([]byte(published.PublishedMarkdown), col.Path)

	view := &ChapterView{
		Column:   ToColumnItem(col),
		Chapters: toChapterItems(col.Chapters),
		Current: ChapterContent{
			Slug:    published.Slug,
			Title:   published.Title,
			Content: body,
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

// CreateColumn inserts a column, or updates the existing one when a column
// with the same slug is already stored.
//
// Publishing is a check-then-act sequence on the client: it lists columns,
// finds no match, then creates. Two overlapping publishes both see "no match"
// and both create, and the second used to fail on the slug unique index even
// though its intent — make the remote match the local content — was perfectly
// satisfiable. Upserting lets repeated and concurrent publishes converge on
// the same state instead of erroring.
func (r *columnRepository) CreateColumn(
	ctx context.Context,
	col *schema.Column,
	tagNames []string,
	chapters []schema.Chapter,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Upsert the row by itself. Associations are replaced further down, so
		// that a conflicting insert cannot duplicate chapters or tag links.
		parent := *col
		parent.Tags = nil
		parent.Chapters = nil

		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "slug"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"title", "summary", "author", "path",
			}),
		}).Create(&parent).Error; err != nil {
			return fmt.Errorf("create column: %w", err)
		}

		// Re-read rather than trusting parent.ID: on the conflict path the
		// insert does not reliably return the existing row's id.
		var stored schema.Column
		if err := tx.Where("slug = ?", col.Slug).First(&stored).Error; err != nil {
			return fmt.Errorf("reload column %q: %w", col.Slug, err)
		}
		col.ID = stored.ID

		txRepo := &columnRepository{db: tx, repoDir: r.repoDir}
		tags, err := txRepo.findOrCreateTags(ctx, tagNames)
		if err != nil {
			return err
		}
		if err := tx.Model(&stored).Association("Tags").Replace(tags); err != nil {
			return fmt.Errorf("replace tags for column %q: %w", col.Slug, err)
		}

		if err := r.reconcileChapters(tx, &stored, chapters); err != nil {
			return err
		}

		return cleanupOrphanTags(tx)
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

		if dir, ok := updates["path"].(string); ok {
			col.Path = dir
		}
		if chapters != nil {
			if err := r.reconcileChapters(tx, &col, *chapters); err != nil {
				return err
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

// Match chapters by URL slug so metadata edits and reordering preserve their identity.
func (r *columnRepository) reconcileChapters(tx *gorm.DB, col *schema.Column, desired []schema.Chapter) error {
	var old []schema.Chapter
	if err := tx.Where("column_id = ?", col.ID).Find(&old).Error; err != nil {
		return err
	}
	bySlug := make(map[string]schema.Chapter, len(old))
	for _, ch := range old {
		bySlug[ch.Slug] = ch
	}
	seen := map[string]bool{}
	latest := int64(0)
	revisionTime := max(time.Now().Unix(), col.UpdatedAt+1)
	for i, ch := range desired {
		if ch.Slug == "" || seen[ch.Slug] {
			return fmt.Errorf("empty or duplicate chapter slug %q", ch.Slug)
		}
		seen[ch.Slug] = true
		prev, exists := bySlug[ch.Slug]
		ch.ID = prev.ID
		ch.CreatedAt = prev.CreatedAt
		ch.UpdatedAt = prev.UpdatedAt
		ch.ContentHash = prev.ContentHash
		ch.ColumnID = col.ID
		ch.Sort = i
		if err := r.checkBody(col.Path, &ch, revisionTime); err != nil {
			return err
		}
		if !exists || ch != prev {
			if err := tx.Save(&ch).Error; err != nil {
				return err
			}
		}
		latest = max(latest, ch.UpdatedAt)
	}
	for _, ch := range old {
		if !seen[ch.Slug] {
			if err := tx.Delete(&ch).Error; err != nil {
				return err
			}
		}
	}
	return tx.Model(col).UpdateColumn("updated_at", latest).Error
}

func (r *columnRepository) checkBody(dir string, ch *schema.Chapter, revisionTime int64) error {
	raw, err := content.ReadBody(r.repoDir, "columns", dir, ch.File)
	if err != nil {
		return err
	}
	ch.PublishedMarkdown = string(raw)
	hash := content.Hash(raw)
	if hash != ch.ContentHash {
		// Seed existing rows without dating the whole archive to migration day.
		if ch.ID == 0 || ch.ContentHash != "" {
			ch.UpdatedAt = max(revisionTime, ch.UpdatedAt+1)
		}
		ch.ContentHash = hash
	}
	return nil
}

// Seed snapshots for legacy rows from the version the old reader was serving.
// Subsequent pushes cannot replace a published snapshot.
func (r *columnRepository) SeedPublishedChapters(ctx context.Context) error {
	db := r.db.WithContext(ctx)
	var ids []int64
	if err := db.Model(&schema.Chapter{}).Where("published_markdown IS NULL").Pluck("id", &ids).Error; err != nil {
		return err
	}
	var failures []error
	for _, id := range ids {
		err := db.Transaction(func(tx *gorm.DB) error {
			var ch schema.Chapter
			if err := tx.Where("published_markdown IS NULL").First(&ch, id).Error; err != nil {
				return err
			}
			var col schema.Column
			if err := tx.First(&col, ch.ColumnID).Error; err != nil {
				return err
			}
			// The old reader served these bytes directly from disk. Preserve its timestamp at migration.
			raw, err := content.ReadBody(r.repoDir, "columns", col.Path, ch.File)
			if err != nil {
				return err
			}
			ch.PublishedMarkdown = string(raw)
			ch.ContentHash = content.Hash(raw)
			if err := tx.Save(&ch).Error; err != nil {
				return err
			}
			return setLatestChapterTime(tx, col.ID)
		})
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (r *columnRepository) HasImage(ctx context.Context, dir, asset string) (bool, error) {
	var bodies []string
	err := r.db.WithContext(ctx).Table("column_chapters").
		Joins("JOIN columns ON columns.id = column_chapters.column_id").
		Where("columns.path IN ? AND published_markdown IS NOT NULL", content.StoredPaths(dir)).
		Pluck("published_markdown", &bodies).Error
	if err != nil {
		return false, err
	}
	for _, body := range bodies {
		if content.ReferencesImage([]byte(body), dir, asset) {
			return true, nil
		}
	}
	return false, nil
}
