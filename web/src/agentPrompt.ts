type PromptInput={gateway:string;token:string;quotaGB:number;expiresAt?:string;projectURL:string;releaseURL:string};

export function buildAgentPrompt(v:PromptInput){return `你负责把当前陌生应用完整接入 VirSree by Rosemary 对象存储。不要只给建议；请检查项目、实现适配、运行验证，并在遇到无法自动判断的业务选择时才询问我。

【本次接入参数】
- VirSree 实例（所有存储 API 和 S3 endpoint 都配置到这里）：${v.gateway}
- 开源项目（只用于源码、Issue 和通用说明，不能作为存储 endpoint）：${v.projectURL}
- 官方 CLI Releases（只用于下载 rvsctl）：${v.releaseURL}
- 一次性 Bootstrap Token：${v.token}
- 最大可申请空间：${v.quotaGB} GB
- Token 失效时间：${v.expiresAt||'以平台响应为准'}

Bootstrap Token 只能兑换一次。禁止把 Token、AK、SK、凭据文件内容或任何预签名 URL 输出到对话、日志、提交记录和测试快照中。

一、先检查应用，不要立即改代码
1. 确认语言、框架、包管理器、运行方式和部署环境。
2. 搜索现有文件上传、下载、本地磁盘、S3 SDK、对象 URL 和环境变量使用位置。
3. 判断应用需要哪些权限：read、write、delete、manage。不要扩大权限。
4. 选择 3–63 位、全小写、只含字母数字和连字符的虚拟桶名。
5. 确定生产 Secret 的写入位置。优先 Kubernetes Secret、Vault 或云 Secret Manager；本地开发才使用 0600 env 文件。

二、检查网关并阅读平台原始文档
先请求：
  curl -fsS ${v.gateway}/health

随后完整阅读：
- ${v.gateway}/docs/integration.md  （从零接入、语言示例、验证和排错）
- ${v.gateway}/docs/api.md          （所有请求字段、响应和权限）
- ${v.gateway}/docs/architecture.md （数据路径、配额、链接失效边界）

如果 health 或文档不可访问，停止兑换 Token，报告具体 URL 和错误。

三、从本项目 GitHub Releases 下载并校验 rvsctl
根据部署机器选择 os=darwin|linux|windows，arch=amd64|arm64。不要运行来源不明的同名工具。

macOS / Linux：
  OS=$(uname -s | tr '[:upper:]' '[:lower:]')
  ARCH=$(uname -m); [ "$ARCH" = "x86_64" ] && ARCH=amd64; [ "$ARCH" = "aarch64" ] && ARCH=arm64; [ "$ARCH" = "arm64" ] && ARCH=arm64
  curl -fsSLo ./rvsctl "${v.releaseURL}/latest/download/rvsctl-$OS-$ARCH"
  curl -fsSLo ./SHA256SUMS "${v.releaseURL}/latest/download/SHA256SUMS"
  chmod 0755 ./rvsctl
  # 在 SHA256SUMS 中找到 rvsctl-$OS-$ARCH 条目并校验 SHA-256。

Windows PowerShell：
  $Arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') {'arm64'} else {'amd64'}
  Invoke-WebRequest "${v.releaseURL}/latest/download/rvsctl-windows-$Arch.exe" -OutFile .\\rvsctl.exe
  Invoke-WebRequest "${v.releaseURL}/latest/download/SHA256SUMS" -OutFile .\\SHA256SUMS
  # 用 Get-FileHash .\\rvsctl.exe -Algorithm SHA256 对照对应条目。

若对应平台返回 404，不要自行从第三方下载；报告缺失的 os/arch。

四、只兑换一次并安全落盘
先确认目标文件不存在且不会进入 Git。将下面占位符替换为实际值：

  ./rvsctl \\
    -endpoint "${v.gateway}" \\
    -token '<上面的 Bootstrap Token>' \\
    -name '<应用名称>' \\
    -bucket '<虚拟桶名>' \\
    -quota '<字节数，不得超过 ${v.quotaGB} GB>' \\
    -visibility private \\
    -env-file '<部署 Secret 路径>'

rvsctl 会创建虚拟桶并把 AWS_ENDPOINT_URL、AWS_REGION、AWS_ACCESS_KEY_ID、AWS_SECRET_ACCESS_KEY、AWS_S3_FORCE_PATH_STYLE 和 RVS_BUCKET 写入权限为 0600 的文件。它不会在终端显示 AK/SK。不要读取或打印该文件；只检查文件存在、权限正确、应用进程能够加载。

五、实现应用适配
1. 列表、HEAD、签名 GET 和 DELETE 可以使用 S3 SDK，endpoint 为 ${v.gateway}/s3，必须启用 path-style，bucket 为 RVS_BUCKET。
2. 禁止对 VirSree 的 /s3 端点调用 PutObject。禁止把上传文件正文发送给 VirSree。
3. 上传必须严格执行：
   a. POST ${v.gateway}/api/v1/buckets/{bucket}/objects/upload，使用 X-RVS-Access-Key / X-RVS-Secret-Key，JSON 提交 key、准确 size、content_type 和由应用按业务决定的 expires_in。
   b. 把文件 PUT 到响应中的短期 url，并带上响应要求的 headers。S3 来源时该地址是真实 S3/CDN；WebDAV 来源时是 VirSree 中转能力地址。不要记录该 URL。
   c. PUT 成功后 POST 响应中的 commit_url，提交 upload_id 和 key。只有 commit 成功才向业务层报告上传完成。
4. 下载可 POST .../objects/download，提交 key、可选 filename 和应用选择的 expires_in。检查响应 direct：true 表示真实 S3/CDN 直链，false 表示当前对象来自 WebDAV、文件会经 VirSree 短期能力地址中转。
5. “公开链接”也必须由应用调用 .../objects/public-link 创建，并明确提交 sign_expires_in、保存返回的 slug；撤销时调用 DELETE .../public-links/{slug}。底层桶始终 Private。
6. 不要在应用中设置一个平台统一签名时长。每次签名都由当前业务场景选择；必须是正整数；S3 SigV4 的协议上限为 604800 秒。
7. 对网络失败实现有限重试。不要在 PUT 成功、commit 状态未知时盲目申请另一上传；先按文档处理状态，避免预留空间和重复对象。

六、完成验证
使用不含隐私的小文件验证：
1. ListObjectsV2 能列出虚拟桶。
2. 检查响应 direct；为 true 时 URL 主机不是 VirSree，为 false 时确认平台配置的是 WebDAV 中转。
3. PUT 到响应 URL 后 commit 成功，并记录 direct 的布尔值。
4. HEAD 显示正确大小和类型。
5. 申请 60 秒下载能力并读取内容；direct=true 时确认最终主机是 S3/CDN，direct=false 时确认是预期的 WebDAV 中转。
6. 若应用拥有 delete 权限，删除测试对象并确认列表中消失。
7. 确认日志、错误追踪、测试输出和 Git diff 中没有 Token、AK/SK 或签名 URL。

七、最终只报告
- 修改了哪些文件，以及采用了哪个存储适配层。
- 虚拟桶名、权限集合、Secret 的保存位置（只报路径，不报内容）。
- 上传和下载响应的 direct 值、S3/CDN 直达或 WebDAV 中转结果，以及各项验证结果。
- 尚未解决的问题。绝不回显凭据、Bootstrap Token 或签名 URL。`}
