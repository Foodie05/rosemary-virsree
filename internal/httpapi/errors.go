package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
)

type localizedError struct {
	Code string
	ZH   string
	EN   string
}

func errorFor(raw string, status int) localizedError {
	message := strings.ToLower(raw)
	switch {
	case strings.Contains(message, "bucket change requires acknowledge_bucket_change"):
		return localizedError{"storage_bucket_change_unacknowledged", "更换真实存储桶前必须确认数据可用性风险。请检查迁移计划并明确提交风险确认。", "Changing the physical bucket requires an explicit data-availability risk acknowledgement. Review the migration plan and submit the acknowledgement."}
	case strings.Contains(message, "storage source kind cannot be changed"):
		return localizedError{"storage_kind_immutable", "存储源类型创建后不能更改。请添加一个新的存储源。", "A storage source type cannot be changed after creation. Add a new storage source instead."}
	case strings.Contains(message, "storage source capacity cannot be lower"):
		return localizedError{"storage_capacity_too_small", "容量不能低于该存储源已使用和已预留空间的总和。", "Capacity cannot be lower than the source's combined used and reserved space."}
	case strings.Contains(message, "storage verification failed") && (strings.Contains(message, "statuscode: 404") || strings.Contains(message, "notfound") || strings.Contains(message, "404 not found")):
		return localizedError{"storage_probe_not_found", "VirSree 已连接到存储服务，但找不到刚上传的验证对象。请检查桶名、Endpoint 和 Path-style 设置是否匹配。", "VirSree reached the storage service but could not find the verification object it just uploaded. Check the bucket name, endpoint, and path-style setting."}
	case strings.Contains(message, "storage verification failed") && (strings.Contains(message, "accessdenied") || strings.Contains(message, "statuscode: 403") || strings.Contains(message, "signaturedoesnotmatch") || strings.Contains(message, "invalidaccesskeyid")):
		return localizedError{"storage_access_denied", "存储服务拒绝了验证请求。请检查 AK/SK、桶权限、Region，以及服务器时间是否正确。", "The storage service rejected the verification request. Check the access keys, bucket permissions, region, and server clock."}
	case strings.Contains(message, "endpoint format is invalid") || strings.Contains(message, "not a valid uri") || strings.Contains(message, "invalid webdav endpoint"):
		return localizedError{"storage_endpoint_invalid", "无法识别存储地址。可以只填写域名，VirSree 会自动补全 HTTPS；也可以填写完整的 HTTP(S) URL。", "The storage endpoint is not recognized. Enter only the domain and VirSree will add HTTPS, or enter a complete HTTP(S) URL."}
	case strings.Contains(message, "storage verification failed") && (strings.Contains(message, "deadline exceeded") || strings.Contains(message, "timeout") || strings.Contains(message, "no such host") || strings.Contains(message, "connection refused")):
		return localizedError{"storage_unreachable", "无法在限定时间内连接存储服务。请检查地址、DNS、防火墙和网络访问策略。", "VirSree could not reach the storage service in time. Check the endpoint, DNS, firewall, and network access policy."}
	case strings.Contains(message, "storage verification failed") && strings.Contains(message, "direct upload probe"):
		return localizedError{"storage_upload_failed", "直传验证失败。请检查公开上传 Endpoint、桶 CORS、写入权限和 Path-style 设置。", "The direct-upload check failed. Check the public upload endpoint, bucket CORS, write permission, and path-style setting."}
	case strings.Contains(message, "storage verification failed") && strings.Contains(message, "download probe"):
		return localizedError{"storage_download_failed", "下载验证失败。请检查下载/CDN Endpoint 是否支持 S3 SigV4，并保留 Host、路径和签名查询参数。", "The download check failed. Ensure the download/CDN endpoint supports S3 SigV4 and preserves the host, path, and signed query parameters."}
	case strings.Contains(message, "storage verification failed"):
		return localizedError{"storage_verification_failed", "存储源验证没有通过。请核对连接信息和读、写、复制、删除权限，然后使用追踪编号查看服务器日志。", "Storage verification failed. Check the connection settings and read, write, copy, and delete permissions, then use the trace ID to inspect server logs."}
	case strings.Contains(message, "oidc") && strings.Contains(message, "not configured"):
		return localizedError{"oidc_not_configured", "VirSree 尚未配置身份登录，请由管理员运行生产配置脚本。", "Identity sign-in has not been configured for VirSree. Ask an administrator to run the production configuration script."}
	case strings.Contains(message, "json") || strings.Contains(message, "unexpected eof"):
		return localizedError{"invalid_json", "请求内容不是有效的 JSON。", "The request body is not valid JSON."}
	case strings.Contains(message, "admin authentication required"):
		return localizedError{"admin_auth_required", "管理员登录已失效，请重新登录。", "The administrator session is no longer valid. Sign in again."}
	case strings.Contains(message, "credential") && strings.Contains(message, "bucket"):
		return localizedError{"bucket_access_denied", "当前凭据无权访问这个虚拟桶。", "These credentials cannot access this virtual bucket."}
	case strings.Contains(message, "permission") || status == http.StatusForbidden:
		return localizedError{"permission_denied", "当前凭据缺少执行此操作所需的权限。", "These credentials do not have permission to perform this operation."}
	case strings.Contains(message, "not found") || strings.Contains(message, "unavailable") || status == http.StatusNotFound:
		return localizedError{"resource_not_found", "请求的资源不存在或已经失效。", "The requested resource does not exist or is no longer available."}
	case strings.Contains(message, "quota") || strings.Contains(message, "capacity"):
		return localizedError{"quota_exceeded", "可用空间或配额不足，请调整配额或添加存储源。", "There is not enough capacity or quota. Increase the limit or add another storage source."}
	case strings.Contains(message, "expires") || strings.Contains(message, "expired"):
		return localizedError{"expiry_invalid", "有效期无效或已经过期，请提交符合协议限制的正整数秒数。", "The expiry is invalid or has passed. Submit a positive number of seconds within the protocol limit."}
	case strings.Contains(message, "required") || strings.Contains(message, "invalid") || status == http.StatusBadRequest:
		return localizedError{"invalid_request", "提交的信息不完整或格式不正确，请检查标出的字段。", "Some submitted values are missing or invalid. Check the highlighted fields."}
	case status == http.StatusUnauthorized:
		return localizedError{"authentication_required", "需要先完成身份验证。", "Authentication is required."}
	case status >= 500:
		return localizedError{"internal_error", "VirSree 暂时无法完成请求，请稍后重试；如问题持续，请使用追踪编号检查日志。", "VirSree could not complete the request. Try again later and use the trace ID to inspect logs if the problem continues."}
	default:
		return localizedError{"request_failed", "请求没有完成，请检查输入后重试。", "The request could not be completed. Check the input and try again."}
	}
}

func preferChinese(r *http.Request) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(r.Header.Get("Accept-Language"))), "zh")
}

func localText(r *http.Request, zh, en string) string {
	if preferChinese(r) {
		return zh
	}
	return en
}

func requestID(r *http.Request) string {
	if value, ok := r.Context().Value(requestIDKey{}).(string); ok {
		return value
	}
	return "req_unknown"
}

func fail(w http.ResponseWriter, r *http.Request, status int, raw string) {
	failProblem(w, r, status, errorFor(raw, status))
}

func failProblem(w http.ResponseWriter, r *http.Request, status int, problem localizedError) {
	message := problem.EN
	if preferChinese(r) {
		message = problem.ZH
	}
	w.Header().Set("X-VirSree-Error-Code", problem.Code)
	write(w, status, map[string]any{
		"error":      message,
		"code":       problem.Code,
		"message_zh": problem.ZH,
		"message_en": problem.EN,
		"trace_id":   requestID(r),
	})
}

func endpointFingerprint(raw, key string) string {
	value := strings.TrimSpace(strings.TrimLeft(raw, "."))
	if value == "" {
		return "none"
	}
	if !strings.Contains(value, "://") {
		value = "https://" + strings.TrimLeft(value, "/")
	}
	u, err := url.Parse(value)
	host := "invalid"
	if err == nil && u.Hostname() != "" {
		host = strings.ToLower(u.Hostname())
	}
	digest := hmac.New(sha256.New, []byte(key))
	_, _ = digest.Write([]byte(host))
	return "hmac-sha256:" + hex.EncodeToString(digest.Sum(nil)[:6])
}
