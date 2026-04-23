// Package updater 基于 GitHub Releases 检测新版本。
// 本模块只做"检测 + 返回信息"，不做"下载并替换 binary"——
// 后者涉及签名、权限、重启逻辑，风险较高，让用户点击跳转浏览器下载更安全。
package updater

import (
	"context"
	"time"

	"github.com/creativeprojects/go-selfupdate"
)

// Repo 指定 GitHub 仓库，形如 "owner/name"。
const Repo = "larkwins/gopaste"

// Result 是一次检测的结果。HasUpdate=false 表示已是最新。
type Result struct {
	HasUpdate      bool   `json:"hasUpdate"`
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion,omitempty"`
	ReleaseURL     string `json:"releaseUrl,omitempty"`
	PublishedAt    string `json:"publishedAt,omitempty"`
	ReleaseNotes   string `json:"releaseNotes,omitempty"`
}

// Check 访问 GitHub Releases API，比对当前版本与最新版本。
// currentVersion 传入 semver 风格字符串（如 "0.1.0"）。
// 无网络或无 release 时返回 HasUpdate=false 且无错误，避免弹错误。
func Check(ctx context.Context, currentVersion string) (Result, error) {
	res := Result{CurrentVersion: currentVersion}

	// 10s 超时避免阻塞 UI
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	latest, found, err := selfupdate.DetectLatest(ctx, selfupdate.ParseSlug(Repo))
	if err != nil {
		return res, err
	}
	if !found {
		return res, nil
	}

	if !latest.GreaterThan(currentVersion) {
		return res, nil
	}

	res.HasUpdate = true
	res.LatestVersion = latest.Version()
	res.ReleaseURL = latest.URL
	res.ReleaseNotes = latest.ReleaseNotes
	if !latest.PublishedAt.IsZero() {
		res.PublishedAt = latest.PublishedAt.Format(time.RFC3339)
	}
	return res, nil
}
