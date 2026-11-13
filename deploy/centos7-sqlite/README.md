# bbs-go CentOS 7 离线包（SQLite）

## 快速开始

```bash
sha256sum -c SHA256SUMS
chmod +x deploy.sh load-images.sh install-docker.sh
./deploy.sh
```

浏览器打开：`http://<服务器IP>:8082/`

首次打开进入 `/install`，数据库类型默认 SQLite，填写站点信息和管理员账号即可。

## 目录

- `images/`：应用镜像
- `docker-runtime/`：可选 Docker 20.10.24 + Compose 插件
- `docker-data/`：运行后生成，包含 SQLite 数据库、配置、索引、日志、附件
- `manual/部署手册.md` / `部署手册.pdf`：完整部署说明

## 常用命令

```bash
docker compose -f docker-compose.yaml ps
docker compose -f docker-compose.yaml logs -f bbs-go
docker compose -f docker-compose.yaml restart bbs-go
```

详细步骤见 `manual/部署手册.pdf`。
