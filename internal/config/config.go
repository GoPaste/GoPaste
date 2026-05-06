// Package config 管理 GoPaste 的数据目录与运行配置。
package config

import (
	"os"
	"path/filepath"
)

// AppName 数据目录名。
const AppName = "GoPaste"

// Website 官网地址（程序内所有引用官网链接的地方都从这里读取）。
const Website = "https://gopaste.wetools.cc/"

// Paths 运行期路径集合。
type Paths struct {
	Root     string // ~/.GoPaste
	DB       string // ~/.GoPaste/gopaste.db
	Images   string // ~/.GoPaste/images
	Key      string // ~/.GoPaste/key.hex
	Settings string // ~/.GoPaste/settings.json
	Lock     string // ~/.GoPaste/.lock (单实例)
}

// ResolvePaths 计算用户数据目录；如不存在则创建。
func ResolvePaths() (*Paths, error) {
	// 优先 XDG_DATA_HOME / LOCALAPPDATA / APPSUPPORT
	var root string
	if ucd, err := os.UserConfigDir(); err == nil {
		root = filepath.Join(ucd, AppName)
	} else {
		home, _ := os.UserHomeDir()
		root = filepath.Join(home, "."+AppName)
	}

	p := &Paths{
		Root:     root,
		DB:       filepath.Join(root, "gopaste.db"),
		Images:   filepath.Join(root, "images"),
		Key:      filepath.Join(root, "key.hex"),
		Settings: filepath.Join(root, "settings.json"),
		Lock:     filepath.Join(root, ".lock"),
	}
	for _, d := range []string{p.Root, p.Images} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return nil, err
		}
	}
	return p, nil
}
