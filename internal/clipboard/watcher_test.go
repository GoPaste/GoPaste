package clipboard

import (
	"testing"
	"time"

	"gopaste/internal/storage"
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

// TestWatcherHandleDispatchesFirstEvent 回归保护：
// Watcher.handle 收到第一条非重复内容时必须立刻向 Events() 派发。
//
// 反模式记录（2026-05-05）：
//   第一版修"清空 + 重启后残留内容凭空复现"时，曾在 startTextWatch/
//   startImageWatch 里用 pasteboardChangeCount 做 baseline —— 但 App
//   启动到采样 baseline 之间存在极短窗口，若用户刚好此时按 Cmd+C，
//   这次复制的 changeCount 会被采进 baseline，后续永远检测不到变化，
//   表现为"偶现首次复制丢失"。
//
//   第二版又在 Watcher.handle 里加 bootstrapped 吞掉首帧 —— 直接把
//   用户启动后的第一次真正复制全部吞了。
//
//   最终方案：Start 里同步读一次当前剪贴板内容，把 hash 塞进 lastSig。
//   之后完全靠 hash 去重，首次派发若与 baseline 相同会被去重，不同会
//   正常入库。本测试锁住"handle 层不得额外吞首帧"这条语义。
func TestWatcherHandleDispatchesFirstEvent(t *testing.T) {
	w := New()
	w.handle(types.TypeText, []byte("first real copy after startup"))

	select {
	case got := <-w.Events():
		if got.Preview != "first real copy after startup" {
			t.Fatalf("派发内容不匹配：got=%q", got.Preview)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("首次 handle 后未收到 Item —— handle 层不应吞掉首次事件")
	}
}

// TestWatcherHandleDedupesSameHash 验证相同内容只派发一次（基于 hash 去重），
// 与 bootstrap 无关，但顺便保护这条核心 invariant。
func TestWatcherHandleDedupesSameHash(t *testing.T) {
	w := New()
	w.handle(types.TypeText, []byte("same content"))
	select {
	case <-w.Events():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("首次 handle 后未收到 Item")
	}

	// 同内容再来一次：必须被 lastSig 去重，不再派发。
	w.handle(types.TypeText, []byte("same content"))
	select {
	case got := <-w.Events():
		t.Fatalf("重复内容不应派发，但收到 Item: preview=%q", got.Preview)
	case <-time.After(50 * time.Millisecond):
		// OK
	}
}

// TestWatcherBootstrapFiltersStaleContent 验证 Bootstrap 语义：
// 把当前剪贴板内容 hash 作为 lastSig baseline 后，如果 tick 读到
// 同样内容会被 hash 去重（吞掉"启动前残留"），读到不同内容会正常派发
// （不影响"启动后用户新复制"）。
func TestWatcherBootstrapFiltersStaleContent(t *testing.T) {
	stale := []byte("clipboard content that already existed before app startup")
	fresh := []byte("a brand new copy the user just made")

	w := New()
	// 模拟 Start 里 bootstrapFromClipboard 的效果：直接把 stale 的 hash
	// 塞进 lastSig（真实调用需要系统剪贴板有内容，测试里绕过 IO 直接注入）。
	w.lastSig = storage.HashBytes(stale)

	// tick 派发"启动前残留"的同样内容：必须被去重。
	w.handle(types.TypeText, stale)
	select {
	case got := <-w.Events():
		t.Fatalf("启动前残留不应被再次入库，但收到 Item: preview=%q", got.Preview)
	case <-time.After(50 * time.Millisecond):
		// OK
	}

	// tick 派发"启动后用户新复制"：必须正常入库。
	w.handle(types.TypeText, fresh)
	select {
	case got := <-w.Events():
		if got.Preview != "a brand new copy the user just made" {
			t.Fatalf("新复制派发内容不匹配：got=%q", got.Preview)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("启动后新复制未派发 —— bootstrap 不应影响后续事件")
	}
}
