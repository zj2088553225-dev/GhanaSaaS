# 生产部署 Runbook

这是可验证、可回滚的单机 Docker Compose 部署，不是严格零停机蓝绿部署。更新
`api`、`web` 时可能有短暂重建窗口；数据库容器和 volume 不会因应用发布而重建。

## 部署前

1. 选择完整 commit hash 或已审核 tag 作为 `DEPLOY_REF`，确认服务器工作区干净。
2. 从 `.env.production.example` 准备 `.env.production`，权限建议为 `600`。
3. 确认 `APP_ENV=production`，并使用至少 32 字节且通过应用强度校验的随机
   `JWT_SECRET`。不要把密钥写入命令历史或部署文档。
4. 明确设置 `OPENAPI_LEGACY_SIGNATURE_ENABLED=false`；所有 OpenAPI 客户端先升级
   HMAC-SHA256 v2。
5. 明确设置前后端 storefront auth/price flags，保持 auth disabled、price hidden；
   `CUSTOMER_PASSWORD_RESET_ENABLED=false` 与
   `VITE_CUSTOMER_PASSWORD_RESET_ENABLED=false`。当前版本没有邮件投递 provider，
   启动和部署预检都会拒绝开启密码重置，不能向用户展示虚假的邮件发送结果。
   `VITE_ADMIN_BASE_PATH=/gc-control-7x`。
6. 确认 `POSTGRES_USER`、`POSTGRES_PASSWORD`、`POSTGRES_DB` 和
   `PLATFORM_MALL_TENANT_ID`。
7. 使用 `df -h` 和 `docker system df` 确认镜像及数据库备份空间充足。不要运行 prune。
8. 阅读只读预检 [product-invariant-preflight.sql](product-invariant-preflight.sql)。部署脚本
   会在备份和容器替换前通过数据库容器内的 `psql` 执行它；发现 orphan product
   category 或重复非空 SKU code 时立即停止，不自动修复数据。
9. 首次初始化管理员只在人工运行 seeder 时设置 `INITIAL_SUPER_ADMIN_*` 和
   `INITIAL_TENANT_ADMIN_*`。普通部署不创建管理员，也不要长期保留初始化密码变量。
10. 明确设置 `RESERVATION_MODE` 和 `VITE_RESERVATION_MODE` 且保持一致，允许
    `disabled`、`anonymous`、`otp`，默认均为 `disabled`。匿名生产按
    [新品预约安全与隐私](reservation-security.md) 配置独立 browser/IP HMAC 密钥、可信
    代理网段、Turnstile hostname 和 `reservation_create` action，不需要联系方式密钥或
    Twilio。OTP 模式仍要求联系方式 HMAC/AES 密钥、`reservation_otp` action 和 Twilio
    Verify，且生产禁止 mock。Turnstile secret 和 Twilio Auth Token 不得进入 Web 构建参数。
11. 使用统一的 `TRUSTED_PROXY_CIDRS` 配置 Cloudflare、NPM、Web 到 API 的受控代理链。
    旧 `RESERVATION_TRUSTED_PROXY_CIDRS` 只保留一版兼容回退。应用仅在 TCP 直接来源
    属于该列表时读取 `CF-Connecting-IP`、`X-Forwarded-For` 或 `X-Real-IP`，并从
    XFF 右向左剥离可信代理。Cloudflare/NPM 必须覆盖或删除外部请求自带的
    `CF-Connecting-IP`；源站应限制为仅 Cloudflare/NPM 可达。仓库内 Web Nginx 会清空
    CF header 并追加 XFF，避免把未经验证的浏览器 header 继续传给 API。
12. Cloudflare 与 Nginx Proxy Manager 缓存规则必须 bypass `/api/*`、
    `/gc-control-7x/*`、登录/注册/密码重置/account 页面，以及
    `/:locale/reservations/mine`。认证 API 自身返回 `Cache-Control: no-store, private`；
    SPA HTML 使用 `no-cache, must-revalidate`，hash 静态资源仍可长期缓存。
13. Web Nginx 返回 HSTS、`nosniff`、Referrer/Permissions Policy、拒绝被嵌入，
    并先使用 `Content-Security-Policy-Report-Only` 覆盖当前 Turnstile、Google Maps、
    Meta Pixel 和 Google Analytics 来源。Cloudflare/NPM 不得删除或改写这些 Header。
    发布后检查浏览器/边缘日志中的 CSP violation，再单独审查是否切换为强制策略；本版本
    不把 report-only 当成 XSS 防护边界。Custom Head 仍只允许 super admin 配置，每段
    代码必须人工审查。

## 商品热度缓存与索引

API 使用进程内并发安全短缓存。设置
`PRODUCT_POPULARITY_CACHE_TTL_SECONDS=120`（有效范围 1–3600 秒）。缓存键隔离公开/后台
模式、租户范围、类目、limit 和 locale。多实例之间可能有最多一个 TTL 的刷新时间差；
缓存只是优化，数据库查询仍是权威数据源。

大型事件索引不会由 API 启动迁移创建。生产发布前先运行只读预检并保存输出：

```bash
docker compose exec -T db psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
  < docs/product-popularity-index-preflight.sql
```

确认数据库快照可恢复后，在低流量窗口、事务外运行：

```bash
docker compose exec -T db psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
  < docs/product-popularity-index-concurrently.sql
```

脚本幂等，使用 `CREATE INDEX CONCURRENTLY IF NOT EXISTS`，设置 5 秒
`lock_timeout` 和 30 分钟 `statement_timeout`，不会更新事件数据。完成后重新运行预检，
确认 `indisvalid=true` 且 `indisready=true`。若中断留下 invalid index，先检查活动会话，
再执行 `DROP INDEX CONCURRENTLY idx_storefront_events_popularity_human` 后重试。删除该索引
就是数据库层回滚；应用仍兼容，只是查询成本升高。

可先进行不接触 Docker 和数据库的配置检查：

```bash
DEPLOY_REF=<FULL_COMMIT_HASH_OR_TAG> DRY_RUN=true ./deploy.sh
```

## 部署

```bash
DEPLOY_REF=<FULL_COMMIT_HASH_OR_TAG> ./deploy.sh
```

脚本按以下顺序执行：

1. 检查参数、生产配置和干净工作区，fetch tags，解析并记录目标完整 commit。
2. 记录当前容器、api/web image 名称和 image ID，然后切换到目标 detached commit。
3. 执行只读 product invariant preflight。
4. 使用 `pg_dump -Fc` 写入 `backups/production/`；空文件或失败均停止部署。
5. 当前容器继续运行时构建 `saas-api:<commit>`、`saas-web:<commit>` 本地镜像。
6. 只替换 api/web，不停止 db，不删除 volume。
7. 等待 API `/healthz` 和 Web `/healthz` 的 Compose healthcheck。
8. 健康后写入 `.deploy/current-attempt.env` 和 `.deploy/last-successful.env`。

运行状态和备份均被 gitignore，不含密码或 token。备份不会被脚本自动删除。

## 健康验证

```bash
docker compose --env-file .env.production ps
curl --fail --max-time 5 http://127.0.0.1:8080/healthz
curl --fail --max-time 5 http://127.0.0.1:3000/healthz
```

随后人工验证：

- `/en`、`/zh` 首页和公开 homepage API。
- `/gc-control-7x/login` 管理员登录，确认 `/admin`、`/login` 仍为 NotFound。
- 后台平台类目中英文远程搜索和商品页。
- storefront 价格仍隐藏，顾客注册/登录/订单/checkout 仍关闭。
- 首页 CMS `published_at`、轮播图片 fallback、product floor 类目诊断。
- `/en`、`/zh` 默认 SKU 文案。
- 被商品引用的平台类目删除仍返回业务错误。

## 平台类目搜索索引受控重建

API 启动只会幂等创建 `pg_trgm` 扩展、`search_text` GIN 索引和查询排序 B-tree
索引，不会自动全量重建历史类目搜索数据。首次启用或发现索引不一致时，必须先完成数据库
备份，再使用 API 镜像内的维护命令。命令复用应用数据库配置，不在服务器手工执行 SQL，
也不得把 DSN 或密码写入命令行。

先执行零写入审计。`--verify` 在发现待创建、待更新、孤立或无效行时返回非零；这是阻止
未经审查继续发布的预期行为，应保存并检查摘要：

```bash
docker compose --env-file .env.production run --rm api \
  ./platform-category-search-rebuild \
  --dry-run \
  --verify
```

确认 `categories`、`expected_rows`、`created`、`updated`、`unchanged`、
`deleted_orphans`、`missing_translations`、`fallback_zh_rows` 和 `invalid_rows` 后，
在维护窗口显式执行：

```bash
docker compose --env-file .env.production run --rm api \
  ./platform-category-search-rebuild \
  --apply \
  --verify
```

随后重复同一条 `--apply --verify` 命令，必须得到 `created=0`、`updated=0` 且
`verified=true`。任一步失败都不得继续 Web 发布。需要缩小修复范围时可增加
`--category-id=<id>`；默认批量大小为 500，可用 `--batch-size` 调整，但不得通过 API
启动过程自动重建。重建命令使用事务和 advisory lock 防止与另一次重建或类目索引更新
并发产生重复/过期行。

## 受控美国联邦节假日导入

API 启动和 `deploy.sh` 都不会自动写入节假日。部署并完成数据库备份后，先人工查看
只读 diff；命令默认也是 dry-run：

```bash
docker compose --env-file .env.production run --rm api \
  ./holiday-import \
  --file /app/holiday-data/us-2026.example.json \
  --country US \
  --dry-run
```

确认 `created`、`updated`、`unchanged`、`conflicts` 和 `rejected` 后，再显式执行一次事务导入：

```bash
docker compose --env-file .env.production run --rm api \
  ./holiday-import \
  --file /app/holiday-data/us-2026.example.json \
  --country US \
  --apply
```

导入严格校验整份文件，任意非法记录都会拒绝整批。业务唯一键为
`country_code + holiday_date + name_en`；重复 apply 是幂等的，不删除数据库中的人工条目，
也不修改其他国家数据。若已有同行的字段与 dataset 不同，默认 dry-run/apply 会列为
`conflict`，apply 整批零写入，以保护后台人工修正。只有人工审查 diff 后才可显式增加
`--overwrite` 覆盖冲突行。命令使用 API 的现有数据库配置，不应向命令行传递 DSN 或密码。

API `/healthz` 不泄漏配置。API 在监听前已经完成 DB Ping、只读 invariant 检查、
AutoMigrate 和约束确认，因此启动失败不会被标记 healthy。

## 应用回滚

目标容器替换或健康检查失败时，脚本自动尝试恢复部署前记录的 api/web image，并再次
等待健康；无论自动恢复是否成功，本次部署都返回非零。

需要人工恢复最后一次成功镜像时，先检查非敏感状态文件：

```bash
set -a
source .deploy/last-successful.env
set +a
API_IMAGE="$TARGET_API_IMAGE" WEB_IMAGE="$TARGET_WEB_IMAGE" \
  docker compose --env-file .env.production up -d --no-deps --force-recreate api web
```

然后重新执行内部健康验证。不要执行 `docker compose down`，不要删除数据库 volume。

应用回滚只恢复代码和镜像。API 启动可能已经执行 AutoMigrate，因此应用回滚成功不等于
数据库 schema 回滚。

2.0 相关 schema 变化包括：

- `open_api_request_nonces` 及 app key/nonce 唯一索引。
- `spus.platform_category_id` 的 NOT NULL 与平台类目 `RESTRICT` FK。
- `skus.erp_sku_code` 的非空部分唯一索引。
- `pg_trgm`、平台类目双语派生搜索表及其 GIN/B-tree 索引。应用启动不回填历史搜索行。

## 数据库恢复（高风险、仅人工确认）

数据库备份恢复不是自动 application rollback 的一部分。恢复会覆盖当前生产数据，必须在
确认备份时间、目标 commit、影响范围并获得人工批准后执行。

部署脚本生成的是 PostgreSQL custom-format dump，必须使用 `pg_restore`，不能使用
`psql < backup.dump`：

```bash
set -a
source .env.production
set +a

docker compose --env-file .env.production stop api
docker compose --env-file .env.production exec -T db \
  pg_restore --exit-on-error --clean --if-exists \
  -U "$POSTGRES_USER" -d "$POSTGRES_DB" < backups/production/<confirmed-backup>.dump
docker compose --env-file .env.production up -d --no-deps api web
```

恢复前应另存当前数据库，恢复后检查 schema、应用日志和全部健康验证。不要由失败 trap
自动执行数据库恢复。

## 2.0 上线清单

- product invariant preflight 通过：orphan category count = 0。
- duplicate non-empty SKU code count = 0。
- `APP_ENV=production` 且 `JWT_SECRET` 合格。
- OpenAPI 客户端已升级 v2，legacy signature switch 为 false。
- 初始化管理员变量只在首次人工 seeder 时使用，随后移除。
- 首页 CMS `published_at` 正确。
- `/en`、`/zh` 默认 SKU、轮播 fallback、product floor 诊断通过。
- 平台类目删除 RESTRICT 回归通过。
- 平台类目搜索 dry-run/apply/重复 apply/verify 摘要已审核，中英文及混合搜索通过。
