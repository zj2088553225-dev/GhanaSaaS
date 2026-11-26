# GhanaSaaS

面向平台自营与第三方商户的多租户电商平台，包含英中双语商城、运营管理后台和 Go API。

## 核心功能

- **多租户与权限**　区分平台和商户，按租户隔离业务数据，支持平台超管、租户管理员及员工角色。
- **商品与类目**　使用 SPU/SKU 管理多规格商品和库存，支持双语类目、Excel 导入导出和 PostgreSQL 模糊搜索。
- **商城与交易**　提供商城、店铺和商品详情页，以及购物车、顾客认证、结算与订单管理。顾客认证、价格展示和预约可按环境启用。
- **内容与运营**　支持可视化首页编辑、SEO 和双语 sitemap，以及浏览、曝光、点击事件的上报、分析和 CSV 导出。
- **业务集成**　提供 HMAC-SHA256 签名 OpenAPI，以及商品预约、联系方式验证和限流。

## 技术栈与代码入口

| 模块 | 技术与位置 |
| --- | --- |
| API | Go、Gin、GORM，[服务入口](cmd/api) |
| 业务与数据访问 | [控制器](internal/controller) → [业务层](internal/service) → [仓储层](internal/repository)，使用 PostgreSQL |
| 商城与管理后台 | Vue 3、TypeScript、Vite、Pinia、Element Plus，[前端源码](saas-web) |
| Linux 部署 | Docker Compose 与 Nginx，[部署说明](docs/production-deployment.md) |

## Linux 部署

从环境变量示例创建 `.env.production`，填写 PostgreSQL 配置与随机生成的 `JWT_SECRET`，再启动 Compose。

```bash
cp .env.production.example .env.production
# 编辑 .env.production 后启动
docker compose --env-file .env.production up -d --build
```

Web 默认监听 `127.0.0.1:3000`，API 默认监听 `127.0.0.1:8080`。首次启动会初始化数据库结构，管理员由 [seeder](cmd/seeder) 单独初始化。需要对外提供 Web 服务时，请参考 [Nginx 与 TLS 配置](docs/deploy-ssl.md)。

[OpenAPI 签名说明](docs/openapi-signature-v2.md) · [预约安全](docs/reservation-security.md) · [运营分析指标](docs/storefront-analytics-metrics.md)\n