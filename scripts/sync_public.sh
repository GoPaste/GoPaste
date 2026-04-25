#!/bin/bash
# sync_public.sh
# 将当前 master 分支的可公开内容同步到 b 仓库（public remote）
# 过滤掉：docs/、scripts/、Makefile、.codebuddy/
#
# 用法：
#   sh ./scripts/sync_public.sh                      # 使用默认 commit message
#   sh ./scripts/sync_public.sh "feat: add feature"  # 自定义 commit message

set -e

SOURCE_BRANCH="master"
PUBLIC_BRANCH="public"
COMMIT_MSG="${1:-sync: update public release}"

CURRENT_BRANCH=$(git symbolic-ref --short HEAD)

# 确保不在 public 分支上执行
if [ "$CURRENT_BRANCH" = "$PUBLIC_BRANCH" ]; then
  echo "错误：请切回 master 分支后再执行此脚本。"
  exit 1
fi

echo ">>> 当前分支：$CURRENT_BRANCH"

# 确保工作区干净
if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "错误：工作区有未提交的变更，请先 commit 或 stash。"
  exit 1
fi

# 异常时自动切回原分支
cleanup() {
  echo ">>> 发生错误，切回原分支..."
  git checkout -f "$CURRENT_BRANCH" 2>/dev/null || true
}
trap cleanup ERR

# 首次初始化：若 public 分支不存在，用 --orphan 创建（无历史）
if ! git show-ref --verify --quiet "refs/heads/$PUBLIC_BRANCH"; then
  echo ">>> 首次初始化 public 分支（无历史）..."
  git checkout --orphan "$PUBLIC_BRANCH"
  git rm -rf . 2>/dev/null || true
else
  git checkout "$PUBLIC_BRANCH"
fi

# 用 master 的内容完整覆盖当前工作区（通过 git checkout master -- .）
git checkout "$SOURCE_BRANCH" -- .

# 删除不公开的文件和目录
rm -rf docs/ scripts/ Makefile .codebuddy/
git add -A

# 如果没有变更，跳过提交
if git diff --cached --quiet; then
  echo ">>> 没有新的变更，无需同步。"
  git checkout "$CURRENT_BRANCH"
  exit 0
fi

# 提交
git commit -m "$COMMIT_MSG"

# 推送到 b 仓库的 master 分支
echo ">>> 推送到 public 仓库..."
git push public "$PUBLIC_BRANCH:master"

echo ">>> 同步完成！commit：$COMMIT_MSG"

# 切回原分支
git checkout "$CURRENT_BRANCH"
echo ">>> 已切回分支：$CURRENT_BRANCH"
