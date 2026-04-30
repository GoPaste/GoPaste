package clipboard

import (
	"testing"

	"gopaste/internal/types"
)

// TestTypeOfText 覆盖 typeOfText 的关键分支，重点保护"中文笔记不被误判为 Code"。
func TestTypeOfText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want types.ItemType
	}{
		{
			name: "empty",
			in:   "   \n  ",
			want: types.TypeText,
		},
		{
			name: "url-https",
			in:   "https://example.com/path?x=1",
			want: types.TypeLink,
		},
		{
			name: "go-code",
			in:   "package main\n\nfunc main() { println(\"hi\") }\n",
			want: types.TypeCode,
		},
		{
			// 关键回归：中文为主、含 "=" 与换行的笔记，过去会被判 Code，现在应为 Text。
			name: "chinese-notes-with-equals-must-be-text",
			in: `常用快捷键：
空格、tab、左右、上下、enter
启动命令：make dev pid=2000`,
			want: types.TypeText,
		},
		{
			// 中文夹少量代码片段、中文占主体 → 仍按 Text 走（避免通用 Code 标签）。
			name: "chinese-with-small-code-snippet",
			in: `这是一段说明文字，描述如下：
关键变量 cfg = nil 时退出；
其它情况按默认流程处理。`,
			want: types.TypeText,
		},
		{
			name: "plain-text-single-line",
			in:   "just a single line of plain text",
			want: types.TypeText,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := typeOfText([]byte(c.in))
			if got != c.want {
				t.Fatalf("typeOfText()=%q, want %q", got, c.want)
			}
		})
	}
}
