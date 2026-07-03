# 定制套餐倍率订阅方案

## 1. 当前结论

定制套餐采用“套餐配置 + 订单快照 + 订阅级虚拟权益”的模型：

- 套餐可配置是否允许定制、最小倍数、最大倍数。
- 用户购买时按 `套餐价格 × 倍数` 计价，限额按倍数放大。
- 新定制购买默认不再创建真实 custom group，而是在来源基础分组的 `user_subscriptions` 记录上写入虚拟权益。
- 上线前已经存在且仍未过期的真实 custom group 继续走兼容续费路径，避免老用户中断。

## 2. 倍数与价格规则

- 倍数为整数，允许 `1x`，套餐配置的最小值不得小于 1。
- 最终价格：`plan.price × multiplier`。
- 如存在原价展示：`plan.original_price × multiplier`。
- 日/周/月限额在鉴权和展示时按倍率放大。

示例：基础分组日/周/月限额为 `20 / 60 / 100`，购买 `3x` 后有效限额为 `60 / 180 / 300`。

## 3. 数据模型

### 3.1 subscription_plans

新增/使用字段：

- `custom_multiplier_enabled`
- `custom_multiplier_min`
- `custom_multiplier_max`

开启定制时：`min >= 1 && max >= min`。

### 3.2 payment_orders

订单保存下单快照：

- `subscription_multiplier`
- `subscription_source_group_id`
- `subscription_source_price`
- `subscription_source_original_price`

`amount` / `pay_amount` 保存最终金额。虚拟权益模式下，`subscription_group_id` 最终保持为来源基础分组 ID；历史真实 custom group 兼容路径下可回写为 custom group ID。

### 3.3 user_subscriptions

新增虚拟权益字段：

- `custom_multiplier`：虚拟定制倍数；为空表示普通订阅。
- `custom_source_plan_id`：来源套餐 ID。
- `custom_source_group_id`：来源基础分组 ID。
- `custom_expires_at`：定制倍率权益过期时间；基础订阅可能比它更晚过期。
- `custom_display_name`：展示名。

约束：

- `custom_multiplier` 为空时，其他虚拟权益字段必须为空。
- `custom_multiplier` 不为空时，来源套餐、来源分组、定制过期时间必须同时存在。

## 4. 展示名规则

虚拟权益展示名和历史真实 custom group 名称统一为：

```text
[倍数x]套餐名#用户id
```

示例：

```text
[2x]speciall#123
[3x]精选套餐#456
```

说明：套餐名为空时使用 `Subscription`；名称按分组名称长度限制截断；不使用用户名。

## 5. 履约流程

### 5.1 普通套餐

按原逻辑对来源基础分组执行 `AssignOrExtendSubscription`。

### 5.2 新定制购买：虚拟权益路径

1. 校验订单快照中的 plan、source group、multiplier。
2. API Key 保持绑定来源基础分组。
3. 对来源基础分组的用户订阅写入/续期虚拟权益字段。
4. 鉴权、限额检查、订阅列表展示通过 `EffectiveSubscriptionGroup` 计算放大后的有效分组。

### 5.3 历史真实 custom group 兼容路径

如果用户对同一来源套餐已有未过期真实 custom group：

- 继续复用该真实 custom group。
- 续费时必须校验当前倍数与订单倍数一致。
- 仍同步来源分组限额、账号绑定、渠道绑定等旧逻辑。

如果只有已过期历史 custom group，新订单走虚拟权益路径，并把该用户绑定在已过期 custom group 上的 API Key 迁回来源分组。

## 6. 续费与并发安全

- 用户有未过期定制权益时，购买页进入续费状态，不允许修改倍数。
- 后端以未过期虚拟权益或未过期真实 custom group 为准重新解析倍数，不能信任前端提交值。
- 同一用户同一套餐存在 pending / paid / recharging 的定制订单时，拒绝创建第二个定制订单。
- 履约在事务内锁定用户，避免并发重复续期。
- 履约成功写入审计日志；重试时先查审计日志，避免重复延长订阅。

## 7. 普通订阅与定制权益重叠的安全规则

同一用户可能先持有较长普通来源分组订阅，再购买较短定制权益。此时必须避免“用 30 天定制订单把 365 天普通订阅全部免费升级为定制”。

规则：

- 基础订阅 `expires_at` 保留较晚的普通订阅过期时间。
- 定制倍率只写入 `custom_expires_at` 对应的购买时长。
- `custom_expires_at` 到期后，若基础订阅仍未过期，鉴权自动退回普通来源分组限额。

## 8. 管理与退款

- 虚拟权益不通过 `/admin/groups` 承载；后续如需人工调整，应在订阅管理维度处理。
- 历史真实 custom group 仍可在 `/admin/groups` 查看/调整，调整倍数时需要重算限额。
- 退款仍按现有订阅扣期逻辑处理，不自动删除历史真实 custom group。

## 9. 迁移与 checksum 规则

- 已应用的 migration 文件不得修改；新增变更必须创建新的 migration。
- 如果出现 checksum mismatch，应优先恢复已应用 migration 的原内容，再新增迁移承载后续变更。
- 当前虚拟权益字段由 `161_user_subscription_virtual_custom_entitlement.sql` 引入。
