# Sub2API 流量伪装功能部署手册

**变更内容：** Codex CLI 指纹自动跟版 + 分组级流量伪装开关
**部署日期：** 2026-06-20
**目标镜像：** `sub2api:traffic-shaping-2026-06-20`
**回滚镜像：** `weishaw/sub2api:latest`（你当前正在跑的）

---

## 0. 部署前必做：备份（5 分钟）

```bash
# 在新加坡服务器上跑
mkdir -p ~/sub2api-backup-$(date +%Y%m%d-%H%M)
cd ~/sub2api-backup-$(date +%Y%m%d-%H%M)

# === 备份数据库（必做） ===
sudo docker exec sub2api-postgres pg_dump -U postgres sub2api | gzip > db.sql.gz

# === 备份 docker-compose 配置 ===
sudo cp /opt/sub2api/docker-compose.yml ./docker-compose.yml.backup
sudo cp /opt/sub2api/.env ./env.backup 2>/dev/null || true

# === 备份当前镜像（万一 docker pull 拉不回来） ===
sudo docker save weishaw/sub2api:latest | gzip > image-original.tar.gz

# 验证
ls -lh
# 应该看到 db.sql.gz, docker-compose.yml.backup, image-original.tar.gz
```

⚠️ 必须看到 `db.sql.gz` 大小不为 0 再继续。

---

## 1. 上传新镜像到服务器

我已经在 Windows build 出了新镜像并保存到 `E:\Temp\sub2api-traffic-shaping.tar.gz`。

```powershell
# === Windows 本地：上传镜像到服务器 ===
scp E:\Temp\sub2api-traffic-shaping.tar.gz <你的用户名>@<服务器IP>:/tmp/

# 镜像约 200-300 MB，按你网速 1-5 分钟
```

```bash
# === 服务器：导入镜像 ===
sudo docker load < /tmp/sub2api-traffic-shaping.tar.gz

# 验证镜像存在
sudo docker images | grep traffic-shaping
# 应该看到: sub2api   traffic-shaping-2026-06-20  ...
```

---

## 2. 修改 docker-compose.yml

```bash
sudo nano /opt/sub2api/docker-compose.yml
# 找到 sub2api 服务下的 image: 那行
# 改成: image: sub2api:traffic-shaping-2026-06-20
# 其他不动
```

---

## 3. 重启 sub2api 容器（中断 ~10 秒）

```bash
cd /opt/sub2api

# 仅重启 sub2api 容器，PG 和 Redis 保持运行
sudo docker compose up -d sub2api

# 看启动日志（关键）
sudo docker logs -f sub2api
```

**重点观察日志中**：
1. ✅ 看到 `migration 154` 相关日志 → 数据库迁移成功跑了
2. ✅ 看到 `Server started on :8080` → 启动成功
3. ✅ 看到 `codex_version: refreshed from GitHub version=0.141.0` → 自动跟版工作了
4. ❌ 看到 `panic`、`fatal`、`migration failed` → **立即回滚**（步骤 6）

按 Ctrl+C 退出日志查看。

---

## 4. 验证功能

### 4.1 健康检查
```bash
curl -i http://127.0.0.1:8080/health
# 期望: HTTP/1.1 200 OK
```

### 4.2 验证当前对话还能工作
你当前跟我对话用的就是这个 sub2api。如果重启后我还能正常回你 —— 重启成功。

### 4.3 进后台 UI 验证
打开你的 sub2api 后台（同样的 URL），登录管理员账号：

- **网关设置** → 应该看到「Codex CLI 指纹版本」卡片，版本显示 `0.141.0`
- **分组管理** → 编辑任意分组 → 应该看到「流量伪装」高级开关（默认关闭）

### 4.4 数据库迁移验证
```bash
sudo docker exec sub2api-postgres psql -U postgres -d sub2api -c "
  SELECT column_name, data_type, column_default
  FROM information_schema.columns
  WHERE table_name = 'groups'
    AND column_name IN ('traffic_shaping_enabled', 'sse_early_keepalive_enabled', 'stream_smoothing_enabled', 'silent_retry_5xx_enabled');
"
```

期望输出 4 行，每行 `boolean`、默认 `false`。

---

## 5. 启用流量伪装（你的目标）

后台 → 分组管理 → 找到对外免费的那个分组 → 编辑

1. 滑到「流量伪装」卡片
2. 打开**总开关**（变成琥珀色）
3. 打开「SSE 极速首字节」子开关
4. 「流式响应平滑」（即将推出，目前点不动）
5. 「上游 5xx 静默重试」标记为「已自动启用」（sub2api 默认 failover 已覆盖）
6. 保存

**自用分组保持总开关关闭**，行为完全不变。

---

## 6. 回滚（如果出问题）

```bash
# === 改回旧 image tag ===
sudo nano /opt/sub2api/docker-compose.yml
# image: 那行改回: weishaw/sub2api:latest

# === 重启 ===
cd /opt/sub2api
sudo docker compose up -d sub2api

# === 看日志确认回滚成功 ===
sudo docker logs -f sub2api
```

**关于 154 迁移：** 即使回滚到旧版本，新增的 4 列留在 `groups` 表里**完全无影响** —— 旧版本不查这些列就不知道它们存在。无需手动撤销迁移。

---

## 7. 部署成功后的清理（可选）

观察 24 小时无问题后：

```bash
# 删除旧镜像（释放磁盘）
sudo docker image prune -a --filter "until=720h"  # 30 天前的

# 但留一份备份镜像 tar 包，万一以后要回滚
ls ~/sub2api-backup-*
```

---

## 8. 监控指标

部署后值得观察：

| 指标 | 预期变化 | 怎么看 |
|---|---|---|
| sub2api 内存占用 | 几乎不变 | `docker stats sub2api` |
| sub2api 启动时间 | <10s | 日志 |
| GitHub API 命中率 | 每 6h 一次 | 日志中 `codex_version: refreshed from GitHub` |
| axonhub 上你的 channel TTFT（启用流量伪装后） | 从几百 ms 降到 ~100-200ms | axonhub 后台 |
| axonhub 上你的 channel 排名（启用后） | 应该上升 | axonhub 后台 |

---

## 故障排查

### 问题 1：迁移失败（容器启动后立即退出）
```bash
sudo docker logs sub2api | grep -i migration
# 看具体错误
```
通常是 154 SQL 语法问题。回滚 + 把日志贴给我。

### 问题 2：UI 打不开 / 后台白屏
前端没嵌入。检查 build 时是否带了 `-tags embed`。回滚 + 把日志贴给我。

### 问题 3：API 调用全 503
看 sub2api 容器是否在 `Up (healthy)` 状态：
```bash
sudo docker ps | grep sub2api
```
如果 `unhealthy`，看日志找原因。回滚是安全的兜底。

### 问题 4：数据库连不上
PG 容器没问题就不会出现。检查：
```bash
sudo docker ps | grep postgres
# 确认 sub2api-postgres 是 Up (healthy)
```
