# 订阅管理增强：统一平移重置窗口 + 订阅统计与使用率

> 落地日期：2026-07-21
> 涉及页面：`/admin/subscriptions`
> 迁移：`backend/migrations/174_subscription_usage_daily.sql`

---

## 1. 背景

管理端此前缺三样能力：

1. 无法统一调整订阅重置窗口的时间点。2026-07-21 需要把全部周限订阅的重置时间整体推迟
   14 小时，只能手写 SQL 直连生产库。
2. 没有跨套餐的额度总览。想知道"用户手上还攥着多少额度"只能逐个订阅看。
3. 看不到订阅周期内的用量走势。`user_subscriptions` 只有三个当前窗口的计数器，没有历史。

---

## 2. 核心口径问题：日限和周限不是同一个量纲

这是本次设计里唯一需要拍板的地方，也是最容易做错的地方。

线上套餐分两类：

| 套餐 | 限额类型 |
|---|---|
| Latte (拿铁) / Americano (美式) / Instant (速溶) | 周限（300 / 105 / 36 USD） |
| Specialty (精选) | **只有日限**（150 USD），没有周限 |
| Frappé (星冰乐) / Shaken Tea (冰摇茶) | 月限 |

周限是"一池水，7 天补一次"；日限是"每天补一次的水龙头"。**把日限折算成周限必须先指定看多久**——
没有时间范围，折算系数就是凭空拍的。

历史上人工估算时用过 `3.5 × 日限`，那个 3.5 的真实来历是"距离周限重置还剩约 4 天"，
即时间范围被压缩成了一个魔法系数。它同时低估了敞口：$150/天 的周等效是 `7 × 150 = $1,050/周`，
是拿铁 $300/周 的 3.5 倍，而不是 `3.5 × 150 = $525`。

**采用的方案：把系数还原成显式的时间范围，不做隐式折算。**

面板给三个数，前两个账实一致，第三个是唯一的派生值且范围可调：

| 指标 | 定义 | 覆盖范围 |
|---|---|---|
| 今日剩余 | `Σ(日限 − 日用量)` | 仅有日限的订阅 |
| 本周剩余 | `Σ(周限 − 周用量)` | 仅有周限的订阅 |
| 未来 N 天上限 | 见下 | 全部订阅，N ∈ {1,3,7,14,30}，默认 7 |

未来 N 天可消耗上限，逐订阅计算后求和：

```
有日限：今日剩余 + 日限 × (N 天内日窗重置次数)
有周限：本周剩余 + 周限 × (N 天内周窗重置次数)
有月限：本月剩余 + 月限 × (N 天内月窗重置次数)
范围终点 = min(now + N 天, expires_at)      // 订阅到期就烧不动了
```

一次性日额度订阅（`HasOneTimeDailyQuota`，`starts_at` 与 `expires_at` 相差不超过一天）
永不重置，只计当前窗口剩余。

窗口已过期（日窗 >24h、周窗 >7d）但尚未被下次请求惰性重置的订阅，
**用量按已重置计 0、剩余计满额**，与计费侧 `CheckAndResetWindows` 的语义一致。

限额一律取 `EffectiveSubscriptionGroup` 的结果，即已应用 `custom_multiplier` 倍率。

---

## 3. 功能一：统一平移重置窗口

```
POST /api/v1/admin/subscriptions/bulk-shift-window
```

```jsonc
{
  "daily": true, "weekly": true, "monthly": false,
  "offset_hours": 14,        // 正数推迟、负数提前；非 0，|offset| ≤ 720
  "dry_run": false,          // true = 只统计不落库，供弹窗预览
  "filters": {               // 缺省 = 全部生效中订阅
    "status": "active", "user_id": 309, "group_id": 4, "platform": "openai"
  }
}
```

响应 `{ matched, updated, skipped_future, dry_run }`。

三条不变量（写在 `userSubscriptionRepository.ShiftUsageWindows` 的 SQL 里）：

1. 只命中**窗口起点非 NULL 且所属分组确实设了该窗口限额**的行，其余行完全不碰。
2. 平移后任一目标窗口起点会落到**未来**的行整行跳过，计入 `skipped_future`。
   否则会造出比一个周期更长的窗口。
3. **只改 `*_window_start`，绝不动 `*_usage_usd`。**

写库后失效对应订阅的 L1（ristretto，TTL 10s）与 Redis 缓存。

### 这是一次性平移，不是永久偏移

窗口长度本身没变（周窗恒为 `7×24h`，硬编码在 `UserSubscription.NeedsWeeklyReset`）。
下一轮重置触发时，`SubscriptionService.CheckAndResetWindows` 会把新窗口起点写成
`startOfDay(now)`，偏移量随之消失、重新对齐到 00:00。

要长期保持非零点对齐，得改 `CheckAndResetWindows` / `CheckAndActivateWindow` 里的
`startOfDay(time.Now())`（例如引入可配置的 `window_offset_hours`），而不是改数据。

### 幂等

handler 走 `executeAdminIdempotentJSON`，但幂等 key **只从 `Idempotency-Key` 请求头取**
（`internal/service/idempotency.go:231`），不传头就完全不去重。前端
`bulkShiftWindow` 在每次非 dry-run 调用内部现生成一个新 key：传输层重试被去重，
用户有意的第二次提交仍能生效。TTL 24 小时——key 复用了就等于 24 小时内无法再平移一次。

已知局限：两次独立点击会拿到两个不同 key，会平移两次。目前挡住它的是前端的
in-flight 守卫（`if (shiftingWindow.value) return`），不是幂等键。

---

## 4. 功能二：订阅统计总览

```
GET /api/v1/admin/subscriptions/stats?horizon_days=7&ranking_limit=20
```

返回 `totals`（三张卡片）、`plans[]`（分套餐明细）、`ranking.daily` / `ranking.weekly`
（按 `usage_ratio` 降序，后端排好，前端不得再排）。

`plans[]` 的 `used_usd` / `quota_usd` / `usage_ratio` 是**主窗口口径**，优先级 **日 > 周 > 月**，
与 `cycle.window_kind` 同源。由后端给出而非前端推导——否则同时设了两种限额的分组会被
静默丢掉一个维度（月限的 Shaken Tea 就是这么暴露出来的）。

数据来源是 `userSubRepo.List` 分页拉全部生效中订阅（当前约 115 条，单页即可），
在 Go 里用现有的 `NeedsDailyReset` / `EffectiveSubscriptionGroup` 等 helper 计算，
不在 SQL 里重写一遍窗口逻辑，避免与计费侧口径漂移。

---

## 5. 功能三：单订阅使用率明细

```
GET /api/v1/admin/subscriptions/:id/usage-series
```

返回 `daily[]`（每天）、`weekly[]`（每周）、`cycle`（整周期）。

- **每天分母**：优先用当天快照的日限；无日限则用 `周限 ÷ 7`（或 `月限 ÷ 30`）折算，
  并置 `limit_is_derived: true`，UI 必须标注为折算值。
- **每周分桶**：以**订阅自身的周窗口起点**为锚点向前对齐，不是自然周——
  这样分桶边界与计费实际的重置边界一致。
- **整周期**：`quota_usd = 限额 × 周期内已经历的窗口数`。分组三个限额全为 0 时
  `cycle` 返回 `null`（没有分母的"使用率"不该渲染成 0%）；有限额但无用量时返回
  `cost_usd: 0` 的正常对象，必须照常渲染。
- `data_from` / `data_complete`：汇总表最早有数据的日期，早于它的历史已随
  `usage_logs` 保留期消失，UI 需显式提示。

---

## 6. 数据模型：`subscription_usage_daily`

### 为什么必须新建表

排查过所有现成数据源，没有一个能用：

| 数据源 | 结论 |
|---|---|
| `usage_logs` | 有 `subscription_id` + `actual_cost`，但线上只剩约 1 天（见下） |
| `usage_dashboard_daily` | 有 115 天历史，但只有全站汇总，无 user/subscription 维度 |
| `usage_dashboard_daily_users` / `hourly_users` | 只是 `(日期, user_id)` 活跃集合，不含金额 |
| `ops_metrics_daily` | 有 group_id，无 user 维度、无用户成本 |
| `billing_usage_entries` | 结构最合适，但线上为空，该写入路径未启用 |

`usage_logs` 只剩 1 天**不是应用配置导致的**。应用侧
`dashboard_aggregation.retention.usage_logs_days` 默认 90 天且线上没覆盖；真正的裁剪来自
宿主机脚本 `/root/clean`（`/etc/cron.d/clean-daily` 触发，sub2api 每周一次），
`SUB2API_KEEP_DAYS` 默认 **1**。该脚本的 `SUB2API_PURGE_TABLES` 是**显式白名单**，
`subscription_usage_daily` 不在其中，因此安全。

### 表结构

```sql
subscription_usage_daily (
    subscription_id, bucket_date,          -- PRIMARY KEY
    user_id, group_id,
    cost_usd, request_count,
    daily_limit_usd, weekly_limit_usd, monthly_limit_usd,   -- 当天有效限额快照
    computed_at
)
```

两个刻意的设计：

- **不加任何外键。** 这张表的价值就在于订阅被撤销或删除后历史仍可回溯，
  级联删除会毁掉它。与 `usage_dashboard_*` 系列同样处理。
- **限额列是快照**（已含 `custom_multiplier` 倍率），套餐改价后历史使用率的分母不会跟着漂移。

保留期 400 天，配置项 `dashboard_aggregation.retention.subscription_daily_days`，
与 `usage_logs` 完全解耦。

### 写入路径

挂在现有 `DashboardAggregationService` 的增量循环上（它本来就在按水位线扫 `usage_logs`），
按完整自然日重新聚合，`ON CONFLICT (subscription_id, bucket_date) DO UPDATE` 整键替换，可重复执行。

来源过滤：`subscription_id IS NOT NULL AND billing_type = 1`，与
`IncrementUsage` 的计费口径一致。已在生产数据上对账验证：按 `subscription_id` 汇总
`actual_cost` 与 `weekly_usage_usd` 逐条吻合。

---

## 7. ⚠️ 陷阱：重算路径绝不能删这张表

`subscription_usage_daily` 与 `usage_dashboard_*` **语义相反**，改聚合代码时极易搞混：

- `usage_dashboard_*` 是 `usage_logs` 的**派生视图** —— 源行被删就该跟着回退，
  所以 `recomputeRangeInTx` 对它们先 DELETE 区间再重建，这是对的。
- `subscription_usage_daily` 是**独立历史记录** —— 它存在的唯一意义就是在 `usage_logs`
  被裁掉之后历史仍在。对它做同样的先删后建 = 永久且无法重建地销毁数据。

触发路径真实存在：`UsageCleanupService`（`internal/service/usage_cleanup_service.go`）
删完某区间的 `usage_logs` 后，会立刻对**同一区间**调 `TriggerRecomputeRange`。
管理员清理一次 6 月的日志，6 月的订阅使用率就没了。

**规则：增量路径（`aggregateRangeInTx`）和重算路径（`recomputeRangeInTx`）对这张表
都只能用纯 upsert。** 唯一允许删它的地方是 `CleanupSubscriptionUsageDaily`（保留期清理）。

护栏：`internal/repository/subscription_usage_daily_integration_test.go` 的
`TestRecomputeKeepsRollupAfterUsageLogsPurged`。已用变异测试验证过——把 DELETE 加回去，
该用例确实变红。

---

## 8. 两个反直觉的真实数据特征

**① 使用率会超过 100%。** 管理员中途重置配额后，汇总表记录的是**实际花费**、
订阅计数器记录的是**当前窗口内计费额**，两者本就不同。线上已存在单周实际花费 355.03
而周限 300（118%）的订阅。进度条宽度可 clamp 到 100%，**百分比数字必须显示真实值**，
不要 `Math.min(ratio, 1)`。

**② `ranking[].window_start` / `window_resets_at` 会是 `null`。** 窗口已过期但尚未被
下次请求惰性重置时，后端刻意不回报陈旧起点，避免前端把过期窗口当成当前窗口渲染。
这类行 `used_usd` 为 0、`remaining_usd` 为满额，倒计时列显示"待重置"占位。

---

## 9. 上线步骤

1. 部署新版本，迁移 174 自动执行（幂等，已在 scratch 库连跑两次验证）。
2. 应用启动后聚合任务会自动补齐今天与昨天（`recompute_days` 默认 2）。
3. **手动回填** `usage_logs` 里剩余的更早日期，见下。再往前的历史无法恢复。

```bash
docker exec -i sub2api-postgres psql -U sub2api -d sub2api -v ON_ERROR_STOP=1 <<'SQL'
BEGIN;

INSERT INTO subscription_usage_daily (
    subscription_id, bucket_date, user_id, group_id,
    cost_usd, request_count,
    daily_limit_usd, weekly_limit_usd, monthly_limit_usd, computed_at
)
SELECT
    agg.subscription_id, agg.bucket_date, agg.user_id, us.group_id,
    agg.cost_usd, agg.request_count,
    NULLIF(g.daily_limit_usd, 0)   * eff.multiplier,
    NULLIF(g.weekly_limit_usd, 0)  * eff.multiplier,
    NULLIF(g.monthly_limit_usd, 0) * eff.multiplier,
    NOW()
FROM (
    SELECT
        ul.subscription_id AS subscription_id,
        (ul.created_at AT TIME ZONE 'Asia/Shanghai')::date AS bucket_date,
        ul.user_id AS user_id,
        COALESCE(SUM(ul.actual_cost), 0) AS cost_usd,
        COUNT(*) AS request_count
    FROM usage_logs ul
    WHERE ul.subscription_id IS NOT NULL
      AND ul.billing_type = 1
    GROUP BY ul.subscription_id, (ul.created_at AT TIME ZONE 'Asia/Shanghai')::date, ul.user_id
) AS agg
JOIN user_subscriptions us ON us.id = agg.subscription_id
LEFT JOIN groups g ON g.id = us.group_id
CROSS JOIN LATERAL (
    SELECT CASE
        WHEN us.custom_multiplier IS NOT NULL
            AND us.custom_multiplier >= 1
            AND us.custom_source_plan_id IS NOT NULL
            AND us.custom_source_group_id IS NOT NULL
            AND us.custom_expires_at IS NOT NULL
            AND us.custom_expires_at > NOW()
            AND COALESCE(g.is_custom_subscription_group, FALSE) = FALSE
        THEN us.custom_multiplier::numeric
        ELSE 1
    END AS multiplier
) AS eff
ON CONFLICT (subscription_id, bucket_date) DO UPDATE SET
    user_id = EXCLUDED.user_id, group_id = EXCLUDED.group_id,
    cost_usd = EXCLUDED.cost_usd, request_count = EXCLUDED.request_count,
    daily_limit_usd = EXCLUDED.daily_limit_usd,
    weekly_limit_usd = EXCLUDED.weekly_limit_usd,
    monthly_limit_usd = EXCLUDED.monthly_limit_usd,
    computed_at = NOW();

SELECT count(*) AS rows, count(DISTINCT subscription_id) AS subscriptions,
       min(bucket_date) AS earliest, max(bucket_date) AS latest,
       round(sum(cost_usd)::numeric, 2) AS total_cost_usd
FROM subscription_usage_daily;

COMMIT;
SQL
```

时区 `'Asia/Shanghai'` 必须与应用 `timezone` 配置一致（线上 `config.yaml` 即此值）。
脚本幂等，可重复执行。

---

## 10. 已知限制

- 汇总表启用之前的历史**永久缺失**，需要养满一个订阅周期（通常一个月）才能看到完整走势。
- 若 sub2api 容器**连续停机超过 `SUB2API_KEEP_DAYS` 天**再启动，那段时间的 `usage_logs`
  可能已被 `/root/clean` 删掉，对应的订阅用量补不回来。可考虑把 `SUB2API_KEEP_DAYS` 调到 3 留余量。
- 窗口平移的双击防护依赖前端 in-flight 守卫，不是幂等键（见 §3）。
- `createIdempotencyKey` 已抽到 `frontend/src/utils/idempotency.ts`，但
  `api/keys.ts` / `api/user.ts` / `api/payment.ts` 里的三份重复副本未迁移
  （`payment.ts` 涉及线上支付流程，超出本次改动范围），值得后续收敛。
