// Package types 定义 gopaste 共享数据结构。
package types

import "time"

// ItemType 剪切板条目类型。
type ItemType string

const (
	TypeText  ItemType = "text"
	TypeImage ItemType = "image"
	TypeLink  ItemType = "link"
	TypeCode  ItemType = "code"
	TypeFile  ItemType = "file"
)

// Item 是剪切板历史中的一条记录。
//
// Content 字段在持久化时会被加密；Preview 为明文摘要，用于快速搜索展示。
type Item struct {
	ID        int64     `gorm:"primaryKey" json:"id"`
	Hash      string    `gorm:"uniqueIndex;size:64" json:"hash"`
	Type      ItemType  `gorm:"index;size:16" json:"type"`
	Content   []byte    `gorm:"type:blob" json:"-"`          // 加密后的完整内容，不直接返回前端
	Preview   string    `gorm:"size:512" json:"preview"`     // 明文摘要（文本前 200 字 / 图片尺寸信息）
	ImagePath string    `gorm:"size:512" json:"imagePath"`   // 图片落盘路径（仅 Type==image）
	Size      int64     `json:"size"`                        // 原始内容大小（字节）
	CharCount int       `json:"charCount"`                   // 字符数（文本类）/ 像素信息（图片由 Preview 表达）
	Pinned    bool      `gorm:"index" json:"pinned"`
	Favorite  bool      `gorm:"index" json:"favorite"`
	Note      string    `gorm:"size:256" json:"note"` // 用户备注（显示时优先于 preview）
	SourceApp string    `gorm:"size:128" json:"sourceApp"`
	CreatedAt time.Time `gorm:"index" json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// TableName 自定义表名。
func (Item) TableName() string { return "clip_items" }

// SearchQuery 前端传入的查询参数。
type SearchQuery struct {
	Keyword  string   `json:"keyword"`
	Type     ItemType `json:"type"`     // 空字符串表示全部
	Favorite bool     `json:"favorite"` // 是否仅显示收藏
	Page     int      `json:"page"`
	PageSize int      `json:"pageSize"`
}

// ListResult 分页结果。
type ListResult struct {
	Items []Item `json:"items"`
	Total int64  `json:"total"`
	Page  int    `json:"page"`
}
