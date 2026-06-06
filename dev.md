# Sub2API 本地开发启动流程

本文记录当前项目在本机源码开发模式下的启动流程，以及容易踩坑的注意事项。

## 依赖服务

本地开发需要先准备：

- Go
- Node.js / pnpm
- PostgreSQL
- Redis

当前本机通常已有 PostgreSQL / Redis 容器在后台运行：

```bash
docker ps | grep -E 'sub2api|postgres|redis'
```

期望端口：

- PostgreSQL: `127.0.0.1:5432`
- Redis: `127.0.0.1:6379`

## 配置文件

源码开发模式下，后端默认使用：

```text
backend/config.yaml
```

当前 `backend/config.yaml` 连接本机 PostgreSQL / Redis。

此外，本项目的本地开发环境变量放在：

```text
deploy/.env
```

注意：直接运行 `go run ./cmd/server` **不会自动读取** `deploy/.env`，需要显式 source。

## 推荐启动方式：源码开发模式

### 1. 启动后端

从项目根目录执行：

```bash
set -a
. ./deploy/.env
set +a
go -C ./backend run ./cmd/server
```

或一行执行：

```bash
set -a; . ./deploy/.env; set +a; go -C ./backend run ./cmd/server
```

后端地址：

```text
http://127.0.0.1:8080
```

健康检查：

```bash
curl http://127.0.0.1:8080/health
```

### 2. 启动前端

另开一个终端，从项目根目录执行：

```bash
pnpm --dir ./frontend run dev -- --host 0.0.0.0
```

前端地址：

```text
http://127.0.0.1:3000
```

前端 Vite 配置默认代理到：

```text
http://localhost:8080
```

如需改后端代理地址，可设置：

```bash
VITE_DEV_PROXY_TARGET=http://127.0.0.1:8080
```

## 登录账号

当前本地环境管理员账号通常为：

```text
邮箱：admin@sub2api.local
密码：见 deploy/.env 中的 ADMIN_PASSWORD
```

## 支付开发模式注意事项

本地调试支付时，可以通过开发直通支付模式绕过真实支付。

`deploy/.env` 中需要有：

```env
PAYMENT_DEV_AUTO_SUCCESS=I_UNDERSTAND_THIS_BYPASSES_REAL_PAYMENTS
PAYMENT_DEV_ENVIRONMENT=development
```

注意事项：

1. **源码开发模式必须 source `deploy/.env`**

   如果只运行：

   ```bash
   go -C ./backend run ./cmd/server
   ```

   后端不会读取 `deploy/.env`，支付开发变量不会生效。

2. **生产保护**

   开发直通支付需要精确 token，并且 `PAYMENT_DEV_ENVIRONMENT` 必须是开发环境值，例如：

   - `development`
   - `dev`
   - `local`

   如果 `APP_ENV` / `ENVIRONMENT` / `NODE_ENV` / `GO_ENV` 为 `production`，不应启用开发直通支付。

3. **支付按钮灰掉的常见原因**

   前端支付按钮是否可点击依赖 `/api/v1/payment/checkout-info` 返回的 `methods`。

   如果本地没有配置任何真实支付 provider，且开发直通支付没有生效，`methods` 可能为空，按钮会灰掉。

   当前代码在开发直通支付开启、且实际支付方式为空时，会自动提供一个虚拟 `alipay` 支付方式，方便前端点击测试。

4. **验证支付开发变量是否生效**

   不建议直接打印整个进程环境，因为里面可能包含数据库密码、JWT secret 等敏感信息。

   更安全的验证方式是：

   - 确认后端是通过 `set -a; . ./deploy/.env; set +a; go -C ./backend run ./cmd/server` 启动
   - 打开支付页，看按钮是否可点击
   - 如仍异常，检查 `/api/v1/payment/checkout-info` 返回的 `methods`

## Docker Compose 启动方式

如果使用 Docker Compose，而不是源码双进程开发：

### local compose

```bash
cd deploy
docker compose -f docker-compose.local.yml up -d --force-recreate
```

### dev compose

```bash
cd deploy
docker compose -f docker-compose.dev.yml up -d --build --force-recreate
```

`docker-compose.dev.yml` 和 `docker-compose.local.yml` 都需要透传：

```yaml
- PAYMENT_DEV_AUTO_SUCCESS=${PAYMENT_DEV_AUTO_SUCCESS:-}
- PAYMENT_DEV_ENVIRONMENT=${PAYMENT_DEV_ENVIRONMENT:-}
```

如果修改了 `deploy/.env`，需要 recreate 容器才能让新环境变量进入容器。

## 常用开发命令

### 前端检查

```bash
pnpm --dir frontend run typecheck
pnpm --dir frontend run lint:check
```

### 后端测试

```bash
go -C backend test ./...
```

支付相关单元测试示例：

```bash
go -C backend test -tags=unit ./internal/service ./internal/handler -run 'Test(PaymentDevAutoSuccess|CreateOrderDevAutoSuccess|Payment|Affiliate|Redeem)'
```

### 后端 lint

提交前建议运行：

```bash
cd backend
golangci-lint run ./...
```

## 重启流程

源码开发模式重启时：

1. 停止当前后端进程
2. 停止当前前端进程
3. 重新以 `deploy/.env` 启动后端
4. 重新启动前端
5. 访问：

```text
http://127.0.0.1:3000
```

## 故障排查

### 端口占用

检查端口：

```bash
lsof -i :8080
lsof -i :3000
```

### 数据库 / Redis 没启动

```bash
docker ps | grep -E 'postgres|redis|sub2api'
```

### 支付页按钮灰掉

检查：

1. 后端是否 source 了 `deploy/.env`
2. `deploy/.env` 是否包含支付开发变量
3. `/api/v1/payment/checkout-info` 的 `methods` 是否为空
4. 后端是否已重启

### 前端代理异常

确认后端健康检查通过：

```bash
curl http://127.0.0.1:8080/health
```

确认前端 Vite 代理目标为：

```text
VITE_DEV_PROXY_TARGET 或默认 http://localhost:8080
```
