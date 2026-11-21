package repository

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"blog-backend/internal/column/schema"

	"gorm.io/gorm"
)

// PublishChapter updates just the selected chapter. The expected hash comes from
// the manager's local file so a lagging deployment cannot publish stale bytes.
func (r *columnRepository) PublishChapter(ctx context.Context, slug string, chapter schema.Chapter, expectedHash string, following []string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var col schema.Column
		if err := tx.Where("slug = ?", slug).First(&col).Error; err != nil {
			return err
		}
		var stored schema.Chapter
		err := tx.Where("column_id = ? AND slug = ?", col.ID, chapter.Slug).First(&stored).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		stored.ColumnID = col.ID
		stored.Slug, stored.File, stored.Title = chapter.Slug, chapter.File, chapter.Title
		if err := r.checkBody(col.Path, &stored, max(time.Now().Unix(), col.UpdatedAt+1)); err != nil {
			return err
		}
		if stored.ContentHash != expectedHash {
			return fmt.Errorf("后端正文与管理端版本不同，请等待内容部署完成后重试")
		}

		var siblings []schema.Chapter
		if err := tx.Select("id", "slug", "sort").Where("column_id = ? AND slug <> ?", col.ID, chapter.Slug).
			Order("sort, id").Find(&siblings).Error; err != nil {
			return err
		}
		position := slices.IndexFunc(siblings, func(ch schema.Chapter) bool { return slices.Contains(following, ch.Slug) })
		if position < 0 {
			position = len(siblings)
		}
		for i, ch := range siblings {
			sort := i
			if i >= position {
				sort++
			}
			if sort != ch.Sort {
				if err := tx.Model(&ch).UpdateColumn("sort", sort).Error; err != nil {
					return err
				}
			}
		}
		stored.Sort = position
		if err := tx.Save(&stored).Error; err != nil {
			return err
		}
		return setLatestChapterTime(tx, col.ID)
	})
}

func (r *columnRepository) WithdrawChapter(ctx context.Context, slug, chapterSlug string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var col schema.Column
		if err := tx.Where("slug = ?", slug).First(&col).Error; err != nil {
			return err
		}
		if err := tx.Where("column_id = ? AND slug = ?", col.ID, chapterSlug).Delete(&schema.Chapter{}).Error; err != nil {
			return err
		}
		return setLatestChapterTime(tx, col.ID)
	})
}

func setLatestChapterTime(tx *gorm.DB, columnID int64) error {
	latest := tx.Model(&schema.Chapter{}).Select("COALESCE(MAX(updated_at), 0)").Where("column_id = ?", columnID)
	return tx.Model(&schema.Column{}).Where("id = ?", columnID).UpdateColumn("updated_at", latest).Error
}
