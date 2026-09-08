# Rosemary VirSree 管理员部署与首次配置

这份文档说明管理台登录、OOBE、多存储源和 CDN 下载。应用接入请看 [integration.md](./integration.md)。

## 1. OIDC 登录参数

Rosemary 使用服务端 OIDC Authorization Code + PKCE 流程。生产环境必须准备：

- Issuer：`https://apiauth.cruty.cn`
- Redirect URI：`https://storage.cruty.cn/auth/callback`
- Scope：`openid profile email`
- 一组 confidential client ID / client secret
- 至少一个允许登录的完整邮箱地址

必须先在身份服务中注册完全一致的 Redirect URI。登录开始时 Rosemary 生成一次性的 `state`、`nonce` 和 PKCE verifier；回调时通过 Discovery 与 JWKS 校验 ID Token 的 RS256 签名、issuer、audience、时间声明、nonce，并要求 ID Token 与 UserInfo 的 subject 一致。随后从 UserInfo 读取邮箱，与 `RVS_ADMIN_EMAILS` 做不区分大小写的完整匹配。浏览器只保存 Secure、HttpOnly、SameSite=Lax 会话 Cookie，站点不接触用户密码。

仓库中的脚本会在本机交互读取 client ID、client secret 和白名单邮箱，再通过 SSH 上传到服务器。服务器首次配置时生成管理 API Token 与数据库加密主密钥；以后重跑会保留它们，避免已加密数据失效。脚本不会打印这些值：

```bash
./deploy/configure-production.sh
```

默认目标是 `cruty.cn`，公开地址是 `https://storage.cruty.cn`。其他部署可通过 `RVS_DEPLOY_HOST` 和 `RVS_PUBLIC_URL` 覆盖。`/etc/rosemary-virsree.env` 和 SQLite 数据库必须一起备份；丢失或更换 `RVS_MASTER_KEY` 后，已加密的存储源凭据与虚拟 SK 无法恢复。

## 2. 首次登录与 OOBE

打开部署域名并使用白名单邮箱登录。空数据库会自动进入三个阶段：

1. **欢迎**：解释调度、凭据保存和验证边界。
2. **连接第一个存储源**：选择 S3 或 WebDAV，填写容量与优先级，并执行真实读写验证。
3. **完成**：验证通过后进入首页。

验证会写入 `rosemary-system/probes/` 下的极小临时对象，并验证上传、HEAD、复制、下载和删除。任一步失败都不会保存存储源配置。生产存储凭据使用 `RVS_MASTER_KEY` 加密后才进入 SQLite，API 永远不返回明文。

## 3. 多存储源调度

OOBE 后可在 **存储源** 页面继续添加来源。每个来源包含：

- 优先级：数字越小越先尝试。
- Rosemary 容量上限：`used + reserved` 达到上限后，新上传自动尝试下一来源。
- 已使用空间：成功 commit 后记账。
- 上传预留：签发上传地址时记账，上传到期后回收。

一个对象一旦 commit，会一直从记录的来源读取、删除和轮换物理键。增加新的高优先级来源不会自动搬迁旧对象。

## 4. S3 存储源

S3 来源需要真实私有桶、Region、AK/SK，以及可选的三类 Endpoint：

| 字段 | 用途 |
|---|---|
| 内部 API Endpoint | Rosemary 执行 HEAD、COPY、DELETE 和 commit 验证 |
| 应用直传 Endpoint | 生成上传签名；留空时使用内部 Endpoint |
| 下载 CDN Endpoint | 只生成下载签名；留空时使用应用直传 Endpoint |

下载 CDN Endpoint 必须兼容 S3 SigV4，并保留签名时使用的 Host、路径和查询参数。它适合带 S3 兼容域名的 OSS/COS/R2/MinIO 网关。CloudFront 私钥签名、阿里云 CDN 鉴权 URL 等供应商专有算法不能直接填入此字段，需要专门的签名适配器。

S3 来源的上传和下载文件正文都不经过 Rosemary。控制请求进入统一网关，响应的 `url` 指向 S3 或配置的兼容 CDN，且 `direct: true`。

真实桶至少需要 `GetObject`、`PutObject`、`DeleteObject` 和服务端 Copy 权限。建议让 `rosemary-staging/` 在 8 天后自动删除，并为浏览器来源配置 GET、HEAD、PUT CORS。

## 5. WebDAV 存储源

WebDAV 需要 Endpoint、用户名和密码或 App Password。标准 WebDAV 没有与 S3 预签名 URL 等价的通用协议，因此 Rosemary 会签发短期能力 URL，并中转 WebDAV 文件正文。应用仍使用同一套“申请上传 URL → PUT → commit”和“申请下载 URL”流程，但响应为 `direct: false`。

部署 WebDAV 时需要按最大文件和并发量规划 Rosemary 与 Apache 的带宽、超时和请求大小。若硬性要求文件永不经过平台，应只配置 S3 来源。

## 6. 登录外的管理 API

浏览器管理台使用 OIDC 会话。自动化可以继续使用 `Authorization: Bearer <RVS_ADMIN_TOKEN>`，该 Token 只保存在服务器环境文件中。存储源管理接口：

```text
GET  /api/v1/setup/status
GET  /api/v1/storage-sources
POST /api/v1/storage-sources
```

`POST` 会同步完成连接与数据面验证，成功时返回 `verified: true`。请勿把真实存储凭据写入日志、工单或源码。

## 7. 上线检查

```bash
curl -fsS https://storage.cruty.cn/health
curl -fsS https://storage.cruty.cn/api/v1/meta
```

在 OOBE 前，健康接口的 `backend_ready` 为 `false`；第一个来源验证保存后应为 `true`。再确认：OIDC 非白名单邮箱被拒绝、S3 探针经直传/CDN 读回、WebDAV 页面明确显示中转、Apache 只代理 `storage.cruty.cn`，以及 Rosemary 只监听 `127.0.0.1:18741`。
