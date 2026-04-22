# GoPaste Makefile
# 使用方式: make <target>
# 示例: make dev / make build-win / make build-all

# ============== 变量 ==============
APP_NAME    := gopaste
VERSION     := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_DIR   := build/bin
WAILS       := wails
GO          := go
LDFLAGS     := -s -w -X main.version=$(VERSION)

# 交叉编译工具链（Linux 宿主打 Windows 包时需要）
WIN_CC      := x86_64-w64-mingw32-gcc
WIN_CXX     := x86_64-w64-mingw32-g++

# Wails 构建通用参数
WAILS_FLAGS := -ldflags "$(LDFLAGS)"

# 颜色
CYAN  := \033[36m
GREEN := \033[32m
RESET := \033[0m

.PHONY: help dev build build-win build-mac build-linux build-all \
        generate tidy test lint clean install-deps doctor

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
build: ## 构建当前平台
	xvfb-run -a $(WAILS) build -clean $(WAILS_FLAGS)
	@echo "$(GREEN)✓ Built: $(BUILD_DIR)/$(APP_NAME)$(RESET)"

build-win: ## 构建 Windows (amd64)
	CC=$(WIN_CC) CXX=$(WIN_CXX) \
	xvfb-run -a $(WAILS) build -clean -platform windows/amd64 $(WAILS_FLAGS)
	@echo "$(GREEN)✓ Built: $(BUILD_DIR)/$(APP_NAME).exe$(RESET)"

build-win-arm: ## 构建 Windows (arm64)
	xvfb-run -a $(WAILS) build -clean -platform windows/arm64 $(WAILS_FLAGS)
	@echo "$(GREEN)✓ Built: $(BUILD_DIR)/$(APP_NAME).exe$(RESET)"

build-mac: ## 构建 macOS (universal) ⚠️ 需在 macOS 上运行
	@if [ "$$(uname)" != "Darwin" ]; then echo "$(CYAN)⚠ macOS 应用只能在 macOS 上构建（Wails 限制）$(RESET)"; echo "  推荐使用 GitHub Actions: git tag v0.1.0 && git push --tags"; exit 1; fi
	unset GOROOT; $(WAILS) build -clean -platform darwin/universal $(WAILS_FLAGS)
	@echo "$(GREEN)✓ Built: $(BUILD_DIR)/$(APP_NAME).app$(RESET)"

build-mac-arm: ## 构建 macOS (Apple Silicon) ⚠️ 需在 macOS 上运行
	@if [ "$$(uname)" != "Darwin" ]; then echo "$(CYAN)⚠ macOS 应用只能在 macOS 上构建$(RESET)"; exit 1; fi
	unset GOROOT; $(WAILS) build -clean -platform darwin/arm64 $(WAILS_FLAGS)

build-mac-intel: ## 构建 macOS (Intel) ⚠️ 需在 macOS 上运行
	@if [ "$$(uname)" != "Darwin" ]; then echo "$(CYAN)⚠ macOS 应用只能在 macOS 上构建$(RESET)"; exit 1; fi
	unset GOROOT; $(WAILS) build -clean -platform darwin/amd64 $(WAILS_FLAGS)

build-linux: ## 构建 Linux (amd64)
	xvfb-run -a $(WAILS) build -clean -platform linux/amd64 $(WAILS_FLAGS)
	@echo "$(GREEN)✓ Built: $(BUILD_DIR)/$(APP_NAME)$(RESET)"

build-all: build-win build-linux ## 构建 Windows + Linux（macOS 需在 Mac 上单独构建或用 CI）
	@echo "$(GREEN)✓ Windows + Linux built$(RESET)"
	@echo "$(CYAN)ℹ macOS 请在 Mac 上运行 make build-mac，或使用 GitHub Actions$(RESET)"

# ============== 代码生成 ==============
generate: ## 重新生成前端 TS 绑定 (wailsjs/)
	$(WAILS) generate module

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
