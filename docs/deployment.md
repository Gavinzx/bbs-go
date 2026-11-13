# bbs-go 部署说明

本文说明当前这套「平台知识管理工具」的组件、部署方式、初始化、持久化、升级与排障。

**范围**：当前仓库 `bbs-go` 的可运行服务及其 Docker/离线部署资产。  
**不在本文范围**：Kubernetes、多副本高可用、云数据库/对象存储的开通、反向代理证书申请、邮件服务器本身的搭建。这些只说明接入点，不展开供应商操作。

---

## 1. 组件总览

系统不是单一 Go 进程，Docker 镜像内实际包含「Go API + Node SSR 前端」两个进程，由 `docker/entrypoint.sh` 同时拉起。

```text
浏览器
  │  :3000（对外）
  ▼
┌──────────────────────────────────────────────┐
│ bbs-go 容器                                  │
│  ├─ Node SSR 前端  :3000                     │
│  │    /api/*、/res/*、/sitemap.xml 反向代理 ─┼─┐
│  └─ Go 后端        :8082（仅在容器内）        │◄┘
│       ├─ REST API / 管理后台接口             │
│       ├─ /res 静态资源与上传文件             │
│       ├─ /sitemap.xml                        │
│       └─ 内嵌 SPA（直接访问 8082 时使用）     │
└──────────────────────────────────────────────┘
  │                         │
  ▼                         ▼
MySQL 8.4 容器            宿主机 docker-data/
  :3306（仅容器网络）      ├─ data/bbs-go.yaml
                           ├─ data/topic_index/   (Bleve 索引)
                           ├─ logs/
                           └─ uploads/            (本地附件)
```

| 组件 | 作用 | 端口 | 持久化 | 是否必需 |
|---|---|---:|---|---|
| Node SSR 前端 | React Router 7 SSR、静态资源、反向代理 API | 3000 | 无状态，镜像内构建产物 | Docker 形态必需 |
| Go 后端 | API、管理后台、认证、检索、上传、内嵌 SPA | 8082 | 配置、索引、上传、日志 | 必需 |
| MySQL 8.4 | 主数据库 | 3306（仅内网） | `docker-data/mysql` 或命名卷 | Docker 形态必需；原生可改用 SQLite |
| SQLite | 单文件数据库 | — | `bbs-go.db` | 原生形态可选 |
| Bleve | 站内全文检索 | — | `data/topic_index/` | 内置于 Go；可重建 |
| 本地文件存储 | 附件/图片 | — | `res/uploads` | 默认；也可配置 OSS/COS/S3 |
| SMTP / OAuth / 短信 | 邮件通知、第三方登录、短信登录 | — | 配置存数据库 | 可选，后台按需配置 |

**没有** Redis、Elasticsearch、消息队列；这些不是当前部署的前置依赖。

---

## 2. 部署形态

### 2.1 Docker Compose（推荐）

适用：生产/测试服务器，希望一条命令拉起数据库和应用。

镜像构建结构见根目录 [Dockerfile](../Dockerfile)：

- `web-builder`：Node 24 + pnpm 10.30.2，构建 SSR 和 SPA 两套产物。
- `server-builder`：Go 1.26，把 SPA 嵌入二进制。
- 运行镜像：Node 24 Alpine，包含 Go 二进制、Node SSR 产物、`locales`、`res`、默认配置。
- MySQL 使用官方 `mysql:8.4`。

入口：

- `3000`：对外访问入口（Node SSR）。
- `8082`：Go API，Compose 默认**不映射到宿主机**，只由 Node 通过 `BBSGO_SERVER_URL=http://127.0.0.1:8082` 访问。
- `3306`：MySQL，Compose 默认只在容器网络内。

### 2.2 离线 Docker 包

适用：内网服务器，目标机不能访问 Docker Hub/npm/Go proxy。

打包脚本：[scripts/package-docker-offline.sh](../scripts/package-docker-offline.sh)

```bash
# 在有网、有 Docker 的构建机执行
./scripts/package-docker-offline.sh
```

产物：`dist/bbs-go-docker-offline/`

```text
bbs-go-docker-offline/
├── images/
│   ├── bbs-go-offline.tar
│   └── mysql-8.4.tar
├── docker-compose.yaml
├── load-images.sh
├── deploy.sh
└── README.md
```

目标机：

在构建机先把目录打包，再传到目标机：

```bash
tar czf bbs-go-docker-offline.tar.gz -C dist bbs-go-docker-offline
```

目标机：

```bash
tar -xzf bbs-go-docker-offline.tar.gz
cd bbs-go-docker-offline
./deploy.sh
```

`deploy.sh` 会：

1. `docker load` 加载两个镜像；
2. 创建 `docker-data/{mysql,data,logs,uploads}`；
3. `docker compose up -d`。

默认架构为 `linux/amd64`；ARM64 目标需在构建时设置 `DOCKER_DEFAULT_PLATFORM=linux/arm64`，并确认 MySQL 镜像有对应架构。

### 2.3 原生二进制 + 内嵌 SPA

适用：不想跑 Docker，或希望单进程部署。

前置：Go 1.26、Node 24、pnpm 10.30.2（仅构建需要，运行不需要 Node）。

```bash
cd web
corepack pnpm install --frozen-lockfile
corepack pnpm build:spa
cd ..

go build -o bbs-go ./main.go
./bbs-go
```

也可以直接：

```bash
make build    # 先构建 SPA，再编译 Go 二进制
./bbs-go
```

访问：`http://<host>:8082/`（默认端口来自 `bbs-go.yaml` 的 `port: 8082`）。

运行目录需要包含：

- `bbs-go.yaml`（首次安装前可复制 [bbs-go.example.yaml](../bbs-go.example.yaml) 并按需修改）；
- `locales/`、`res/`（随二进制部署）；
- 可写目录：`logs/`、`res/uploads/`、`data/`（Bleve 索引）；
- SQLite 模式还需要 `bbs-go.db` 所在目录可写。

原生模式不启动 Node；SPA 已通过 `go:embed` 嵌入 Go 二进制。只有需要 SSR 时才需要额外运行 Node。

### 2.5 CentOS 7 + SQLite 离线包

针对 CentOS 7 和 SQLite 的轻量离线包位于 `deploy/centos7-sqlite/`，构建后生成：

```text
dist/bbs-go-centos7-sqlite-offline/
dist/bbs-go-centos7-sqlite-offline.tar.gz
```

包内包含应用镜像、可选 Docker 20.10.24 静态运行时、Compose 插件、启动脚本和 `manual/部署手册.pdf`。运行时采用「Go 单进程 + 内嵌 SPA」形态，不启动 Node SSR，也不再需要 MySQL：

```bash
cd bbs-go-centos7-sqlite-offline
./deploy.sh
```

默认访问 `http://<host>:8082/`，安装向导默认选中 SQLite。完整步骤见包内 [部署手册](../deploy/centos7-sqlite/manual/部署手册.md)。

### 2.4 开发模式

仅用于本地开发，不用于生产：

```bash
# 终端 1：后端（dev 标签从磁盘读 SPA）
go run -tags dev main.go

# 终端 2：前端 Vite dev server
cd web
corepack pnpm dev
```

`web/.env` 需要：

```env
BBSGO_SERVER_URL=http://localhost:8082
```

访问 `http://localhost:3000`，Vite 把 `/api`、`/res`、`/sitemap.xml` 代理到 8082。

---

## 3. Docker 部署步骤

### 3.1 前置条件

- Docker Engine 24+，Docker Compose v2（`docker compose` 命令）。
- 目标机可拉取/加载镜像；离线场景使用上面的离线包。
- 建议至少 2 vCPU、4 GB 内存、10 GB 可用磁盘；实际取决于附件量。
- 宿主机端口 `3000` 可用；若被占用，见 3.3。

### 3.2 使用仓库内 `docker-compose.yml`

根目录 Compose 使用官方镜像 `mlogclub/bbs-go:${BBSGO_IMAGE_TAG:-latest}`，并自带一个 MySQL 服务：

```bash
docker compose up -d
```

如果使用本地构建的镜像，需要先让 Compose 能找到它：

```bash
docker build -t mlogclub/bbs-go:offline .
BBSGO_IMAGE_TAG=offline docker compose up -d
```

默认数据位置：

- MySQL：命名卷 `mysql-data`（项目名前缀）；
- 应用：`./docker-data/data`、`./docker-data/logs`、`./docker-data/uploads`。

> 根 Compose 的 `3000:3000` 是写死的；需要改宿主机端口时，优先用离线 Compose（支持 `BBSGO_WEB_PORT`），或修改 `docker-compose.yml`。

### 3.3 使用离线 Compose（支持改端口）

编辑 `docker-compose.yaml` 同目录的 `.env`（没有就新建）：

```env
BBSGO_WEB_PORT=3000
BBSGO_DOCKER_MYSQL_DATABASE=bbsgo
BBSGO_DOCKER_MYSQL_USER=bbsgo
BBSGO_DOCKER_MYSQL_PASSWORD=请替换为强密码
BBSGO_DOCKER_MYSQL_ROOT_PASSWORD=请替换为强密码
```

启动：

```bash
./deploy.sh
# 或
docker compose -f docker-compose.yaml up -d
```

只临时改端口：

```bash
BBSGO_WEB_PORT=13000 ./deploy.sh
```

打开：`http://<服务器IP>:<BBSGO_WEB_PORT>`。

> 本机 3000 被 gitea 占用时，当前实例就是用 `BBSGO_WEB_PORT=13000` 启动的。

### 3.4 安装向导

首次打开会进入 `/install`：

1. **数据库**：
   - Docker 内置 MySQL：`BBSGO_INSTALL_DOCKER_BUILTIN_MYSQL=true` 时，数据库地址/账号由环境变量自动注入，页面会显示“Docker 内置 MySQL”，直接下一步即可。
   - 外部 MySQL：不要设置上述环境变量；在页面填写 host/port/database/username/password。数据库必须是**空库**。
   - SQLite：仅原生模式；安装时选择 SQLite，文件落在配置目录。
2. **站点信息**：站点名称、描述、**站点 URL**。站点 URL 必须是对外访问地址（例如 `http://192.168.1.10:13000`），会影响邮件链接、sitemap、绝对 URL。
3. **管理员**：设置第一个管理员用户名/密码/头像。
4. 安装完成后写入 `bbs-go.yaml` 的 `installed: true`，自动进入站点/后台。

安装完成后：

- 前台：`http://<host>:<port>/`
- 后台：`http://<host>:<port>/dashboard`
- 健康检查：`http://<host>:<port>/api/install/status`

---

## 4. 配置说明

### 4.1 `bbs-go.yaml`

配置文件位于运行目录；Docker 中固定为 `/app/data/bbs-go.yaml`（挂载到宿主机 `docker-data/data/bbs-go.yaml`）。

关键项：

```yaml
language: zh-CN
port: 8082
allowedOrigins:
  - "*"
installed: true

idCodec:
  key: 1234567890   # 安装时生成，不要修改

logger:
  filename: /app/logs/bbs-go.log
  maxSize: 100
  maxAge: 10
  maxBackups: 10

db:
  type: mysql       # mysql | sqlite
  url: ""           # Docker 内置 MySQL 模式下由环境变量注入，保持为空
  maxIdleConns: 50
  maxOpenConns: 200

search:
  indexPath: /app/data/topic_index
```

注意：

- `idCodec.key` 一旦安装后**不要修改**，否则历史的公开 ID 链接会失效。
- Docker 内置 MySQL 模式下，`db.url` 故意持久化为空，实际连接串由 `BBSGO_DOCKER_BUILTIN_MYSQL_*` 环境变量在启动时注入；不要手工填死，否则切换环境会连错库。
- `port` 是 Go API 端口（默认 8082），不是外网访问端口；外网入口是 Node 的 3000。
- `allowedOrigins` 默认 `*` 仅适合内网/测试；生产建议按实际域名收紧。

### 4.2 后台配置（存数据库，不在 YAML）

登录后台 → 系统设置：

- 站点标题/Logo/描述/URL/关键词/公告；
- 登录方式：密码、短信、微信、Google、GitHub、开放注册开关；
- SMTP（邮件通知；前端“忘记密码”入口已移除，SMTP 仍用于通知等邮件能力）；
- 附件上传方式：本地、阿里云 OSS、腾讯云 COS、AWS S3；
- 发帖验证码、邮箱验证、审核、违禁词等；
- 分类类型（普通分类 / 问答分类）、角色权限。

这些设置保存在 `t_sys_config`，修改后立即生效并随数据库备份。

### 4.3 容器环境变量

| 变量 | 作用 | 默认/示例 |
|---|---|---|
| `BBSGO_ENV` | 运行环境 | `prod` |
| `PORT` | Node SSR 端口 | `3000` |
| `BBSGO_SERVER_URL` | Node 代理到的 Go API 地址 | `http://127.0.0.1:8082` |
| `BBSGO_INSTALL_DOCKER_BUILTIN_MYSQL` | 启用 Docker 内置 MySQL 自动配置 | `true` |
| `BBSGO_DOCKER_BUILTIN_MYSQL_HOST` | 内置 MySQL 主机 | `mysql` |
| `BBSGO_DOCKER_BUILTIN_MYSQL_PORT` | 内置 MySQL 端口 | `3306` |
| `BBSGO_DOCKER_BUILTIN_MYSQL_DATABASE` | 数据库名 | `bbsgo` |
| `BBSGO_DOCKER_BUILTIN_MYSQL_USERNAME` | 数据库用户 | `bbsgo` |
| `BBSGO_DOCKER_BUILTIN_MYSQL_PASSWORD` | 数据库密码 | 必须替换默认值 |
| `BBSGO_WEB_PORT` | 离线 Compose 映射到宿主机的端口 | `3000` |
| `TZ` | 时区 | `Asia/Shanghai` |

---

## 5. 数据、备份与恢复

### 5.1 数据目录

**离线 Docker（当前使用）**：

```text
deploy/docker-offline/docker-data/
├── mysql/                 MySQL 8.4 数据目录
├── data/
│   ├── bbs-go.yaml        运行配置
│   └── topic_index/       Bleve 搜索索引
├── logs/                  bbs-go.log
└── uploads/               本地附件/图片
```

**根目录 Compose**：

- `./docker-data/data`、`./docker-data/logs`、`./docker-data/uploads`；
- MySQL 在命名卷 `mysql-data`，不是普通目录；备份时除 `docker-data` 外还需单独导出该卷（停服务后用临时容器打包，或直接用 `docker compose exec mysql mysqldump`）。

**原生二进制**：

- `bbs-go.yaml`、SQLite 模式下的 `bbs-go.db`；
- `data/topic_index/`、`logs/`、`res/uploads/`。

### 5.2 备份

离线 Docker（推荐整目录、停服务保证一致性）：

```bash
docker compose -f docker-compose.yaml stop
tar czf bbs-go-backup-$(date +%F-%H%M).tar.gz docker-data
docker compose -f docker-compose.yaml start
```

原生：

```bash
# 停止 bbs-go 后
tar czf bbs-go-backup-$(date +%F).tar.gz \
  bbs-go.yaml bbs-go.db data logs res/uploads
```

搜索索引可以备份，也可以不备份；丢失后可在后台“系统设置 → 搜索索引”重建。

### 5.3 恢复

1. 停止应用（以及 MySQL）；
2. 将备份的 `docker-data`/数据目录放回原路径；
3. 保持目录属主和权限可写；
4. 启动：
   ```bash
   docker compose -f docker-compose.yaml up -d
   ```
5. 检查 `/api/install/status` 与页面。

### 5.4 升级

```bash
# 1. 先备份
docker compose -f docker-compose.yaml stop
tar czf bbs-go-before-upgrade.tar.gz docker-data
docker compose -f docker-compose.yaml start

# 2. 加载新镜像（离线包）
docker load -i images/bbs-go-offline.tar

# 3. 重建应用容器，保留数据目录
docker compose -f docker-compose.yaml up -d --force-recreate bbs-go

# 4. 观察日志与健康状态
docker compose -f docker-compose.yaml ps
docker compose -f docker-compose.yaml logs -f bbs-go
```

数据库迁移在应用启动时自动执行（`internal/install.InitMigrations`），当前迁移包含分类、注册开关、评论计数重算等。迁移是向前的，没有自动回滚；回滚需要恢复旧镜像 + 升级前数据库备份。

---

## 6. 反向代理与 HTTPS

生产建议在 3000 前放 Nginx/Caddy/负载均衡：

```nginx
server {
    listen 80;
    server_name bbs.example.com;

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

要点：

- 只暴露 3000；不要暴露 8082 和 3306。
- 后台“站点 URL”改成 HTTPS 公网地址，否则邮件/ sitemap 绝对链接仍指向内网。
- 若使用外部 CDN/WAF，确保 `/api`、`/res`、`/sitemap.xml` 正常透传。
- Node SSR 本身不处理 TLS；证书放在反向代理。

---

## 7. 验证与排障

### 7.1 启动后检查

```bash
docker compose ps
curl -fsS http://127.0.0.1:3000/api/install/status
```

正常示例：

```json
{"success":true,"data":{"dbType":"mysql","dockerBuiltinMysql":true,"installed":true}}
```

### 7.2 常见问题

| 现象 | 排查 |
|---|---|
| 页面打不开，容器没起来 | 宿主端口被占用（如 3000 被 gitea 占用）；用 `BBSGO_WEB_PORT=13000` 或改端口；`docker compose logs` |
| 502 / 前端起来但 API 不通 | Go 进程未启动或 `BBSGO_SERVER_URL` 写错；容器内应为 `http://127.0.0.1:8082` |
| 安装页提示数据库非空 | 换空库，或清空目标库；不要复用旧库直接安装 |
| 修改 `.env` 数据库密码后连不上 | MySQL 数据卷已初始化，旧密码仍生效；用原密码，或重建数据卷（会丢数据，先备份） |
| 上传图片 404 | 检查 `docker-data/uploads` 是否挂载到 `/app/res/uploads`，目录权限是否可写 |
| 搜索无结果 | 后台“系统设置 → 搜索索引”重建；检查 `data/topic_index` 可写 |
| 邮件发不出 | 未配置 SMTP；后台通知设置填写 SMTP 后测试；当前默认 SMTP 为空 |
| 登录后跳转/邮件链接域名不对 | 后台“站点 URL”配置错误 |
| 容器反复重启 | 看 `docker compose logs bbs-go`；常见为数据库连接、挂载权限、配置文件损坏 |

### 7.3 日志

```bash
docker compose logs -f bbs-go
tail -f docker-data/logs/bbs-go.log
```

日志按 `maxSize`/`maxAge`/`maxBackups` 轮转。

---

## 8. 安全建议

1. **首次启动前**修改 Compose 中的 MySQL 默认密码，不要使用示例值。
2. 只公网暴露 3000（HTTPS 反向代理）；8082/3306 仅容器网络。
3. 安装后立即修改管理员密码；不要保留测试账号。
4. `allowedOrigins` 从 `*` 收紧为实际域名。
5. 定期备份 `docker-data`（含数据库、配置、附件）；备份文件含密钥，妥善保存。
6. SMTP 授权码、OAuth Secret 存在数据库/后台设置中，不要写进公开文档或镜像。
7. 若使用外部 MySQL，开启 TLS/限制来源 IP，并单独备份。

---

## 9. 本机当前实例（供核对，不是通用参数）

当前开发机同时跑了两套：

| 实例 | 形态 | 地址 | 数据库 | 数据位置 |
|---|---|---|---|---|
| 本地开发 | 原生二进制 + 内嵌 SPA | `http://127.0.0.1:8082` | SQLite | 仓库根 `bbs-go.db`、`data/`、`res/uploads/` |
| Docker | 离线 Compose | `http://127.0.0.1:13000` | MySQL 8.4 | `deploy/docker-offline/docker-data/` |

Docker 实例的宿主端口是 `13000 -> 容器 3000`（因为本机 3000 被 gitea 占用）；容器内 Go 仍是 8082，MySQL 仍是 3306。

当前 Docker 配置：

```bash
cd deploy/docker-offline
BBSGO_WEB_PORT=13000 docker compose -p bbsgo-local up -d --force-recreate bbs-go
```

---

## 10. 明确不包含

- Kubernetes/Helm、多副本水平扩展；
- MySQL 主从/云数据库高可用；
- 对象存储桶、SMTP 账号、OAuth 应用的申请与开通；
- Nginx/Caddy 证书申请与自动续期；
- 多节点共享上传目录/搜索索引的方案。

当前应用是单节点设计；搜索索引和本地附件都落本机磁盘。需要多副本时，至少要把数据库、上传存储、搜索索引外置并单独设计，不能直接复制容器。
