# Agent 接入提示词模板

管理台的 **Agent 接入** 页面会把下面的占位符替换为当前实例地址、可信 Release 地址、一次性 Token 和空间上限。请复制管理台生成的版本；不要手工把示例 Token 当成真实凭据。

```text
你负责把当前陌生应用完整接入 Rosemary VirSree 对象存储。不要只给建议；请检查项目、实现适配、运行验证，并在无法自动判断业务选择时才询问我。

【本次接入参数】
- Rosemary 部署实例（所有存储 API 和 S3 endpoint 都配置到这里）：{{GATEWAY}}
- 开源项目（只用于源码、Issue 和通用说明，不能作为存储 endpoint）：{{PROJECT_URL}}
- 可信 CLI Releases（只用于下载 rvsctl）：{{RELEASE_URL}}
- 一次性 Bootstrap Token：{{BOOTSTRAP_TOKEN}}
- 最大可申请空间：{{MAX_QUOTA_BYTES}} 字节
- Token 失效时间：{{TOKEN_EXPIRES_AT}}

Bootstrap Token 只能兑换一次。禁止把 Token、AK、SK、凭据文件内容或任何预签名 URL 输出到对话、日志、提交记录和测试快照中。

一、先检查应用，不要立即改代码
1. 确认语言、框架、包管理器、运行方式和部署环境。
2. 搜索已有文件上传、下载、本地磁盘、S3 SDK、对象 URL 和环境变量使用位置。
3. 判断应用需要 read、write、delete、manage 中的哪些权限，不要扩大权限。
4. 选择 3–63 位、全小写、只含字母数字和连字符的虚拟桶名。
5. 确定生产 Secret 写入位置。优先 Kubernetes Secret、Vault 或云 Secret Manager；本地开发才使用 0600 env 文件。

二、核对地址并阅读实例文档
先请求：
  curl -fsS {{GATEWAY}}/health
  curl -fsS {{GATEWAY}}/api/v1/meta

确认 meta.gateway_url 对应 {{GATEWAY}}，project_url 和 release_url 与本提示一致。随后完整阅读：
- {{GATEWAY}}/docs/integration.md  （从零接入、语言示例、验证和排错）
- {{GATEWAY}}/docs/api.md          （全部请求字段、响应和权限）
- {{GATEWAY}}/docs/architecture.md （数据路径、配额、链接失效边界）

如果 health、meta 或文档不可访问，停止兑换 Token，报告具体 URL 和错误。只有 health 返回 backend_ready=true 才继续对象操作。

三、从可信 GitHub Releases 下载并校验 rvsctl
根据部署机器选择 os=darwin|linux|windows、arch=amd64|arm64。不要运行第三方同名工具。

macOS / Linux：
  OS=$(uname -s | tr '[:upper:]' '[:lower:]')
  ARCH=$(uname -m)
  [ "$ARCH" = x86_64 ] && ARCH=amd64
  [ "$ARCH" = aarch64 ] && ARCH=arm64
  curl -fsSLo ./rvsctl "{{RELEASE_URL}}/latest/download/rvsctl-$OS-$ARCH"
  curl -fsSLo ./SHA256SUMS "{{RELEASE_URL}}/latest/download/SHA256SUMS"
  chmod 0755 ./rvsctl
  EXPECTED=$(awk -v f="rvsctl-$OS-$ARCH" '$2==f {print $1}' SHA256SUMS)
  printf '%s  %s\n' "$EXPECTED" rvsctl | shasum -a 256 -c -

Linux 可将最后的 shasum -a 256 换成 sha256sum。

Windows PowerShell：
  $Arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') {'arm64'} else {'amd64'}
  Invoke-WebRequest "{{RELEASE_URL}}/latest/download/rvsctl-windows-$Arch.exe" -OutFile .\rvsctl.exe
  Invoke-WebRequest "{{RELEASE_URL}}/latest/download/SHA256SUMS" -OutFile .\SHA256SUMS
  Get-FileHash .\rvsctl.exe -Algorithm SHA256

把结果与 SHA256SUMS 对应文件名逐字比较。若文件返回 404、找不到校验条目或哈希不符，不要执行并报告具体 os/arch。

四、只兑换一次并安全落盘
先确认目标文件不存在、父目录只允许部署身份访问，而且该路径已被 Git 忽略。替换下面的应用名称、桶名、配额和 Secret 路径：

  ./rvsctl \
    -endpoint "{{GATEWAY}}" \
    -token '<本提示顶部的一次性 Bootstrap Token>' \
    -name '<应用名称和环境>' \
    -bucket '<虚拟桶名>' \
    -quota '<字节数，不得超过 {{MAX_QUOTA_BYTES}}>' \
    -visibility private \
    -env-file '<部署 Secret 路径>'

rvsctl 会创建虚拟桶，把 AWS_ENDPOINT_URL、AWS_REGION、AWS_ACCESS_KEY_ID、AWS_SECRET_ACCESS_KEY、AWS_S3_FORCE_PATH_STYLE 和 RVS_BUCKET 写入权限为 0600 的文件。它拒绝覆盖已有文件，也不会在终端显示 AK/SK。不要读取或打印该文件；只检查文件存在、权限正确、应用进程可以加载。

五、实现应用适配
1. S3 SDK endpoint 使用 {{GATEWAY}}/s3，启用 path-style，并加载 rvsctl 写入的 region、AK/SK 和桶名。
2. ListObjectsV2、HeadObject、GetObject、DeleteObject 可走 S3 SDK；具体能力仍受 Key 的独立权限控制。
3. 禁止对 Rosemary 的 /s3 调用 PutObject，禁止把上传文件正文发送给 Rosemary。
4. 上传严格执行三步：
   a. POST {{GATEWAY}}/api/v1/buckets/{bucket}/objects/upload。用 X-RVS-Access-Key 和 X-RVS-Secret-Key 认证，JSON 提交 key、准确 size、content_type 和当前业务选择的 expires_in。
   b. 把文件 PUT 到响应中的短期 url，并原样带上 required_headers。S3 来源时它是真实 S3 地址；WebDAV 来源时它是 Rosemary 中转能力地址。不要记录或持久化该 URL。
   c. PUT 成功后 POST 响应中的 commit_url，提交 upload_id 和 key。只有 commit 成功才向业务层报告上传完成。
5. 下载调用 POST {{GATEWAY}}/api/v1/buckets/{bucket}/objects/download，提交 key、可选 filename 和当前业务选择的 expires_in；检查响应 direct：true 表示真实 S3/CDN 直链；false 表示 WebDAV 中转能力地址。
6. “公开链接”调用 .../objects/public-link 创建，也要明确提交 sign_expires_in，并保存返回的 slug；撤销时调用 DELETE .../public-links/{slug}。底层桶始终 Private。
7. 每次签名都由当前业务场景选择有效期。平台没有业务默认值；必须是正整数，S3 SigV4 的协议上限为 604800 秒。
8. 对网络错误做有限重试。真实 S3 PUT 成功但 commit 结果未知时，先按 API 文档核实，避免重复对象和预留空间。

六、完成验证
使用不含隐私的小文件逐项验证：
1. ListObjectsV2 能列出虚拟桶。
2. HeadObject 对不存在对象返回正确的不存在语义。
3. 直接向 {{GATEWAY}}/s3/{bucket}/{key} PUT 返回 405。
4. 检查响应 direct；为 true 时 url 主机不是 Rosemary，为 false 时确认平台配置的是 WebDAV 中转。
5. PUT 到响应 URL 后 commit 成功，HEAD 显示正确大小和类型。
6. 申请 60 秒下载能力且内容一致；按 direct 验证 S3/CDN 直达或 WebDAV 中转。
7. 若有 delete 权限，删除测试对象并确认列表中消失。
8. 没有 write 权限的 Key 申请上传必须返回 403。
9. 日志、错误追踪、测试输出和 Git diff 不含 Token、AK/SK 或签名 URL。

七、最终只报告
- 修改了哪些文件，以及使用哪个存储适配层。
- 虚拟桶名、权限集合、Secret 保存位置（只报路径，不报内容）。
- 上传和下载响应的 direct 值、S3/CDN 直达或 WebDAV 中转结果，以及各项验证结果。
- 尚未解决的问题。

绝不回显凭据、Bootstrap Token 或签名 URL。
```

这个流程降低凭据进入终端和对话记录的风险，但不能从密码学上阻止拥有目标文件读取权限的 Agent。生产环境应把部署身份限制为只能写入或引用 Secret Manager，并让应用运行身份读取 Secret。
