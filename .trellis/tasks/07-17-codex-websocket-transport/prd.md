# Codex WebSocket 本地链路修复与验证记录

## 目标

让 Codex 通过 `http://localhost:7301/v1` 使用 Responses WebSocket，完成本地开发链路验收：

```text
Codex
  -> Sub2API Vite :7301        [仅开发环境代理]
  -> Sub2API backend :8080
  -> Aether Vite :5175         [仅开发环境代理]
  -> Aether gateway :8084
  -> official Codex Responses WebSocket
```

其中 `:7301` 和 `:5175` 是开发服务器代理层，不属于生产链路。生产核心链路为：

```text
Codex
  -> Sub2API 对外 API 入口
  -> Sub2API backend
  -> Aether gateway
  -> official Codex Responses WebSocket
```

生产环境的具体入口代理或负载均衡由部署配置决定，但不会经过上述两层 Vite 开发代理。

验收要求：

- 不发生 HTTPS fallback。
- 收到 `response.completed` 等合法终态事件。
- 成功请求记录为 `relay_complete`、`relay_completed`。
- 不将终态交付后的客户端正常关闭记录为 `client_disconnected`。

## 已完成改动

### WebSocket 代理

- `frontend/vite.config.ts`
  - 为 `/v1` 代理增加 `ws: true`，修复 `:7301 -> :8080` Upgrade 超时。
- `D:/sh/Aether/frontend/vite.config.ts`
  - 为 `/v1/` 代理增加 `ws: true`，修复 `:5175 -> :8084` Upgrade 超时。
  - 保留用户原有 `port: 5175` 与 `strictPort: true` 配置。

### route-v1 Relay 与 Aether 调度边界

- `backend/internal/config/config.go`
  - `gateway.openai_ws.mode_router_v2_enabled` 默认改为 `true`。
  - `gateway.openai_ws.aether_route_control_enabled` 默认改为 `true`。
  - reconnect migration 的依赖校验仅在该功能启用时执行。
- `backend/internal/config/config_test.go`
  - 覆盖上述默认值和显式关闭行为。
- Sub2API 负责 route-v1 ingress、控制帧校验和 Relay 生命周期，不负责账号调度。
- 账号选择、握手阶段 failover、写入后禁止重放和后续 step 绑定由 Aether 负责。
- 账号级配置已确认需要同时包含：

```json
{
  "openai_apikey_responses_websockets_v2_mode": "passthrough",
  "openai_apikey_responses_websockets_v2_enabled": true,
  "aether_ws": {
    "schema_version": 1,
    "enabled": true,
    "required_control_protocol": "route-v1"
  }
}
```

### Relay 完整性与关闭语义

- `backend/internal/service/openai_ws_v2/passthrough_relay.go`
  - 增加原子状态，记录终态是否已实际写给客户端。
  - 只有终态已交付后，客户端 EOF 才能视为正常完成。
  - 上游 EOF 或双方关闭但未观察到协议终态时，不再误判成功。
  - 支持 `response.done`、`response.cancelled`、`response.canceled` 等终态别名。
- `backend/internal/service/openai_ws_v2/passthrough_relay_test.go`
  - 增加“终态写入后客户端关闭仍成功”的回归测试。
  - 保留“终态仅在断开后的 drain 中观察到但未交付”必须失败的测试。
  - 修正 binary frame 用例的合法协议顺序。
- `backend/internal/service/openai_ws_v2/passthrough_relay_internal_test.go`
  - 补充终态别名断言。

## 已完成验证

- Sub2API 前端 TypeScript 检查通过。
- Aether 前端 TypeScript 检查通过。
- 定向 Go 测试通过：

```text
internal/config
internal/service/openai_ws_v2
internal/service
```

- Sub2API focused `go vet` 通过。
- Aether `codex_ws`：107/107。
- Aether quota concurrency：10/10。
- Aether orchestration：117/117，其中 Redis-backed 用例使用隔离 Redis 8.2.0。
- Race test 未执行：当前 Windows Go 环境未启用 CGO，不属于代码测试失败。
- 本机 Codex 连续三次真实 WebSocket 请求均成功：

```text
WS_LIVE_1
WS_LIVE_2
WS_LIVE_3
```

对应日志均确认包含 HTTP 101、`relay_dial_ok`、`relay_turn_completed`、`relay_completed` 和 Aether 请求完成事件，且没有 HTTPS fallback、1011、relay、proxy 或 settlement 错误。

## 最终链路验收

2026-07-18 已通过正式入口 `http://localhost:7301/v1` 连续完成三次真实 Codex 请求。客户端分别精确输出：

```text
WS_LIVE_1
WS_LIVE_2
WS_LIVE_3
```

三次调用 exit code 均为 0，客户端未出现 HTTPS fallback 提示。对应事件链为：

```text
Codex GET /v1/responses + WebSocket Upgrade
  -> Sub2API openai.websocket_ingress_started
  -> passthrough/router v2 relay_start
  -> Aether route_kind=responses_ws auth allowed
  -> Aether HTTP 101
  -> relay_dial_ok
  -> official Codex native WebSocket
  -> relay_turn_completed terminal_event=response.completed
  -> relay_complete
  -> relay_completed terminal_event=response.completed
  -> Sub2API HTTP 101 completed
```

三次请求所在时间窗口均观察到完整 WebSocket 生命周期；该窗口另有一条并发请求，因此日志事件不能在不读取请求体的情况下逐条关联 marker，但三次 CLI 精确输出、exit code 0、同时段 HTTP 101 和完整生命周期共同证明真实链路成功。

该请求时间窗口未出现：

```text
relay_failed
client_disconnected
openai.websocket_proxy_failed
```

完整运行链路、配置条件、事件语义和排查顺序见：

```text
docs/CODEX_WEBSOCKET_LOCAL_CHAIN.md
```

## 后续维护

1. 依赖、协议或路由配置变更后，重新执行正式入口真实请求。
2. 同时核对 Codex 客户端、Sub2API Relay 和 Aether `responses_ws` 日志，不能只以客户端文本作为传输类型证据。
3. 本地开发环境保持两层 Vite `/v1` 代理的 `ws: true`；生产环境不部署或经过这两层 Vite 代理。
4. 保持 Aether 中间跳 API Key、route-v1 和 native Codex WS 开关有效。
5. Race test 需在启用 CGO 的环境中补充执行。
6. Aether quota 去重是进程内机制；多 Gateway 副本仍需依赖共享状态、generation fence 和持久层幂等，并在生产环境补充多实例验证。

## 发布记录

```text
Aether commit: 66509b5a6ab2f108d54baecc3db6938f6e4ef6aa
Aether tag: backend-v0.7.35
Sub2API commit: 01d142e94b097674c5ed7755062c942c193653c6
Sub2API tag: cafecode-v0.0.28
```

两个 tag 的发布工作流均已完成且成功。Aether GitHub Release 已发布；Sub2API Container Image Release workflow 已成功。

最终日志必须包含：

```text
relay_dial_ok
relay_turn_completed terminal_event=response.completed
relay_complete
relay_completed
```

不得包含：

```text
Falling back from WebSockets to HTTPS transport
relay_failed
client_disconnected
openai.websocket_proxy_failed
```

## 约束

- 不记录或输出原始 API Key、JWT secret、数据库密码、管理员密码。
- 不停止用户现有 `:7301`、`:8080`、`:8084` 服务。
- 不覆盖或清理用户现有未提交修改和 worktree。
- 不执行 `pnpm start`、`vite dev`、`pnpm build` 或等价开发服务器启动命令。
- 文档与源码发布操作必须由用户明确授权。
