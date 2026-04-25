// Package storage 基于 SQLite 提供剪切板历史的持久化能力。
package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"gopaste/internal/crypto"
	"gopaste/internal/types"
)

// Repo 剪切板条目仓储。
//
// 所有写入的 Content 会通过 cipher 加密；读取时解密。
// Preview 保持明文以支持 SQL LIKE 搜索。图片内容落盘到 imageDir 下 `<hash>.png`，DB 只存路径。
type Repo struct {
	db       *gorm.DB
	cipher   *crypto.Cipher
	imageDir string
}

// Open 打开或创建 SQLite 数据库。imageDir 用于存放图片文件。
func Open(dbPath, imageDir string, cipher *crypto.Cipher) (*Repo, error) {
	if dbPath == "" {
		return nil, errors.New("storage: empty db path")
	}
	if err := ensureDir(filepath.Dir(dbPath)); err != nil {
		return nil, err
	}
	if imageDir != "" {
		if err := ensureDir(imageDir); err != nil {
			return nil, err
		}
	}
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("storage: open db: %w", err)
	}
	if err := db.AutoMigrate(&types.Item{}); err != nil {
		return nil, fmt.Errorf("storage: migrate: %w", err)
	}
	return &Repo{db: db, cipher: cipher, imageDir: imageDir}, nil
}

// Close 关闭底层连接。
func (r *Repo) Close() error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// Save 保存或更新一条记录。如果相同 Hash 已存在，则刷新 UpdatedAt。
//
// 对图片类型：原始字节写入 imageDir/<hash>.png，DB 中仅保留 ImagePath，不加密文件。
// 对文本类型：Content 经 AES-GCM 加密后写入 DB。
func (r *Repo) Save(item *types.Item) error {
	if item.Hash == "" {
		item.Hash = HashBytes(item.Content)
	}

	// 去重
	var existing types.Item
	err := r.db.Where("hash = ?", item.Hash).First(&existing).Error
	if err == nil {
		existing.UpdatedAt = time.Now()
		return r.db.Save(&existing).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	// 图片走文件系统
	if item.Type == types.TypeImage && r.imageDir != "" && len(item.Content) > 0 {
		fp := filepath.Join(r.imageDir, item.Hash+".png")
		if err := os.WriteFile(fp, item.Content, 0o600); err != nil {
			return fmt.Errorf("storage: write image: %w", err)
		}
		item.ImagePath = fp
		item.Content = nil // 不再持久化到 DB
	} else if r.cipher != nil && len(item.Content) > 0 {
		enc, err := r.cipher.Encrypt(item.Content)
		if err != nil {
			return err
		}
		item.Content = enc
	}

	now := time.Now()
	item.CreatedAt = now
	item.UpdatedAt = now
	return r.db.Create(item).Error
}

// List 按条件分页查询。
func (r *Repo) List(q types.SearchQuery) (*types.ListResult, error) {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 || q.PageSize > 500 {
		q.PageSize = 50
	}
	tx := r.db.Model(&types.Item{})
	if q.Type != "" {
		tx = tx.Where("type = ?", q.Type)
	}
	if q.Favorite {
		tx = tx.Where("favorite = ?", true)
	}
	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		tx = tx.Where("preview LIKE ?", "%"+kw+"%")
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, err
	}

	var items []types.Item
	err := tx.Order("pinned DESC, updated_at DESC").
		Offset((q.Page - 1) * q.PageSize).
		Limit(q.PageSize).
		Find(&items).Error
	if err != nil {
		return nil, err
	}

	// 列表不返回 Content（大字段），前端按需调用 GetContent 获取
	for i := range items {
		items[i].Content = nil
	}

	return &types.ListResult{Items: items, Total: total, Page: q.Page}, nil
}

// GetContent 返回指定条目的明文 Content。
func (r *Repo) GetContent(id int64) ([]byte, error) {
	_, b, err := r.GetItemWithContent(id)
	return b, err
}

// GetItemWithContent 一次性返回类型 + 明文内容，便于写回剪切板。
func (r *Repo) GetItemWithContent(id int64) (types.ItemType, []byte, error) {
	var item types.Item
	if err := r.db.First(&item, id).Error; err != nil {
		return "", nil, err
	}
	// 图片从文件读
	if item.Type == types.TypeImage && item.ImagePath != "" {
		b, err := os.ReadFile(item.ImagePath)
		if err != nil {
			return "", nil, fmt.Errorf("storage: read image: %w", err)
		}
		return item.Type, b, nil
	}
	if r.cipher == nil || len(item.Content) == 0 {
		return item.Type, item.Content, nil
	}
	pt, err := r.cipher.Decrypt(item.Content)
	if err != nil {
		return "", nil, err
	}
	return item.Type, pt, nil
}

// Delete 删除一条记录（若为图片同时删除文件）。
func (r *Repo) Delete(id int64) error {
	var item types.Item
	if err := r.db.First(&item, id).Error; err != nil {
		return err
	}
	if item.ImagePath != "" {
		_ = os.Remove(item.ImagePath)
	}
	return r.db.Delete(&types.Item{}, id).Error
}

// Clear 清空所有非收藏、非置顶的记录（同时清理图片文件）。
func (r *Repo) Clear() error {
	var list []types.Item
	if err := r.db.Where("pinned = ? AND favorite = ?", false, false).Find(&list).Error; err != nil {
		return err
	}
	for _, it := range list {
		if it.ImagePath != "" {
			_ = os.Remove(it.ImagePath)
		}
	}
	return r.db.Where("pinned = ? AND favorite = ?", false, false).Delete(&types.Item{}).Error
}

// Prune 清理策略：保留最近 maxItems 条非收藏非置顶的记录，或保留最近 maxDays 天的记录。
// maxItems <= 0 表示不按条数限制；maxDays <= 0 表示不按时间限制。
func (r *Repo) Prune(maxItems int, maxDays int) (deleted int64, err error) {
	var toDelete []types.Item
	tx := r.db.Model(&types.Item{}).Where("pinned = ? AND favorite = ?", false, false)

	if maxDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -maxDays)
		var olds []types.Item
		if err := tx.Where("updated_at < ?", cutoff).Find(&olds).Error; err != nil {
			return 0, err
		}
		toDelete = append(toDelete, olds...)
	}

	if maxItems > 0 {
		var extras []types.Item
		err := r.db.Where("pinned = ? AND favorite = ?", false, false).
			Order("updated_at DESC").
			Offset(maxItems).
			Find(&extras).Error
		if err != nil {
			return 0, err
		}
		toDelete = append(toDelete, extras...)
	}

	// 去重
	seen := make(map[int64]bool, len(toDelete))
	var ids []int64
	for _, it := range toDelete {
		if seen[it.ID] {
			continue
		}
		seen[it.ID] = true
		if it.ImagePath != "" {
			_ = os.Remove(it.ImagePath)
		}
		ids = append(ids, it.ID)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	res := r.db.Where("id IN ?", ids).Delete(&types.Item{})
	return res.RowsAffected, res.Error
}

// SetPinned 设置置顶状态。
func (r *Repo) SetPinned(id int64, pinned bool) error {
	return r.db.Model(&types.Item{}).Where("id = ?", id).Update("pinned", pinned).Error
}

// SetFavorite 设置收藏状态。
func (r *Repo) SetFavorite(id int64, favorite bool) error {
	return r.db.Model(&types.Item{}).Where("id = ?", id).Update("favorite", favorite).Error
}

// SetNote 设置备注。
func (r *Repo) SetNote(id int64, note string) error {
	return r.db.Model(&types.Item{}).Where("id = ?", id).Update("note", note).Error
}

// Count 返回总条数。
func (r *Repo) Count() (int64, error) {
	var n int64
	err := r.db.Model(&types.Item{}).Count(&n).Error
	return n, err
}

// HashBytes 计算 SHA-256 十六进制摘要。
func HashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
