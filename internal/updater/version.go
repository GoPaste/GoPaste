package updater

// Version 是当前应用版本号（semver）。
// 构建时由 Makefile 自动从 wails.json 读取 productVersion 并通过
// -ldflags "-X gopaste/internal/updater.Version=x.y.z" 注入，无需手动修改。
var Version = "0.2.0"
