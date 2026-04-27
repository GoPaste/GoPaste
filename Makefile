# GoPaste Makefile
# 使用方式: make <target>
# 示例: make dev / make build-win / make build-all

# ============== 变量 ==============
APP_NAME    := GoPaste
# Wails outputfilename（与 wails.json 的 outputfilename 保持一致）
WAILS_OUT   := GoPaste
# 版本号统一从 wails.json 的 productVersion 字段读取，是唯一的版本来源
VERSION     := $(shell node -e "console.log(require('./wails.json').info.productVersion)" 2>/dev/null || \
               python3 -c "import json;print(json.load(open('wails.json'))['info']['productVersion'])" 2>/dev/null || \
               echo "dev")
BUILD_DIR   := build/bin
WAILS       := wails
GO          := go
LDFLAGS     := -s -w -X gopaste/internal/updater.Version=$(VERSION)

# Wails 构建通用参数
WAILS_FLAGS := -ldflags "$(LDFLAGS)"

# 颜色
CYAN  := \033[36m
GREEN := \033[32m
RESET := \033[0m

.PHONY: help dev build build-win build-mac build-linux build-all \
        generate gen-icons gen-icon-template tidy test lint clean install-deps doctor

# ============== 默认目标 ==============
help: ## 显示帮助
	@echo ""
	@echo "$(CYAN)GoPaste$(RESET) - 跨平台剪切板管理工具"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  $(GREEN)%-18s$(RESET) %s\n", $$1, $$2}'
	@echo ""

# ============== 开发 ==============
dev: ## 启动开发模式（热重载）
	$(WAILS) dev

debug: ## 启动开发模式 + 开启 DevTools
	$(WAILS) dev -devtools

# ============== 构建 ==============
# xvfb-run 仅在 Linux 宿主（无 DISPLAY 的 CI / headless 环境）下需要，
# macOS / Windows 根本没这命令。用 uname 判断宿主，mac 走 build-mac 的
# unset GOROOT 流程（避免 toolchain 目录污染 Wails 构建）。
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Linux)
	XVFB := xvfb-run -a
else
	XVFB :=
endif

build: ## 构建当前平台
ifeq ($(UNAME_S),Darwin)
	unset GOROOT; $(WAILS) build -clean $(WAILS_FLAGS)
	mv $(BUILD_DIR)/$(WAILS_OUT).app $(BUILD_DIR)/$(APP_NAME)_$(VERSION).app 2>/dev/null || \
	mv $(BUILD_DIR)/$(WAILS_OUT)     $(BUILD_DIR)/$(APP_NAME)_$(VERSION)     2>/dev/null || true
else
	$(XVFB) $(WAILS) build -clean $(WAILS_FLAGS)
	mv $(BUILD_DIR)/$(WAILS_OUT) $(BUILD_DIR)/$(APP_NAME)_$(VERSION) 2>/dev/null || true
endif
	@echo "$(GREEN)✓ Built: $(BUILD_DIR)/$(APP_NAME)_$(VERSION)$(RESET)"

build-win: ## 构建 Windows (amd64)
	$(XVFB) $(WAILS) build -clean -platform windows/amd64 $(WAILS_FLAGS)
	mv $(BUILD_DIR)/$(WAILS_OUT).exe $(BUILD_DIR)/$(APP_NAME)_$(VERSION).exe
	@echo "$(GREEN)✓ Built: $(BUILD_DIR)/$(APP_NAME)_$(VERSION).exe$(RESET)"

build-win-arm: ## 构建 Windows (arm64)
	$(XVFB) $(WAILS) build -clean -platform windows/arm64 $(WAILS_FLAGS)
	mv $(BUILD_DIR)/$(WAILS_OUT).exe $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_arm64.exe
	@echo "$(GREEN)✓ Built: $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_arm64.exe$(RESET)"

build-mac: ## 构建 macOS (universal) ⚠️ 需在 macOS 上运行
	@if [ "$$(uname)" != "Darwin" ]; then echo "$(CYAN)⚠ macOS 应用只能在 macOS 上构建（Wails 限制）$(RESET)"; echo "  推荐使用 GitHub Actions: git tag v0.1.0 && git push --tags"; exit 1; fi
	unset GOROOT; $(WAILS) build -clean -platform darwin/universal $(WAILS_FLAGS)
	mv $(BUILD_DIR)/$(WAILS_OUT).app $(BUILD_DIR)/$(APP_NAME)_$(VERSION).app
	@echo "$(GREEN)✓ Built: $(BUILD_DIR)/$(APP_NAME)_$(VERSION).app$(RESET)"

build-mac-arm: ## 构建 macOS (Apple Silicon) ⚠️ 需在 macOS 上运行
	@if [ "$$(uname)" != "Darwin" ]; then echo "$(CYAN)⚠ macOS 应用只能在 macOS 上构建$(RESET)"; exit 1; fi
	unset GOROOT; $(WAILS) build -clean -platform darwin/arm64 $(WAILS_FLAGS)
	mv $(BUILD_DIR)/$(WAILS_OUT).app $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_arm64.app

build-mac-intel: ## 构建 macOS (Intel) ⚠️ 需在 macOS 上运行
	@if [ "$$(uname)" != "Darwin" ]; then echo "$(CYAN)⚠ macOS 应用只能在 macOS 上构建$(RESET)"; exit 1; fi
	unset GOROOT; $(WAILS) build -clean -platform darwin/amd64 $(WAILS_FLAGS)
	mv $(BUILD_DIR)/$(WAILS_OUT).app $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_intel.app

build-linux: ## 构建 Linux (amd64) ⚠️ 需在 Linux 上运行，或使用 GitHub Actions
	@if [ "$$(uname)" != "Linux" ]; then \
		echo "$(CYAN)⚠ Linux 应用只能在 Linux 上原生构建（避免 CGO 交叉编译兼容问题）$(RESET)"; \
		echo "  推荐使用 GitHub Actions: git tag v0.x.x && git push --tags"; \
		exit 1; \
	fi
	$(WAILS) build -clean -platform linux/amd64 $(WAILS_FLAGS)
	mv $(BUILD_DIR)/$(WAILS_OUT) $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_linux_amd64
	@echo "$(GREEN)✓ Built: $(BUILD_DIR)/$(APP_NAME)_$(VERSION)_linux_amd64$(RESET)"

build-all: build-win build-linux ## 构建 Windows + Linux
	@echo "$(GREEN)✓ Windows + Linux built$(RESET)"
	@echo "$(CYAN)ℹ macOS 请在 Mac 上运行 make build-mac，或使用 GitHub Actions$(RESET)"

# ============== 代码生成 ==============
generate: ## 重新生成前端 TS 绑定 (wailsjs/)
	$(WAILS) generate module

gen-icons: ## 重新生成所有图标（彩色 appicon + 彩色/灰色菜单栏图标）
	python3 scripts/gen_appicon.py
	python3 scripts/gen_tray_icon_gray.py

gen-icon-template: ## 重新生成模板图标（tray-template.png）
	$(GO) run scripts/gen_tray_icon.go

# ============== Go 工具链 ==============
tidy: ## 整理 Go 依赖
	$(GO) mod tidy

test: ## 运行后端单元测试
	$(GO) test -v -race ./internal/...

test-cover: ## 运行测试 + 覆盖率报告
	$(GO) test -coverprofile=coverage.out ./internal/...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)✓ 覆盖率报告: coverage.html$(RESET)"

lint: ## 运行 Go vet
	$(GO) vet ./...

bench: ## 运行性能测试
	$(GO) test -bench=. -benchmem ./internal/...

# ============== 前端 ==============
fe-install: ## 安装前端依赖
	cd frontend && npm install

fe-build: ## 仅构建前端
	cd frontend && npm run build

fe-dev: ## 启动前端开发服务器（独立）
	cd frontend && npm run dev

# ============== 工具 ==============
doctor: ## 检查 Wails 环境
	$(WAILS) doctor

install-deps: ## 安装系统依赖 (仅 Linux)
	@echo "正在检测包管理器..."
	@if command -v apt >/dev/null 2>&1; then \
		sudo apt install -y libgtk-3-dev libwebkit2gtk-4.1-dev libx11-dev libxtst-dev; \
	elif command -v yum >/dev/null 2>&1; then \
		sudo yum install -y gtk3-devel webkit2gtk4.0-devel libX11-devel libXtst-devel libXi-devel libXrandr-devel; \
	else \
		echo "未识别的包管理器，请手动安装依赖"; exit 1; \
	fi
	@echo "$(GREEN)✓ 系统依赖已安装$(RESET)"

install-wails: ## 安装 Wails CLI
	$(GO) install github.com/wailsapp/wails/v2/cmd/wails@latest

# ============== 清理 ==============
clean: ## 清理构建产物
	rm -rf $(BUILD_DIR)/*
	rm -rf frontend/dist
	rm -f coverage.out coverage.html
	@echo "$(GREEN)✓ 已清理$(RESET)"

clean-all: clean ## 深度清理（含 node_modules）
	rm -rf frontend/node_modules
	@echo "$(GREEN)✓ 深度清理完成$(RESET)"

# ============== 发布 ==============
release: clean build-all ## 构建全平台发布包
	@echo ""
	@echo "$(CYAN)构建产物:$(RESET)"
	@ls -lh $(BUILD_DIR)/
	@echo ""
	@echo "$(GREEN)✓ 发布构建完成 $(VERSION)$(RESET)"

# ============== 信息 ==============
info: ## 显示项目信息
	@echo "App:     $(APP_NAME)"
	@echo "Version: $(VERSION)"
	@echo "Go:      $(shell $(GO) version)"
	@echo "Node:    $(shell node -v 2>/dev/null || echo 'not found')"
	@echo "Wails:   $(shell $(WAILS) version 2>/dev/null || echo 'not found')"
	@echo "OS/Arch: $(shell $(GO) env GOOS)/$(shell $(GO) env GOARCH)"
