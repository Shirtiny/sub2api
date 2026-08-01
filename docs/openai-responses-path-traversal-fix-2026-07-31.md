# 网关上游 URL 路径穿越修复：/responses 子路径、Gemini 模型名、Grok video request_id 片段校验

- 记录日期：2026-07-31
- 时间口径：本文业务时间均为 UTC+8（Asia/Shanghai）
- 文档性质：安全漏洞调查 + 上游修复移植说明 + Aether 侧链路判定 + 发布/部署记录 + 攻击痕迹取证；不包含用户邮箱、API Key 或 OAuth 凭据
- 涉及入口：`POST /v1/responses/*subpath`、`POST /responses/*subpath`、`POST /backend-api/codex/responses/*subpath`；`/v1beta/models/*`（Gemini）；Grok video 状态查询

| 组件 | 版本 / 提交 | 状态 |
|---|---|---|
| Sub2API（路径片段校验） | `cafecode-v0.0.38` / `f6384c6b6` | 已提交 / push / tag；**2026-08-01 已部署上线**（见第 8 节） |
| 上游修复来源 | `Wei-Shaw/sub2api` `main` PR #5137 / `017f6bbd5`（2026-07-31 合并） | 已移植 |
| Aether | `backend-v0.7.33` | **无需改动**（见第 5 节） |

---

## 1. 报告的现象

外部报告指出：三条 `POST /responses/*subpath` 通配路由把客户端提供的子路径未经校验直接拼进上游 URL。由于到达业务代码的 `URL.Path` 已是百分号解码后的结果，形如 `POST /backend-api/codex/responses/..%2f..%2f<path>` 的请求会拼出 `https://chatgpt.com/backend-api/codex/responses/../../<path>`；Go 的 `net/http` 不消解 dot-segment，该路径原样发出，被上游归一化为 `https://chatgpt.com/backend-api/<path>`。网关随后为其附上所选上游账号的凭据并把响应返回给调用方。

结论先行：**custom-prod 当时确实存在此漏洞；上游 `main` 已于同日修复；Aether 不受影响。**

## 2. 验证（PoC）

在 `internal/service` 写临时测试端到端验证后删除，实测：

```
URL.Path 到达 handler 时     = "/backend-api/codex/responses/../../me"   （gin 已解码）
派生 suffix                  = "/../../me"
拼出的 targetURL             = "https://chatgpt.com/backend-api/codex/responses/../../me"
下游 HTTP 客户端实际发出       = 原样（Go 不消解 dot-segment）
```

漏洞链路（改前）：

- 路由：`backend/internal/server/routes/gateway.go` 三条 `/responses/*subpath`
- 未校验拼接：`openAIResponsesRequestPathSuffix(c)` 取 `c.Request.URL.Path` 中 `/responses` 之后的部分，经 `appendOpenAIResponsesRequestPathSuffix` 原样拼到上游 URL；调用点在 `openai_gateway_service.go`（OAuth 与 API Key 两种账号均覆盖）
- 中间无任何 `path.Clean` / dot-segment 校验

受影响的是 **OpenAI / Grok / Codex 上游**；Anthropic 分组走 Claude `/v1/messages` 常量、不拼此 suffix，不受影响。

## 3. 生产拓扑与影响面

线上 Sub2API 的上游账号为 **API-Key 类型，`base_url` 指向本机 Aether**（`http://127.0.0.1:<port>`）。据此：

- 路径穿越**改不了 host**（`@`/`//`/`?` 只落在 path/query，authority 已在 base_url 固定），只能在 **Aether 这一个 host:port 上跳路径**，跳不到本机其它服务或其它内网机器。
- "内部地址 / 本机"不构成缓解：Sub2API 本身是公网入口且与 Aether 同机，攻击者借 Sub2API 之手访问本机 Aether，请求以 Sub2API 的可信身份到达。
- 因此实际暴露面为：**任意持普通 Key 的租户 → 借 Sub2API 凭据访问 Aether 主机上的任意路径并读回响应**。严重性取决于 Aether 是否在同 host 暴露敏感路径、以及 Aether 是否也有同类洞——两点见第 5 节，结论是均不成立。

## 4. 改动：`cafecode-v0.0.38`（移植上游 #5137）

上游把客户端可控字符串拼进上游 URL path 的若干位置统一加了**闭集允许清单（默认拒绝）**校验。因 custom-prod 与上游架构已分叉（suffix 函数在 `openai_gateway_service.go`，上游在 `openai_gateway_request_body.go`），采用**人工移植（方案 B）**而非 cherry-pick；移植后守卫逻辑与上游逐字节一致，提交统计 `489+/29−` 与上游 `017f6bbd5` 逐行吻合。

| 文件 | 改动 |
|---|---|
| `backend/internal/service/upstream_path_guard.go`（新增） | 路径片段闭集允许清单：仅放行 `[A-Za-z0-9_.-]`，拒绝空片段、纯点片段（`.`/`..`）、超长（>128）、后缀过深（>8 段） |
| `backend/internal/service/gemini_upstream_url.go`（新增） | `buildGeminiAIStudioModelActionURL`：Gemini AI Studio URL 唯一构造点，校验模型片段 + action 白名单 |
| `backend/internal/server/routes/gateway.go` | 新增 `guardResponsesSubpath` 闭包，包裹三条 `/responses/*subpath`；不可转发子路径入口直接 404 |
| `backend/internal/service/openai_gateway_service.go` | 拆分 `openAIResponsesRequestPathSuffix`，新增 `rawOpenAIResponsesRequestPathSuffix`、`IsForwardableOpenAIResponsesRequestPath`；`append...` 增加兜底校验 |
| `backend/internal/handler/gemini_v1beta_handler.go` | `GeminiV1BetaGetModel` / `GeminiV1BetaModels` 入口校验模型片段（同时覆盖 gemini 与 antigravity 两族路由） |
| `backend/internal/pkg/xai/oauth.go` | `BuildVideoURL` 对 `request_id` 增加纯点片段 / 控制字符校验 |
| `account_test_service.go`、`gemini_chat_completions_compat_service.go`、`gemini_messages_compat_service.go` | 6 处 `fmt.Sprintf` 拼 URL 收敛到 `buildGeminiAIStudioModelActionURL`；`ForwardAIStudioGET` 改用路径护栏 |
| `gateway_test.go` + 两个新增 `*_test.go` | 端到端 + 单元回归测试，锁定畸形子路径被拒、合法子路径（`/compact`、`/{id}/cancel`、正常模型名）不变 |

合法路径行为不变，通配路由保留。

## 5. Aether 侧判定：无需改动

Aether 是独立 Rust 工程（axum + hyper + reqwest），入站为 catch-all `/{*path}` → `proxy_request`，但**上游 URL 按 API 格式重新映射到规范端点**，客户端子路径不参与拼接：

- `crates/aether-provider-transport/src/url.rs` `build_openai_responses_url`：上游后缀是闭集，只可能是 `"/responses"` 或 `"/responses/compact"`，由 `compact: bool` 决定。`chat`/`search`/`messages` 同为固定端点。
- 因此即便有人把 `/v1/responses/../../x` 发到 Aether，planner 只取"是否 compact"的布尔，`../../x` 到不了上游 URL。**"Aether 拿真实 OAuth 号打 chatgpt.com/backend-api/*"这条链在 Aether 这一跳接不起来。**
- 叠加 Sub2API 现已在入口 404 拒绝，此 Responses 链**双重关闭**。

### 待确认的相邻面（非本次漏洞，未确认可利用）

`crates/aether-provider-transport/src/video/mod.rs` 的 OpenAI 视频创建路径（无 `custom_path` 分支）会把 `parts.uri.path()`（去 `/v1` 前缀后）经 `build_passthrough_path_url` 裸拼进上游 URL，结构上属同类。可利用性取决于：是否走该分支（配了 `custom_path` 即不吃客户端路径）、Sub2API 是否透传带 dot-segment 的视频请求、且仍 host-locked。**尚未确认，留待单独排查**；Gemini content 走 `mapped_model`（配置来源），相对安全。

## 6. 验证

- `go build ./...`、`go vet`（service/routes/handler/xai）、`gofmt -l`：均通过
- 定向测试（guard / gemini url / responses suffix / 路由端到端）：通过
- 扩大测试 `go test ./internal/service ./internal/server/routes ./internal/handler`：全部通过
- 完整性复核：三条 responses 路由全部包守卫；gemini URL 拼接零残留（唯一命中为 canonical builder 自身）；gemini/antigravity 的 GET/POST model 路由均由 handler 层校验覆盖；端到端测试用真实 `RegisterGatewayRoutes`，非假绿

## 7. 发布与 Git 状态

- Commit：`f6384c6b6`（仅本次 12 文件，未带入工作区既有改动）
- Push：`8a1082e0b..f6384c6b6 → origin/custom-prod`
- Tag：`cafecode-v0.0.38`（带注解）已创建并 push
- **`last-merged` 未移动**：本次仅审查移植了 #5137 一项，`last-merged..7ceabb3fd` 范围内尚有约 900 个未审提交，移动水位线会导致后续 `/merge-main` 静默跳过它们

## 8. 生产部署（2026-08-01）

- 部署前生产运行 `cafecode-v0.0.36`（digest `sha256:2ea87465…`，`154e8fdaa`），已 4 天。
- 宿主 compose `/opt/stacks/sub2api-deploy/docker-compose.yml` 按 **digest 固定**镜像（非 `:latest`）。更新方式：把 sub2api 服务 image digest 由 v0.0.36 改为 v0.0.38（`sha256:a0789c89…`；`:latest` 已核实 = `cafecode-v0.0.38` / revision `f6384c6b6`），再 `docker compose up -d`。
- v0.0.36 → v0.0.38 **无 DB 迁移**。本次实际生效的运行改动仅两项：本安全修复（v0.0.38）与此前打了 tag 但一直未部署的**套餐并发回填**（v0.0.37 / `9300fa947`，`UpdatePlan` 把套餐组未过期权益抬到新值并失效相关鉴权缓存）；其余为文档提交。
- 重建时 compose 因配置哈希漂移一并重建了 pg/redis（宿主目录卷 `./postgres_data`/`./redis_data`，数据无丢失），三容器均 healthy，`/health` 返回 `{"status":"ok"}`，启动日志无异常。
- 回退：备份 `docker-compose.yml.bak-v0.0.36-20260801-004555`；旧镜像 `ghcr.io/shirtiny/sub2api@sha256:2ea87465be44e15f76faf61b4956b47498f35e0342944bd4b0cc72245d4e4e90`。
- **未随本次上线**：cafe 优惠券月末 `AddDate` 修复（`bd78e9ca5`，独立提交、CI 已绿），随下次 `cafecode-v0.0.39` 发布。

## 9. 攻击痕迹取证（三源交叉，2026-08-01）

修复上线后回查该漏洞是否曾被利用/探测，交叉核对三个数据源：

| 数据源 | 覆盖 | 窗口 | 穿越痕迹 |
|---|---|---|---|
| Cloudflare 边缘（`httpRequestsAdaptiveGroups`，全部入口请求含 4xx/被拦） | host `www.cafecode.work`（zone `cafecode`） | ~30 天（07-02 → 08-01） | **0** |
| sub2api `usage_logs`（成功/计费，`upstream_endpoint` 保留原始子路径） | 应用层 | ~8 天（07-25 → 08-01） | **0** |
| sub2api `ops_error_logs`（≥400 错误，含 `request_path`/`client_ip`/`ua`） | 应用层 | ~3 天（07-30 → 08-01） | **0** |

查询方式：
- CF：`/opt/stacks/ldc-shop/cafe-ban/tools/cf-gql.sh` 发 GraphQL，按 `clientRequestPath` 全量枚举 + 定向搜穿越符（`..`/`%2e`/`%2f`/`%5c`）。
- 应用层：`usage_logs.upstream_endpoint ~ '\.\.'`、`ops_error_logs.request_path` 同类正则。

发现：
- CF 30 天内**无任何含 `../`/`%2e`/`%2f` 的穿越路径**；500 条 path 均为正常 web UI/API 路由、CF 自身端点，及打所有站的通用扫描噪音（`/wp-admin/*`、`/xmlrpc.php` 等，与本漏洞无关）。
- 唯三"非标准"端点均**非穿越**且未得逞：`/v1/models/responses`（404，2 IP）、`/v1/responses/input_tokens`（404，1 IP）、`/1/responses`（200，客户端漏 `v` 笔误）。
- 应用层 `upstream_endpoint`/`request_path` 无一条穿越形态；若曾成功利用，应用层会把原始 `/v1/responses/../../x` 记入 `upstream_endpoint`——没有。

**判定：在所有可查数据窗口内，未发现针对该漏洞的成功利用，也无一次像样的穿越探测。该漏洞在被利用前即已修复。**

盲区（诚实说明）：
- 数据保留有限——CF ~30 天、`usage_logs` ~8 天、`ops_error_logs` ~3 天；更早时段无日志可查（宿主 `/root/clean` 周任务裁剪）。
- **Caddy 未配 `log` 指令，代理层无访问日志**；CF 分析字段会归一化路径，但回源给 origin 的是原始路径，故应用层记录为权威依据。
- CF 全量枚举有 500 行（按量）上限，已用定向穿越搜 + 应用层原始路径记录覆盖该缝隙。

## 10. 后续

- 排查第 5 节 Aether 视频创建透传面是否真可利用；若成立，照 `build_openai_responses_url` 的闭集思路加片段校验。
- cafe 优惠券月末修复随 `cafecode-v0.0.39` 发布。
- 后续 `/merge-main` 处理剩余范围时，本修复内容已对齐上游，`git cherry` 会将其识别为 patch-equivalent 自动跳过。
