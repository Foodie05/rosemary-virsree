# VirSree by Rosemary 管理员部署与首次配置

这份文档说明管理台登录、OOBE、多存储源和 CDN 下载。应用接入请看 [integration.md](./integration.md)。

## 1. OIDC 登录参数

VirSree 使用服务端 OIDC Authorization Code + PKCE 流程。生产环境必须准备：

- Issuer：`https://apiauth.cruty.cn`
- Redirect URI：`https://storage.cruty.cn/auth/callback`
- Scope：`openid profile email`
- 一组 confidential client ID / client secret
- 至少一个允许登录的完整邮箱地址

必须先在身份服务中注册完全一致的 Redirect URI。登录开始时 VirSree 生成一次性的 `state`、`nonce` 和 PKCE verifier；回调时通过 Discovery 与 JWKS 校验 ID Token 的 RS256 签名、issuer、audience、时间声明、nonce，并要求 ID Token 与 UserInfo 的 subject 一致。随后从 UserInfo 读取邮箱，与 `RVS_ADMIN_EMAILS` 做不区分大小写的完整匹配。浏览器只保存 Secure、HttpOnly、SameSite=Lax 会话 Cookie，站点不接触用户密码。

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

验证会写入 `virsree-system/probes/` 下的极小临时对象，并验证上传、HEAD、复制、下载和删除。任一步失败都不会保存存储源配置。S3 探针还会通过接受上传的同一 Endpoint 签发清理请求，避免地址配置错误时留下探针。生产存储凭据使用 `RVS_MASTER_KEY` 加密后才进入 SQLite，API 永远不返回明文。

## 3. 多存储源调度

OOBE 后可在 **存储源** 页面继续添加来源。每个来源包含：

- 优先级：数字越小越先尝试。
- VirSree 容量上限：`used + reserved` 达到上限后，新上传自动尝试下一来源。
- 已使用空间：成功 commit 后记账。
- 上传预留：签发上传地址时记账，上传到期后回收。

一个对象一旦 commit，会一直从记录的来源读取、删除和轮换物理键。增加新的高优先级来源不会自动搬迁旧对象。

## 4. S3 存储源

S3 来源需要真实私有桶、Region、AK/SK，以及可选的三类 Endpoint：

| 字段 | 用途 |
|---|---|
| 内部 API Endpoint | VirSree 执行 HEAD、COPY、DELETE 和 commit 验证 |
| 应用直传 Endpoint | 生成上传签名；留空时使用内部 Endpoint |
| 下载 CDN 加速域名 | 只生成下载链接；留空时使用应用直传 Endpoint |

填写下载 CDN 后必须选择与服务端一致的鉴权方式：

- **缤纷云高级鉴权**：适用于启用了高级鉴权的缤纷云 CDN 项目。再填写该项目的“鉴权 Key”；VirSree 按应用每次提交的 `expires_in` 生成 `_ts`，并在服务端计算 `_btf_tk`。Key 与存储凭据一同加密保存，API 和日志不回显。
- **S3 SigV4 兼容**：只适用于明确接受 AWS SigV4 预签名 GET 的下载 Endpoint。普通 CDN 自定义域名通常不属于这一类。

缤纷云 CDN 加速域名若启用了高级鉴权，直接访问返回 403 是预期行为。把它误选成 S3 SigV4 会因签名协议不同而验证失败。CloudFront 私钥签名、阿里云 CDN 鉴权 URL 等其他供应商算法仍需要相应的签名适配器。

S3 来源的上传和下载文件正文都不经过 VirSree。控制请求进入统一网关，响应的 `url` 指向 S3 或配置的兼容 CDN，且 `direct: true`。

真实桶至少需要 `GetObject`、`PutObject`、`DeleteObject` 和服务端 Copy 权限。建议让 `rosemary-staging/` 在 8 天后自动删除，并为浏览器来源配置 GET、HEAD、PUT CORS。

## 5. WebDAV 存储源

WebDAV 需要 Endpoint、用户名和密码或 App Password。标准 WebDAV 没有与 S3 预签名 URL 等价的通用协议，因此 VirSree 会签发短期能力 URL，并中转 WebDAV 文件正文。应用仍使用同一套“申请上传 URL → PUT → commit”和“申请下载 URL”流程，但响应为 `direct: false`。

部署 WebDAV 时需要按最大文件和并发量规划 VirSree 与 Apache 的带宽、超时和请求大小。若硬性要求文件永不经过平台，应只配置 S3 来源。

## 6. 登录外的管理 API

浏览器管理台使用 OIDC 会话。自动化可以继续使用 `Authorization: Bearer <RVS_ADMIN_TOKEN>`，该 Token 只保存在服务器环境文件中。存储源管理接口：

```text
GET  /api/v1/setup/status
GET  /api/v1/storage-sources
POST /api/v1/storage-sources
GET  /api/v1/storage-sources/{id}
PUT  /api/v1/storage-sources/{id}
```

`POST` 和 `PUT` 都会同步完成连接与完整数据面验证，成功时返回 `verified: true`。编辑时凭据字段留空会继续使用原先加密保存的凭据；只有验证成功后才会一次性替换配置，失败时现有来源继续运行。

更换 S3 的真实桶可能让尚未迁移的现有对象无法下载、删除或轮换链接。管理台会使用统一的 VirSree 风险弹窗说明影响，确认后才提交 `acknowledge_bucket_change: true`；API 客户端也必须显式提交该字段。VirSree 不会自动迁移对象。请勿把真实存储凭据写入日志、工单或源码。

## 7. 上线检查

```bash
curl -fsS https://storage.cruty.cn/health
curl -fsS https://storage.cruty.cn/api/v1/meta
```

在 OOBE 前，健康接口的 `backend_ready` 为 `false`；第一个来源验证保存后应为 `true`。再确认：OIDC 非白名单邮箱被拒绝、S3 探针经直传/CDN 读回、WebDAV 页面明确显示中转、Apache 只代理 `storage.cruty.cn`，以及 VirSree 只监听 `127.0.0.1:18741`。

管理台和网关的版本应保持一致。`GET /api/v1/version` 返回 Release 构建写入的版本与提交号，且必须保留 VirSree 返回的 `Cache-Control: no-store`。入口 HTML 同样不可缓存；文件名带内容哈希的 `/assets/` 资源可以按 `public, max-age=31536000, immutable` 缓存。管理台发现版本不一致时，会通过带版本号和时间戳的 URL 自动重新载入。

## 8. 使用追踪编号安全排错

每个 API 错误都包含 `trace_id`，响应头 `X-Request-ID` 也会返回同一个值。无需向用户索取凭据或签名链接，直接查找对应请求：

```bash
sudo journalctl -u rosemary-virsree --since today | grep 'req_example'
```

请求日志只记录追踪编号、HTTP 方法、注册路由模板、状态码、耗时和稳定错误代码。存储验证失败时会额外记录存储类型、探针阶段、CDN 鉴权模式、Path-style 选项，以及使用服务器主密钥对 Endpoint 主机名计算的短 HMAC-SHA-256 标识。VirSree 不记录查询字符串、真实桶名和对象名、原始 Endpoint 主机名、请求或响应正文、CDN 鉴权 Key、AK/SK、OIDC Token 或签名 URL。主机名标识只用于在同一实例内判断两次尝试是否使用同一地址，不能跨实例关联或离线枚举常见域名。
