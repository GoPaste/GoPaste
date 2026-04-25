package updater

// Version 是当前应用版本号（semver）。
// 发布时可通过 -ldflags "-X gopaste/internal/updater.Version=x.y.z" 覆盖。
// 需与 wails.json 里的 productVersion 保持同步。
var Version = "0.1.0"
