package storage

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"gopaste/internal/types"
)

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 128, A: 200})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestMakeThumb(t *testing.T) {
	th, err := makeThumb(pngBytes(t, 1000, 500))
	if err != nil || th == nil {
		t.Fatalf("thumb=%v err=%v", th == nil, err)
	}
	cfg, _, _ := image.DecodeConfig(bytes.NewReader(th))
	if cfg.Width != thumbMaxDim || cfg.Height != thumbMaxDim/2 {
		t.Fatalf("got %dx%d", cfg.Width, cfg.Height)
	}

	small, err := makeThumb(pngBytes(t, 100, 80))
	if err != nil || small != nil {
		t.Fatalf("small image should not be scaled: %v %v", small != nil, err)
	}

	if _, err := makeThumb([]byte("not an image")); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestImageThumbLifecycle(t *testing.T) {
	dir := t.TempDir()
	repo, err := Open(filepath.Join(dir, "t.db"), filepath.Join(dir, "images"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	it := &types.Item{Type: types.TypeImage, Content: pngBytes(t, 800, 800), Preview: "img"}
	if err := repo.Save(it); err != nil {
		t.Fatal(err)
	}

	// 存量路径：删掉缩略图后应懒生成
	tp := thumbPathFor(it.ImagePath)
	_ = os.Remove(tp)
	b, err := repo.GetThumbnail(it.ID)
	if err != nil || len(b) == 0 {
		t.Fatalf("thumb err=%v len=%d", err, len(b))
	}
	if _, err := os.Stat(tp); err != nil {
		t.Fatalf("thumb file not cached: %v", err)
	}
	orig, _ := os.ReadFile(it.ImagePath)
	if len(b) >= len(orig) {
		t.Fatalf("thumb (%d) not smaller than original (%d)", len(b), len(orig))
	}

	if err := repo.Delete(it.ID); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{it.ImagePath, tp} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("%s should be removed", p)
		}
	}
}

func TestFileThumb(t *testing.T) {
	dir := t.TempDir()
	repo, err := Open(filepath.Join(dir, "t.db"), filepath.Join(dir, "images"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	src := filepath.Join(dir, "a.png")
	if err := os.WriteFile(src, pngBytes(t, 600, 300), 0o600); err != nil {
		t.Fatal(err)
	}
	it := &types.Item{Type: types.TypeFile, Content: []byte(src), Preview: "a.png"}
	if err := repo.Save(it); err != nil {
		t.Fatal(err)
	}
	b, err := repo.GetThumbnail(it.ID)
	if err != nil || len(b) == 0 {
		t.Fatalf("err=%v len=%d", err, len(b))
	}
	m, _ := filepath.Glob(filepath.Join(repo.fileThumbDir(), "*.png"))
	if len(m) != 1 {
		t.Fatalf("cache files = %d", len(m))
	}
	if err := repo.Delete(it.ID); err != nil {
		t.Fatal(err)
	}
	m, _ = filepath.Glob(filepath.Join(repo.fileThumbDir(), "*.png"))
	if len(m) != 0 {
		t.Fatalf("cache not cleaned: %v", m)
	}
}
