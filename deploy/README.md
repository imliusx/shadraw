# shadraw 生产部署

单台 VPS,docker compose 跑 Next.js + Go API + Postgres + MinIO,
宿主机 nginx 做反代和 TLS。

## VPS 上的目录布局

```
~/shadraw-studio/
├── shadraw/           # 后端仓库 (本仓库)
│   └── deploy/        # ← 在这里跑 ./deploy.sh
└── shadraw-ui/        # 前端仓库
```

`shadraw` 和 `shadraw-ui` 要并列放在同一个父目录,`docker-compose.prod.yml`
通过 `../../shadraw-ui` 引用前端构建上下文。

## 首次部署

1. 在 VPS 上装 Docker + Docker Compose plugin (Ubuntu: `apt install docker.io docker-compose-plugin`)
2. 克隆两个仓库:
   ```
   mkdir -p ~/shadraw-studio && cd ~/shadraw-studio
   git clone https://github.com/imliusx/shadraw.git
   git clone https://github.com/imliusx/shadraw-ui.git
   ```
3. 配置环境变量:
   ```
   cd shadraw/deploy
   cp .env.prod.example .env
   # 编辑 .env: 生成随机的 JWT_SECRET / MASTER_KEY / POSTGRES_PASSWORD / S3_SECRET_ACCESS_KEY
   # 填入 SITE_URL=https://your-domain.com, ADMIN_EMAIL
   ```
4. DNS A 记录把 `your-domain.com` 指向 VPS IP
5. 一键启动:
   ```
   ./deploy.sh
   ```
6. 配置宿主机 nginx (见下一节)

## nginx 反代

容器把端口绑在宿主机 loopback 上,**外部不可达**,只能通过宿主机 nginx 访问:

| 服务 | 宿主端口 | nginx 上游 |
|---|---|---|
| Next.js (web) | `127.0.0.1:3000` | `location /` |
| Go API (api) | `127.0.0.1:8080` | `location /api/` + `location = /healthz` |
| Postgres | (仅 docker 内网) | 不暴露 |
| MinIO | (仅 docker 内网) | 不暴露 |

nginx server 块的核心:

```nginx
location /api/ {
    proxy_pass         http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header   Host              $host;
    proxy_set_header   X-Real-IP         $remote_addr;
    proxy_set_header   X-Forwarded-For   $proxy_add_x_forwarded_for;
    proxy_set_header   X-Forwarded-Proto $scheme;
    proxy_read_timeout 300s;   # 生图请求可能比较久
}

location = /healthz {
    proxy_pass http://127.0.0.1:8080;
}

location / {
    proxy_pass         http://127.0.0.1:3000;
    proxy_http_version 1.1;
    proxy_set_header   Host              $host;
    proxy_set_header   X-Real-IP         $remote_addr;
    proxy_set_header   X-Forwarded-For   $proxy_add_x_forwarded_for;
    proxy_set_header   X-Forwarded-Proto $scheme;
    proxy_set_header   Upgrade           $http_upgrade;
    proxy_set_header   Connection        "upgrade";
}
```

`.env` 里的 `SITE_URL` 必须和 nginx server_name + 协议完全一致,例如
`SITE_URL=https://shadraw.example.com`,前端 build 会把它写进客户端 bundle。

## 访问地址

部署完成后,直接打开 `SITE_URL` 即可 (例如 `https://shadraw.example.com`)。
前端在根路径,API 走 `/api/v1/*`,健康检查 `/healthz`。

## 升级

```
cd ~/shadraw-studio/shadraw    && git pull
cd ~/shadraw-studio/shadraw-ui && git pull
cd ~/shadraw-studio/shadraw/deploy
./deploy.sh
```

`deploy.sh` 默认走 `up -d --build`,只重建有变更的镜像。

## 其他常用命令

```
./deploy.sh ps      # 看服务状态
./deploy.sh logs    # 跟踪日志
./deploy.sh down    # 停服,卷保留
```

## 数据备份

- Postgres:
  ```
  docker compose -f docker-compose.prod.yml exec -T db pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB" \
    | gzip > backup-$(date +%F).sql.gz
  ```
- MinIO: 数据在 docker named volume `shadraw_miniodata`。例如:
  ```
  docker run --rm -v shadraw_miniodata:/data -v "$(pwd)":/backup alpine \
    tar czf "/backup/minio-$(date +%F).tgz" /data
  ```

## 改了 SITE_URL 怎么办

`NEXT_PUBLIC_API_BASE` 是 **build-time** 写进前端 bundle 的。改了 `.env` 里的
`SITE_URL` 之后必须重 `--build`,直接 restart 不生效。`./deploy.sh` 已经带
`--build`。

## 故障排查

- 容器没起:`./deploy.sh ps` + `./deploy.sh logs`
- 前端能开但调 API 报 CORS / 404:检查 `SITE_URL` 是否和 nginx server_name 一致
- nginx 502:确认容器在跑 (`docker compose ps`) 且监听的是 `127.0.0.1:3000` / `:8080`
  (`ss -tlnp | grep -E '3000|8080'`)
