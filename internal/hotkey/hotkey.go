// Package hotkey 提供全局快捷键注册能力。
//
// 实际调用由 golang.design/x/hotkey 完成，平台行为差异（尤其是 Linux Wayland）会在 Register 时返回错误。
package hotkey

import (
	"context"
	"fmt"
	"strings"
	"sync"

	hk "golang.design/x/hotkey"
)

// Manager 快捷键管理器。
//
// 历史上只持有单个快捷键（用于唤出主面板）；现支持额外注册多个"附加"快捷键
// （比如 Alt+0..6 用于唤起并切到对应 tab）。Close 时会一并注销。
type Manager struct {
	hotkey *hk.Hotkey
	cancel context.CancelFunc

	// extras 的读写发生在：
	//   - Add（app.registerHotkey 启动期 & UpdateSettings 里调用）
	//   - Close（shutdown & UpdateSettings 重注册前会先关）
	// UpdateSettings 触发时，旧 Manager 的 Close 与新 Manager 的 Add 会相继进入
	// 同一个对象 —— 实际上不会，因为 app 层每次都是 new 一个新 Manager。但并发
	// 安全还是显式加锁更稳。
	mu     sync.Mutex
	extras []*Manager
}

// New 根据给定的修饰键 + 主键注册全局快捷键。
//
// modifiers 支持 "ctrl" / "shift" / "alt" / "cmd"（大小写不敏感）；
// key 支持 A-Z、0-9（其他保留字符后续扩展）。
// 成功后 fn 会在每次按下时被调用。调用方应在不再需要时调用 Close。
func New(ctx context.Context, modifiers []string, key string, fn func()) (*Manager, error) {
	mods, err := parseModifiers(modifiers)
	if err != nil {
		return nil, err
	}
	k, err := ParseKey(key)
	if err != nil {
		return nil, err
	}

	h := hk.New(mods, k)
	if err := h.Register(); err != nil {
		return nil, err
	}

	cctx, cancel := context.WithCancel(ctx)
	go func() {
		// 事件消费循环必须"接到就立刻释放"，不能在这里同步调 fn()。
		//
		// 背景（mac 偶现热键失效 → 重启才恢复 的真正根因）：
		//   golang.design/x/hotkey 内部的事件管道链是
		//       主线程 Carbon handler → keydownCallback → hk.keydownIn (unbuffered)
		//         → newEventChan 中转 goroutine → hk.keydownOut → 这里
		//
		//   如果我们在这里同步调 fn()（= togglePanel → wailsruntime.* → 主线程 dispatch_sync），
		//   期间中转 goroutine 因没人 recv 而阻塞在 `out <-`；
		//   然后主线程 Carbon handler 再次 fire 时，`keydownIn <- Event{}` 无法
		//   写入 unbuffered channel —— 主线程就卡在这条 cgo 调用里出不来。
		//   主线程 runloop 一旦停转，后续所有全局热键、菜单、托盘点击全部失效，
		//   必须杀进程重启才能恢复。
		//
		//   用独立 goroutine 跑 fn() 让本循环始终就绪，就能把整条链路解耦：
		//     - 中转 goroutine 永远能把事件投进来
		//     - keydownCallback 的主线程阻塞窗口缩到纳秒级
		//     - 即使 togglePanel 某次卡住 5 秒也不会波及热键事件流
		//
		//   副作用：用户在 fn() 执行中连按同一热键会并发触发多次 togglePanel。
		//   togglePanel 本身是幂等的（加锁读 windowVisible），最多看到一次多余闪烁，
		//   远比"整个应用的快捷键死掉"好接受。
		for {
			select {
			case <-cctx.Done():
				return
			case <-h.Keydown():
				go fn()
			}
		}
	}()
	return &Manager{hotkey: h, cancel: cancel}, nil
}

// Add 在已有 Manager 上追加注册一个快捷键。
//
// 用于在主热键之外再挂多个全局热键（如 Alt+0..6 切 tab）。
// 失败时单个快捷键忽略（返回 error 但不影响其它已注册项）。
// 调用 Close 时会一并注销所有附加项。
func (m *Manager) Add(ctx context.Context, modifiers []string, key string, fn func()) error {
	if m == nil {
		return fmt.Errorf("hotkey: nil manager")
	}
	sub, err := New(ctx, modifiers, key, fn)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.extras = append(m.extras, sub)
	m.mu.Unlock()
	return nil
}

// Close 注销快捷键（包括所有附加项）。
func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	// 先把 extras 切出来再遍历，避免持锁期间长时间关闭 cgo 资源。
	m.mu.Lock()
	extras := m.extras
	m.extras = nil
	m.mu.Unlock()
	for _, e := range extras {
		_ = e.Close()
	}

	if m.hotkey == nil {
		return nil
	}
	if m.cancel != nil {
		m.cancel()
	}
	return m.hotkey.Unregister()
}

func parseModifiers(mods []string) ([]hk.Modifier, error) {
	if len(mods) == 0 {
		return nil, fmt.Errorf("hotkey: empty modifiers")
	}
	out := make([]hk.Modifier, 0, len(mods))
	for _, m := range mods {
		switch strings.ToLower(strings.TrimSpace(m)) {
		case "ctrl", "control":
			out = append(out, modCtrl())
		case "shift":
			out = append(out, hk.ModShift)
		case "alt", "option":
			out = append(out, modAlt())
		case "cmd", "command", "super", "win":
			out = append(out, modCmd())
		default:
			return nil, fmt.Errorf("hotkey: unknown modifier %q", m)
		}
	}
	return out, nil
}
