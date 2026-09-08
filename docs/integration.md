# Rosemary VirSree：从零接入指南

本文面向第一次接触 Rosemary VirSree 的应用开发者。完成后，应用只保存一组虚拟 AK/SK；真实存储凭据、物理位置、对象键和配额策略都由 Rosemary 管理。S3 文件直达真实存储或兼容 CDN；WebDAV 因没有通用预签名协议而通过短期能力地址中转。

## 1. 先分清部署实例和开源网站

Rosemary VirSree 是开源软件，任何组织都可以部署自己的实例。接入时会遇到两个不同的网站：

| 网站 | 示例 | 用途 |
|---|---|---|
| 你的 Rosemary 部署实例 | `https://storage.example.com` | 应用实际连接的 API、虚拟 S3、健康检查和实例文档 |
| 公共开源项目 | `https://github.com/Foodie05/rosemary-virsree` | 源码、Issue、通用文档和 Release 下载 |

应用的 `AWS_ENDPOINT_URL` 必须指向部署实例的 `/s3`，绝不能指向 GitHub。实例管理员可配置自己信任的源码和 Release 地址；用下面的公开接口核对：

```bash
curl -fsS https://storage.example.com/api/v1/meta
```

响应中的 `gateway_url` 是应用地址，`project_url` 是源码地址，`release_url` 是 CLI 下载来源。后续示例分别保存它们：

```bash
export RVS_GATEWAY=https://storage.example.com
export RVS_RELEASES=https://github.com/Foodie05/rosemary-virsree/releases
```

## 2. 理解实例内的三个地址

假设平台地址是 `https://storage.example.com`：

| 地址 | 用途 | 是否传输文件正文 |
|---|---|---|
| `https://storage.example.com/api/v1` | 创建桶、申请上传/下载签名、commit、公开链接和运维 | 否，只传 JSON |
| `https://storage.example.com/s3` | ListObjectsV2、HeadObject、GetObject、DeleteObject 的虚拟 S3 入口 | GET 会 307 到真实 S3 |
| 响应中的短期 `url` | 上传或下载文件 | `direct=true` 直连 S3/CDN；`false` 经 WebDAV 中转 |

Rosemary 的 `/s3` 不接受 `PutObject`。上传前必须先取得真实 S3 预签名 URL。

## 3. 准备信息

接入前确认：

- 应用名称和一个 3–63 位虚拟桶名，例如 `invoice-service-prod`。
- 所需最大空间，按字节提交。
- 所需权限：`read`、`write`、`delete`、`manage`。
- Secret 保存位置。生产环境优先使用 Kubernetes Secret、Vault、AWS Secrets Manager 等。
- 每类业务链接需要多长时间。平台没有统一默认时长。

## 4. 检查平台

```bash
curl -fsS "$RVS_GATEWAY/health"
```

正常响应：

```json
{"status":"ok","backend_ready":true}
```

`backend_ready=false` 表示管理面在线，但还没有可用存储源，不能签发或提交对象操作。

## 5. 从 Release 下载 rvsctl

平台提供以下组合：

- `darwin/arm64`、`darwin/amd64`
- `linux/arm64`、`linux/amd64`
- `windows/arm64`、`windows/amd64`

macOS / Linux：

```bash
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

curl -fsSLo rvsctl "$RVS_RELEASES/latest/download/rvsctl-$OS-$ARCH"
curl -fsSLo SHA256SUMS "$RVS_RELEASES/latest/download/SHA256SUMS"
chmod 0755 rvsctl
EXPECTED=$(awk -v f="rvsctl-$OS-$ARCH" '$2==f {print $1}' SHA256SUMS)
printf '%s  %s\n' "$EXPECTED" rvsctl | shasum -a 256 -c -
```

Linux 可把最后一条命令中的 `shasum -a 256` 换成 `sha256sum`。若找不到对应条目或校验失败，不要执行该文件。

Windows PowerShell：

```powershell
$Releases = 'https://github.com/Foodie05/rosemary-virsree/releases'
$Arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') {'arm64'} else {'amd64'}
Invoke-WebRequest "$Releases/latest/download/rvsctl-windows-$Arch.exe" -OutFile .\rvsctl.exe
Invoke-WebRequest "$Releases/latest/download/SHA256SUMS" -OutFile .\SHA256SUMS
Get-FileHash .\rvsctl.exe -Algorithm SHA256
```

把哈希与 `SHA256SUMS` 中 `rvsctl-windows-$Arch.exe` 的条目逐字比较。Release 页面只用于下载工具，`rvsctl -endpoint` 仍必须填写部署实例。

## 6. 兑换一次性 Token

管理员在 **Agent 接入** 页面创建一次性 Bootstrap Token。Token 有最大桶空间和自身失效时间，只能成功兑换一次。

```bash
./rvsctl \
  -endpoint "$RVS_GATEWAY" \
  -token "$RVS_BOOTSTRAP_TOKEN" \
  -name "Invoice service production" \
  -bucket invoice-service-prod \
  -quota 10737418240 \
  -visibility private \
  -env-file /run/secrets/rosemary.env
```

成功时终端只输出保存路径。文件内容如下，但工具不会打印具体值：

```dotenv
AWS_ENDPOINT_URL=https://storage.example.com/s3
AWS_REGION=us-east-1
AWS_ACCESS_KEY_ID=RVS...
AWS_SECRET_ACCESS_KEY=rvs_...
AWS_S3_FORCE_PATH_STYLE=true
RVS_BUCKET=invoice-service-prod
```

`rvsctl` 拒绝覆盖已存在文件，避免误删旧凭据。若兑换后部署失败，请保留文件并修复部署；不要尝试重复使用 Token。

## 7. 配置 S3 SDK

SDK 用于列表、HEAD、GET 和 DELETE。必须启用 path-style。

### JavaScript / TypeScript

```ts
import { S3Client, ListObjectsV2Command, HeadObjectCommand } from '@aws-sdk/client-s3';

const s3 = new S3Client({
  endpoint: process.env.AWS_ENDPOINT_URL,
  region: process.env.AWS_REGION,
  forcePathStyle: true,
  credentials: {
    accessKeyId: process.env.AWS_ACCESS_KEY_ID!,
    secretAccessKey: process.env.AWS_SECRET_ACCESS_KEY!,
  },
});

await s3.send(new ListObjectsV2Command({ Bucket: process.env.RVS_BUCKET }));
await s3.send(new HeadObjectCommand({ Bucket: process.env.RVS_BUCKET, Key: 'reports/q3.pdf' }));
```

### Python / boto3

```python
import os, boto3
from botocore.config import Config

s3 = boto3.client(
    "s3",
    endpoint_url=os.environ["AWS_ENDPOINT_URL"],
    region_name=os.environ["AWS_REGION"],
    aws_access_key_id=os.environ["AWS_ACCESS_KEY_ID"],
    aws_secret_access_key=os.environ["AWS_SECRET_ACCESS_KEY"],
    config=Config(signature_version="s3v4", s3={"addressing_style": "path"}),
)
```

### Go

```go
cfg, err := config.LoadDefaultConfig(ctx,
    config.WithRegion(os.Getenv("AWS_REGION")),
    config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
        os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_ACCESS_KEY"), "")),
)
if err != nil { return err }

client := s3.NewFromConfig(cfg, func(o *s3.Options) {
    o.BaseEndpoint = aws.String(os.Getenv("AWS_ENDPOINT_URL"))
    o.UsePathStyle = true
})
```

不要用这些客户端向 Rosemary 调用 `PutObject`。

## 8. 实现上传：签名、直传、commit

### 第一步：向网关申请 URL

```http
POST /api/v1/buckets/invoice-service-prod/objects/upload
X-RVS-Access-Key: <virtual AK>
X-RVS-Secret-Key: <virtual SK>
Content-Type: application/json

{
  "key": "invoices/2026/INV-1001.pdf",
  "size": 284190,
  "content_type": "application/pdf",
  "expires_in": 600
}
```

`size` 必须是实际字节数。`expires_in` 由当前业务选择且必须为正整数；S3 SigV4 的协议上限是 604800 秒。响应里的 `direct` 明确说明文件是否绕过 Rosemary。
上传签名会绑定这个 Content-Length。浏览器会依据 `Blob` / `File` 自动发送该请求头；不要尝试在前端 JavaScript 中手工设置受浏览器保护的 `Content-Length`。

响应示例：

```json
{
  "upload_id": "obj_...",
  "method": "PUT",
  "url": "https://real-private-s3.example/...signature...",
  "expected_size": 284190,
  "required_headers": {"Content-Type":"application/pdf"},
  "commit_url": "https://storage.example.com/api/v1/buckets/invoice-service-prod/objects/commit",
  "expires_in": 600
}
```

### 第二步：把正文发给响应 URL

```bash
curl -f -X PUT "$SIGNED_REAL_S3_URL" \
  -H 'Content-Type: application/pdf' \
  --data-binary @INV-1001.pdf
```

必须原样使用 `required_headers`。不要记录、持久化或返回签名 URL。

### 第三步：commit

```http
POST <commit_url>
X-RVS-Access-Key: <virtual AK>
X-RVS-Secret-Key: <virtual SK>
Content-Type: application/json

{"upload_id":"obj_...","key":"invoices/2026/INV-1001.pdf"}
```

Rosemary 会向 S3 执行 HEAD，核对大小，再将预留空间记为已使用。业务记录只能在 commit 成功后标记上传完成。

## 9. 实现下载

```http
POST /api/v1/buckets/invoice-service-prod/objects/download
X-RVS-Access-Key: <virtual AK>
X-RVS-Secret-Key: <virtual SK>
Content-Type: application/json

{"key":"invoices/2026/INV-1001.pdf","filename":"invoice.pdf","expires_in":300}
```

响应中的 `url` 是短期文件地址。`direct=true` 时指向真实 S3 或兼容 CDN，文件不经过 Rosemary；`direct=false` 时对象位于 WebDAV，地址由 Rosemary 中转。

## 10. 私有链接与“公开”链接

- 普通下载 URL：一次签发，直到所选时长结束。撤销虚拟 AK/SK 不会让它提前失效。
- `/p/{slug}`：稳定的 Rosemary 地址。每次访问由平台重新签发真实 URL 并 307 跳转。
- 创建公开别名时，应用必须明确指定 `sign_expires_in`。可用 `link_expires_in` 控制别名本身的寿命；`0` 表示不设置别名到期时间。
- 保存创建响应中的 `slug`。不再需要别名时，用 `DELETE /api/v1/buckets/{bucket}/public-links/{slug}` 立即停止后续跳转。
- 底层 S3 桶始终保持 Private。

## 11. 权限

| 权限 | 可以执行 |
|---|---|
| `read` | 列表、HEAD、申请下载 URL |
| `write` | 申请上传 URL、commit |
| `delete` | 删除对象 |
| `manage` | 创建公开别名、物理键轮换使旧链接失效 |

权限互不包含。`manage` 不会自动获得 `read`。

## 12. 配额

- 平台总配额限制所有虚拟桶配额之和。
- 每桶配额限制已使用空间加上传预留。
- 申请上传时预留空间；commit 后转为已使用。
- 同名覆盖只预留相对旧对象增长的部分。
- 上传 URL 到期后遗留的预留会在后续申请上传时清理。
- 每桶对象数和同时待提交上传数还有独立上限，避免零字节对象绕过字节配额。

## 13. 验证清单

1. `/health` 返回 `backend_ready=true`。
2. rvsctl 输出中没有 AK/SK，Secret 文件权限是 0600。
3. ListObjectsV2、HeadObject 工作。
4. 对 `/s3/{bucket}/{key}` 直接 PUT 返回 405。
5. 上传签名响应 URL 的主机是 S3，不是 Rosemary。
6. PUT、commit、签名下载均成功，下载最终主机是 S3。
7. 使用不具备 write 权限的 Key 申请上传返回 403。
8. 日志和 Git diff 中没有凭据或签名 URL。

## 14. 常见故障

| 状态/现象 | 原因 | 处理 |
|---|---|---|
| `401 admin authentication required` | 管理令牌错误 | 仅管理 API 使用 Bearer 管理令牌 |
| `403 invalid credentials` | 虚拟 AK/SK 错误或已撤销 | 检查 Secret 注入和目标环境 |
| `403 permission denied` | Key 缺少所需权限 | 由管理员签发最小权限的新 Key |
| `400 expires_in is required` | 应用未选择签名时长 | 当前调用显式提交秒数 |
| `400 ...604800` | 超过 SigV4 上限 | 缩短到 604800 秒以内 |
| `400 bucket quota exceeded` | 桶已用空间和预留不足 | 清理对象或调整配额 |
| commit 大小不匹配 | 声明 size 与真实对象不同 | 使用准确字节数重新申请上传 |
| 浏览器 S3 请求被 CORS 拒绝 | 真实桶 CORS 未允许应用域名 | 配置真实桶的 GET/HEAD/PUT CORS |
| 直接 PutObject 返回 405 | 正确的安全边界 | 改用签名上传三步流程 |

## 15. 真实桶运维要求

Rosemary 的正式对象使用 `rosemary/` 前缀，上传临时对象使用 `rosemary-staging/` 前缀。由于 S3 预签名 PUT 在协议上不能提前撤销，已经 commit 的上传 URL 仍可能在到期前重写其临时键，但不会影响正式对象，且 Content-Length 已绑定。请在真实桶上为 `rosemary-staging/` 配置 **8 天后删除** 的生命周期规则，以清理 URL 被复用或客户端中断产生的临时对象；8 天覆盖 SigV4 最长 7 天有效期。

完整字段定义见 [api.md](./api.md)，设计边界见 [architecture.md](./architecture.md)。
