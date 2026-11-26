# 新品预约安全与隐私

新品预约不恢复商城账号、密码、购物车、价格、订单或支付。预约可运行在关闭、匿名或
手机号 OTP 三种模式；匿名模式使用服务端生成的浏览器 Cookie 身份，不收集手机号，也
不使用 Canvas、字体、WebGL 或硬件指纹。

## 模式开关

`RESERVATION_MODE` 与 Web 构建变量 `VITE_RESERVATION_MODE` 允许
`disabled`、`anonymous`、`otp`，均默认 `disabled`，部署时必须保持一致。

- `disabled`：只注册只读 capabilities；预约创建、OTP、查询、取消及后台预约路由不注册。
  不构造预约密钥、Turnstile 或 OTP provider。
- `anonymous`：要求 Turnstile、浏览器 HMAC 密钥、IP HMAC 密钥和可信代理网段；不要求
  联系方式密钥、OTP provider、OTP channels 或 Twilio。
- `otp`：保留原手机号 OTP 流程和全部严格生产校验，生产禁止 mock provider。

匿名模式不会把 mock OTP 当作降级方案；切回 `otp` 必须重新提供联系方式密钥和 Twilio
Verify 配置。

## 匿名浏览器身份

首次访问匿名创建或“我的预约”接口时，API 生成 32 个随机字节并设置
`gc_reservation_device` Cookie。Cookie 使用 `HttpOnly`、`Secure`、`SameSite=Lax`、
`Path=/` 和 180 天 `Max-Age`。数据库只保存
`HMAC-SHA256(RESERVATION_BROWSER_HASH_SECRET, cookie_token)`，不保存 Cookie token
明文。清除 Cookie 后会被视为新浏览器，这是匿名模式的明确限制。

匿名预约编号由安全随机值生成，不暴露自增 ID。每条预约默认在
`RESERVATION_ANONYMOUS_TTL_HOURS=24` 后过期；查询、创建、启动清理、周期清理和后台批量
清理都会幂等地把到期记录标记为 `expired`。

## 联系方式

- 使用 libphonenumber 元数据解析和验证国家号码，发送给 provider 前统一为 E.164。
- `RESERVATION_CONTACT_HASH_SECRET` 用带 `reservation-contact:v1:` 域分隔符的
  HMAC-SHA256 生成等值查询键；生产至少 32 字节，并与其他密钥独立。该密钥不能静默
  轮换，否则既有等值查询键将无法命中；轮换必须另行设计双读/回填流程。
- `RESERVATION_CONTACT_ENCRYPTION_KEY` 必须解码为 32 字节，联系方式使用
  AES-256-GCM 加密；每次使用新的随机 nonce，v2 密文使用固定领域 AAD，数据库同时记录
  加密版本。解密保留 v1 兼容，但新写入一律为 v2。
- API、错误和应用日志不输出完整号码。Super Admin 默认只看到类似
  `+233 *** *** 4567` 的掩码。
- OTP、Turnstile token、provider credential、Authorization、JWT 和请求原文均不入库。
- 管理 token 是域分隔 HMAC 导出的 32 字节不可预测 bearer token；相同 verify
  idempotency key 的响应丢失重试会得到同一 token，数据库只保存 SHA-256。其他请求不能
  重新签发，丢失后也不能用手机号枚举恢复。

## Turnstile 与 OTP provider

`RESERVATION_MODE=anonymous` 或 `otp` 的生产环境必须满足以下条件，否则 API 启动失败：

- `TURNSTILE_ENABLED=true`，且 site key、secret key、expected hostname、expected action
  均已配置；secret key 只传给 API，不能进入 Web 构建参数。
- Cloudflare 官方公开测试 site/secret key 仅允许 development/test；生产启动校验和
  `deploy.sh` 预检都会拒绝这些固定测试值。
- `anonymous` 的 expected action 固定为 `reservation_create`；`otp` 固定为
  `reservation_otp`。
- 仅 `otp` 要求 `OTP_PROVIDER=twilio_verify` 和完整的 Twilio Account SID、Auth Token、
  Verify Service SID。`OTP_PROVIDER=mock` 在生产被拒绝。

后端 Siteverify 固定验证配置中的 action 和 hostname，不接受客户端声明；网络超时返回可
重试的 503。应用还对 token digest 做并发/重放保护：同一 idempotency key 只执行一次
Siteverify，其他 key 不能重用该 token。OTP resend 必须提交新的 Turnstile token，
Siteverify 网络重试使用同一 idempotency key。

合法联系方式在 Turnstile 成功后的 OTP request 使用统一最小响应时间
`RESERVATION_OTP_RESPONSE_MIN_MILLISECONDS`（生产默认 750ms），并统一 pending 响应结构，
避免 pending/verified/cancelled/cooldown/contact-limit 与新号码之间出现直接状态枚举。第三方
网络慢于该下限时仍可能产生自然延迟，因此该边界同时依赖 Turnstile 和 contact/IP/global
限流，而不声称公网请求具备密码学意义的 constant-time。

Web 构建只接收 `VITE_TURNSTILE_ENABLED` 和公开的 `VITE_TURNSTILE_SITE_KEY`。SPA
组件以 explicit rendering 加载 Cloudflare 脚本，全局只创建一个 script；每个组件实例
在 mount 后 render，在 token 过期时清空并 reset，在 unmount 时 remove。每次首次发送和
resend 都必须取得新的 token。site key 缺失会显示配置错误，不会伪装验证成功。

## 前台与 Super Admin

公开路径为 `/en/reservations`、`/zh/reservations` 及各自详情页。页面不要求账号或密码，
也不显示价格、购物车、购买、订单或支付动作。前台只返回 `new`、`popular`、`trending`、
`highly_requested` 热度等级，不返回精确兴趣或验证预约人数。

`/gc-control-7x/reservation-products` 及编辑、分析子路由只允许 `super_admin`。后台展示精确
兴趣、提交、验证、取消和拦截聚合，以及脱敏联系方式。当前没有可靠的独立预约曝光事件
源，因此预约 analytics 中 impressions/page_views 保持零，不复用或改变既有 Storefront
Analytics 指标语义，也不伪造历史曝光。

`/gc-control-7x/reservations` 展示匿名/手机号身份类型、随机预约编号、browser/IP 哈希前
12 位、到期时间、近 24 小时限流标记及同身份频次。管理员可确认、拒绝、取消匿名预约、
标记异常并批量执行幂等过期处理；完整哈希、原始 IP 和 Cookie token 永不返回后台。

## IP、可信代理与限流

IP 只用于限流和风险聚合，不是用户 ID。匿名模式使用独立的稳定
`RESERVATION_IP_HASH_SECRET` 计算 HMAC-SHA256；OTP 模式继续使用绑定 UTC 日期的短期
风险键。数据库、后台和应用日志都不保存原始 IP；后台只显示哈希前 12 位。

`RESERVATION_TRUSTED_PROXY_CIDRS` 必须精确列出部署链路中可信的反向代理网段。只有直接
peer 位于这些 CIDR 时，预约模块才接受 `CF-Connecting-IP`，否则从右到左解析可信的
`X-Forwarded-For` 代理链；非可信直接来源只能使用 `Request.RemoteAddr`。
`0.0.0.0/0`、`::/0` 和任意客户端可写 header 均不会被信任。

匿名模式在事务内按已接受创建事件执行 browser 小时/日、IP 小时/日、IP+商品日五个
窗口，默认分别为 5、20、20、100、3。相同 browser hash 与商品的有效预约使用数据库
唯一键兜底，取消、拒绝或过期时释放该键。当前单 API 实例还用进程内 key lock 减少同一
身份竞争；扩展多实例前应把高吞吐限流迁移到 Redis 等共享原子存储，数据库唯一约束仍是
最终去重边界。

## 审计保留

`reservation_attempt_events` 只保存聚合所需 action/result/reason 和可选的 visitor、
contact、browser、IP HMAC。默认保留策略为 30 天，由
`RESERVATION_ATTEMPT_RETENTION_DAYS=30` 记录。本版本不自动执行 destructive purge；
上线后应通过单独审核的维护任务实现保留策略。

后台不展示逐条风险事件，也不提供完整联系方式 CSV。未来通知导出必须另设权限和审计。
