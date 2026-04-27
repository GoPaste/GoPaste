# sync-public

对比 `master` 分支与 `public` 分支的差异，自动生成 commit message，然后执行 `scripts/sync_public.sh` 将可公开内容同步到 b 仓库。

## 执行步骤

### 第一步：获取两分支的差异摘要

运行以下命令获取自上次同步以来 master 的新增 commit：

```bash
git log public..master --oneline --no-merges
```

如果 `public` 分支不存在（首次同步），则获取最近 10 条：

```bash
git log --oneline -10
```

### 第二步：分析差异，生成 commit message

根据上一步的 commit 列表，分析变更类型，按照以下规则生成一条简洁的 commit message：

- 若包含新功能：`feat: <功能描述>`
- 若仅有修复：`fix: <修复描述>`
- 若包含多种类型：`release: <版本或功能描述摘要>`
- 若无法判断：`sync: update public release`

commit message 使用中文描述核心变更，保持简洁（不超过 50 字）。

### 第三步：执行同步脚本

将生成的 commit message 作为参数传入：

```bash
sh ./scripts/sync_public.sh "<生成的 commit message>"
```

### 注意事项

- 执行前确保当前在 `master` 分支，工作区无未提交变更
- `docs/`、`scripts/`、`Makefile`、`.codebuddy/` 不会同步到 public 仓库
- 同步完成后 b 仓库（`public` remote）的 `master` 分支会新增一条 commit
