# Selectively Merge `main` Into `custom-prod`

从远端 `main` 审查、挑选并移植改动到长期维护的 `custom-prod`。默认不是整分支 merge，目标是保留上游修复，同时不破坏 custom 分支的定制业务。

## 核心原则

1. **先 fetch，只读审查，不直接 pull/merge。**
2. **以 `last-merged` 为审查水位线。** 它表示该 main 提交及以前已完成审查，不表示这些提交全部进入了 custom。
3. **按功能组审查，逐项由维护者决定。** 结论通常是：保留、保留并修复、拒绝、需要 Aether 配合。
4. **批准一项才修改一项。** 未明确批准前不 cherry-pick、不改代码。
5. **默认不提交、不推送、不打发布 Tag、不部署。** 这些动作必须单独得到明确授权。
6. **不能用整文件 `ours/theirs` 覆盖业务逻辑。** 冲突解决必须同时理解 main 意图和 custom 定制。
7. **不碰无关工作区内容。** 不擅自 reset、stash、清理或提交用户原有修改和未跟踪文件。

## Custom 分支保护清单

审查和冲突处理时，以下内容默认视为高风险：

- 自定义返利/分销逻辑：不能用 main 的实现覆盖。
- 定制分组、虚拟订阅组、套餐和订阅权益。
- 支付履约、余额、充值累计和并发恢复流程。
- 渠道定价、显式零价格、优先级/Flex/Batch 计费。
- 管理端定制路由和功能开关行为。
- 数据库迁移编号、已执行迁移和生产数据兼容性。
- 与 `/opt/stacks/aether` 共同维护的协议转换、用量和计费口径。

凡是涉及以上区域，必须检查代码、测试和数据语义，不能只看是否能编译。

---

## 一、开始前检查

### 1. 确认所在分支和工作区

```bash
git status --short --branch
git branch --show-current
git remote -v
```

必须确认当前分支是 `custom-prod`。

如果已有修改：

- 记录哪些是用户原有修改；
- 不擅自提交、stash 或 reset；
- 后续只操作本次批准的文件/hunk；
- 使用 `git diff` 和 `git diff --cached` 区分暂存与未暂存内容。

### 2. 检查是否存在未完成的 Git 操作

```bash
git status
git rev-parse -q --verify MERGE_HEAD
git rev-parse -q --verify CHERRY_PICK_HEAD
git rev-parse -q --verify REBASE_HEAD
```

如果存在未完成的 merge/cherry-pick/rebase，先弄清来源，不得直接启动新操作。

---

## 二、获取并确定审查范围

### 1. 只获取远端 main

```bash
git fetch --tags origin main
git log -1 --oneline --decorate origin/main
```

不要执行：

```bash
git pull
git merge origin/main
```

除非维护者明确要求整分支合并。

### 2. 检查 `last-merged`

```bash
git show --no-patch --oneline --decorate last-merged
git merge-base --is-ancestor last-merged origin/main
```

解释：

- 返回成功：审查范围为 `last-merged..origin/main`。
- Tag 不存在：由维护者指定起始提交，不能自行猜测。
- Tag 不再是 main 的祖先：说明 main 历史被重写，停止并报告。

### 3. 列出新增提交

```bash
git log --reverse --no-merges \
  --format='%h %ad %s' --date=iso-strict \
  last-merged..origin/main

git diff --stat last-merged..origin/main
```

同时检查 main 中是否已有补丁等价内容：

```bash
git cherry custom-prod origin/main
```

- `-`：patch-equivalent，通常无需重复合入。
- `+`：custom 中没有等价补丁，但仍需人工判断是否适用。

`git cherry` 只能比较补丁等价性，不能代替业务审查。

---

## 三、提交分组与逐项审判

不要简单按 commit 一条条搬。先按依赖和业务域分组，例如：

- 计费与用量统计
- API 协议转换
- Chat/Responses/WebSocket
- 管理端和前端
- 支付、余额、订阅
- 数据库迁移
- 稳定性和 nil/panic 修复
- Aether 联动

对每组输出：

```markdown
### 第 N 项：<功能名称>

- main commits: `<hash...>`
- 解决的问题：...
- 影响路径：...
- 对 custom 定制的影响：...
- 是否需要 Aether：是/否
- 风险：低/中/高
- 建议：保留 / 保留并修复 / 拒绝
```

维护者给出结论后再进入修改。若要求 review，先做静态代码审查和测试分析，再次请求结论。

### 连续修复必须作为一组

如果后续提交是在修复前一提交，例如：

- 功能提交
- 边界修复
- 冲突检测
- 回归测试

则必须按原始顺序整组移植。不能只拿最后一个补丁。

---

## 四、应用已批准的改动

### 方案 A：提交独立且工作区允许时

```bash
git cherry-pick --no-commit <commit>
```

多个有依赖的提交按 main 原始顺序执行。

`--no-commit` 只是应用到工作区/索引，不代表可以跳过审查。

### 方案 B：提交混入无关内容或架构已分叉时

优先人工移植需要的 hunk：

```bash
git show --stat <commit>
git show <commit> -- path/to/file
```

适用于：

- 一个提交混有 migration、前端或其他未批准内容；
- main 文件已拆分，而 custom 已合并到其他文件；
- main 逻辑会覆盖 custom 返利/分组/套餐语义；
- Aether 需要采用同一口径但代码结构不同。

### 冲突处理规则

1. 先读完整函数和相关测试，不只看冲突标记。
2. 明确 main 新行为与 custom 既有行为。
3. 在 custom 当前架构中重新表达 main 意图。
4. 保留双方必要测试，必要时新增联合回归测试。
5. 检查残留冲突：

```bash
git diff --name-only --diff-filter=U
grep -R -n -E '^(<<<<<<<|=======|>>>>>>>)' backend frontend
git diff --check
git diff --cached --check
```

### 失败恢复

`cherry-pick --no-commit` 发生冲突时，某些情况下不会留下 `CHERRY_PICK_HEAD`。不要默认执行 `git cherry-pick --abort` 就能恢复。

恢复前必须确认操作开始前跟踪文件是否干净，并保护已有工作：

```bash
git status --short
git diff
git diff --cached
```

只有确认待丢弃内容全部来自本次失败应用时，才可恢复对应路径。若操作前已有修改，不得使用全局 `git reset --hard`。

---

## 五、Aether 联动规则

Sub2API 的上游为 Aether。以下改动必须联合审查 `/opt/stacks/aether`：

- Chat Completions ↔ Responses 字段映射；
- `parallel_tool_calls` 等显式 `false`/`0` 的保留；
- 缓存写入、缓存读取及普通输入 token 口径；
- 模型别名和最终上游模型名；
- service tier、长上下文和官方价格；
- 最终 User-Agent/originator 等上游身份头。

联合计费必须验证：

1. token 分类相同；
2. 模型别名解析相同；
3. Standard/Priority/Flex/Batch 价格相同；
4. 阈值边界和倍率相同；
5. 显式价格覆盖优先级相同；
6. 两边管理账单能核对原始 token、实际单价和总额。

Aether 原有未提交改动必须单独识别，不能随本次提交带入。

---

## 六、验证策略

### 后端

先运行受影响包和定向测试：

```bash
cd backend
go test ./internal/pkg/apicompat -run '<pattern>' -count=1
go test ./internal/service -run '<pattern>' -count=1
go test ./internal/repository -run '<pattern>' -count=1
```

再根据影响扩大范围：

```bash
go test ./internal/service ./internal/repository ./internal/handler/...
```

### 前端

```bash
cd frontend
pnpm type-check
pnpm test --run
```

### Aether

```bash
cargo fmt --all -- --check
cargo test -p aether-ai-formats
cargo test -p aether-usage-runtime
cargo test -p aether-billing
cargo test -p aether-admin
git diff --check
```

测试命令应使用非生产环境，不重启或更新线上服务。

---

## 七、提交、推送和发布

完成全部逐项审查后，先报告：

- 已保留项；
- 已拒绝项；
- 人工修复项；
- Aether 联动项；
- 测试结果；
- 尚存工作区内容。

只有维护者明确授权后才能：

```bash
git commit
git push
git tag
```

提交时只暂存本次批准文件。未跟踪 Logo、SQL、历史文档和其他既有修改不得自动加入。

推送前：

```bash
git fetch origin <branch>
git status --short --branch
git log --left-right --oneline HEAD...origin/<branch>
```

远端已前进时，先保留远端提交，再进行快进推送；禁止未经确认的 force push。

---

## 八、更新 `last-merged` 水位线

只有 `last-merged..origin/main` 范围内的所有功能都已完成审查（包括明确拒绝）后，才更新 Tag：

```bash
git tag -f -a last-merged -m "Last merged main commit" origin/main
git show --no-patch --decorate --oneline last-merged
```

默认只创建/移动本地 Tag，不推送。

注意：

- `last-merged` 是**可移动的审查水位线**，不是发布 Tag。
- 它不保证该 main 提交是 `custom-prod` 的祖先。
- 它表示该点之前的 main 改动已逐项处理：合入、改写或明确拒绝。
- 如果需要远端共享水位线，必须单独授权；移动远端同名 Tag 涉及强制更新，应谨慎操作。

---

## 九、最终检查清单

- [ ] 当前分支是 `custom-prod`
- [ ] 审查范围从 `last-merged` 到最新 `origin/main`
- [ ] 所有提交已按功能组归类
- [ ] 每组均得到维护者明确结论
- [ ] 没有覆盖自定义返利逻辑
- [ ] 定制分组、套餐、订阅行为已验证
- [ ] migration 没有被无意带入
- [ ] Sub2API/Aether 计费和协议口径一致
- [ ] 未提交文件和未跟踪文件来源清楚
- [ ] 冲突标记与 `git diff --check` 均通过
- [ ] 定向测试及必要的扩大测试通过
- [ ] 提交和推送只在明确授权后执行
- [ ] 全部审查结束后更新 `last-merged`

## 报告模板

```markdown
## Main Selection Status
- Base branch: `custom-prod`
- Review range: `last-merged..origin/main`
- Main tip: `<hash>`

## Decisions
- Kept: ...
- Kept with fixes: ...
- Rejected: ...
- Aether coordination: ...

## Conflicts / Manual Ports
- `<path>`: main intent + preserved custom behavior

## Verification
- `<command>`: pass/fail

## Git State
- Commit: not created / `<hash>`
- Push: not performed / completed
- `last-merged`: `<hash>`
- Remaining local changes: ...
```

> 好的挑选合并不是“让 Git 不再报冲突”，而是让 main 的修复意图与 custom 的业务约束同时成立，并且有测试证明。
