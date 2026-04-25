#!/bin/bash
# sync_public.sh
# 将当前 master 分支的可公开内容同步到 b 仓库（public remote）
# 过滤掉：docs/、scripts/、Makefile

set -e

CURRENT_BRANCH=$(git symbolic-ref --short HEAD)
PUBLIC_BRANCH="public-sync"

echo ">>> 当前分支：$CURRENT_BRANCH"
echo ">>> 开始同步到 public 仓库..."

# 确保工作区干净
if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "错误：工作区有未提交的变更，请先 commit 或 stash。"
  exit 1
fi

# 删除旧的临时分支（如果存在）
git branch -D "$PUBLIC_BRANCH" 2>/dev/null || true

# 从 master 创建临时分支
git checkout -b "$PUBLIC_BRANCH"

# 删除不公开的文件和目录
git rm -r --cached docs/ scripts/ Makefile 2>/dev/null || true
git commit -m "chore: remove private files for public release" --allow-empty

# 推送到 b 仓库的 master 分支
git push public "$PUBLIC_BRANCH:master" --force

echo ">>> 同步完成！"

# 切回原分支，清理临时分支
git checkout "$CURRENT_BRANCH"
git branch -D "$PUBLIC_BRANCH"

echo ">>> 已切回分支：$CURRENT_BRANCH"
