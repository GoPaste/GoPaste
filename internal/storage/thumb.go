package storage

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "golang.org/x/image/bmp"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"gopaste/internal/types"
)

// thumbMaxDim 缩略图长边像素。列表缩略图显示区约几十到一百多 px，留 2x 余量。
const thumbMaxDim = 256

// thumbSem 限制同时进行的缩略图生成数，避免多张大图同时解码造成内存峰值。
var thumbSem = make(chan struct{}, 2)

// thumbPathFor 由图片原图路径推导缩略图路径：<hash>.png -> <hash>.thumb.png。
func thumbPathFor(imagePath string) string {
	return strings.TrimSuffix(imagePath, filepath.Ext(imagePath)) + ".thumb.png"
}

// fileThumbDir 文件类型条目（单个图片文件）的缩略图缓存目录。
func (r *Repo) fileThumbDir() string {
	if r.imageDir == "" {
		return ""
	}
	return filepath.Join(r.imageDir, "filethumbs")
}

// makeThumb 把图片字节缩放到长边 thumbMaxDim 并编码为 PNG。
// 若原图本身不超过 thumbMaxDim，返回 (nil, nil)，调用方直接使用原图。
func makeThumb(src []byte) ([]byte, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= thumbMaxDim && cfg.Height <= thumbMaxDim {
		return nil, nil
	}
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	w, h := cfg.Width, cfg.Height
	if w >= h {
		h = max(1, h*thumbMaxDim/w)
		w = thumbMaxDim
	} else {
		w = max(1, w*thumbMaxDim/h)
		h = thumbMaxDim
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Src, nil)

	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, dst); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeAtomic 先写临时文件再 rename，避免读到写了一半的缩略图。
func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// ensureImageThumb 为落盘的原图生成缩略图文件（已存在则跳过）。
// 返回缩略图字节；原图本身足够小时返回原图字节且不写文件。
func (r *Repo) ensureImageThumb(imagePath string) ([]byte, error) {
	tp := thumbPathFor(imagePath)
	if b, err := os.ReadFile(tp); err == nil {
		return b, nil
	}

	thumbSem <- struct{}{}
	defer func() { <-thumbSem }()

	// 排队期间可能已被其它请求生成
	if b, err := os.ReadFile(tp); err == nil {
		return b, nil
	}

	src, err := os.ReadFile(imagePath)
	if err != nil {
		return nil, fmt.Errorf("storage: read image: %w", err)
	}
	thumb, err := makeThumb(src)
	if err != nil {
		return nil, err
	}
	if thumb == nil {
		return src, nil
	}
	if err := writeAtomic(tp, thumb); err != nil {
		// 写缓存失败不影响本次返回
		return thumb, nil
	}
	return thumb, nil
}

// GetThumbnail 返回条目的列表缩略图字节（PNG 或原图小图）。
//   - 图片条目：读取/生成 <hash>.thumb.png；
//   - 文件条目且仅含一个图片文件：缩放后缓存到 filethumbs/；
//   - 其它：返回 (nil, nil)。
func (r *Repo) GetThumbnail(id int64) ([]byte, error) {
	var item types.Item
	if err := r.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	switch item.Type {
	case types.TypeImage:
		if item.ImagePath == "" {
			return nil, nil
		}
		return r.ensureImageThumb(item.ImagePath)
	case types.TypeFile:
		_, content, err := r.GetItemWithContent(id)
		if err != nil {
			return nil, err
		}
		return r.fileThumbnail(id, string(content))
	}
	return nil, nil
}

var thumbableExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true,
	".gif": true, ".bmp": true, ".webp": true,
}

func (r *Repo) fileThumbnail(id int64, content string) ([]byte, error) {
	paths := strings.Split(content, "\n")
	if len(paths) != 1 {
		return nil, nil // 多文件不预览
	}
	p := strings.TrimSpace(paths[0])
	if !thumbableExts[strings.ToLower(filepath.Ext(p))] {
		return nil, nil
	}
	st, err := os.Stat(p)
	if err != nil {
		return nil, nil // 文件可能已删除
	}

	dir := r.fileThumbDir()
	var cache string
	if dir != "" {
		sum := sha1.Sum([]byte(p + "|" + strconv.FormatInt(st.ModTime().UnixNano(), 10) + "|" + strconv.FormatInt(st.Size(), 10)))
		cache = filepath.Join(dir, fmt.Sprintf("%d_%s.png", id, hex.EncodeToString(sum[:4])))
		if b, err := os.ReadFile(cache); err == nil {
			return b, nil
		}
	}

	thumbSem <- struct{}{}
	defer func() { <-thumbSem }()
	if cache != "" {
		if b, err := os.ReadFile(cache); err == nil {
			return b, nil
		}
	}

	src, err := os.ReadFile(p)
	if err != nil {
		return nil, nil
	}
	thumb, err := makeThumb(src)
	if err != nil {
		return nil, nil // 解码失败：前端回退为占位图
	}
	if thumb == nil {
		return src, nil
	}
	if cache != "" {
		if err := ensureDir(dir); err == nil {
			_ = writeAtomic(cache, thumb)
		}
	}
	return thumb, nil
}

// removeItemFiles 删除条目关联的磁盘文件：原图、图片缩略图、文件缩略图缓存。
func (r *Repo) removeItemFiles(it types.Item) {
	if it.ImagePath != "" {
		_ = os.Remove(it.ImagePath)
		_ = os.Remove(thumbPathFor(it.ImagePath))
	}
	if it.Type == types.TypeFile {
		if dir := r.fileThumbDir(); dir != "" {
			matches, _ := filepath.Glob(filepath.Join(dir, fmt.Sprintf("%d_*.png", it.ID)))
			for _, m := range matches {
				_ = os.Remove(m)
			}
		}
	}
}
