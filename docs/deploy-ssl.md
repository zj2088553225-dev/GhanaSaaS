# 外层 TLS / SSL 部署指南

项目 Compose 不管理证书，也不向 `web` 容器挂载证书。`api` 和 `web` 只监听宿主机
`127.0.0.1:8080` 与 `127.0.0.1:3000`；公网域名和 TLS 应由宿主机 Nginx、Nginx
Proxy Manager、Caddy 或同等外部反向代理负责。

## Host Nginx + Certbot

1. 将 `deploy/nginx-host.conf` 复制为宿主机站点配置并替换域名。
2. 检查并 reload 宿主机 Nginx。
3. 使用 Certbot 为宿主机 Nginx 签发证书：

```bash
sudo certbot --nginx -d example.com -d www.example.com
```

续期由宿主机 Certbot timer 管理，可用以下命令演练：

```bash
sudo certbot renew --dry-run
```

证书申请和续期不需要停止或重建项目容器。

## 外部 Proxy Manager

使用 Nginx Proxy Manager 等服务时，将 upstream 指向宿主机的 Web HTTP 端口
`3000`。Web 容器会把 `/api/v1/`、`/sitemap.xml` 和 `/uploads/` 转发到 API。
也可以按 `deploy/nginx-host.conf` 分别代理这些路径。

- `/api/v1/` 必须保留 300 秒 timeout，支持导入和大文件操作。
- `/sitemap.xml` 必须单独代理到 API。
- `/uploads/` 的不存在文件必须保持 404。
- 不添加 maintenance rewrite。
- `/gc-control-7x` 是后台入口；不要恢复 `/admin` 或 `/login`。
- Cloudflare 或外层 CDN 不应缓存 `/api/v1/` 响应。

项目 Web 容器的 `/healthz` 仅用于内部容器健康检查，不替代公网 TLS 监控。
