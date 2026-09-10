package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Listen, PublicURL, DatabasePath, WebDir                     string
	DocsDir, DownloadDir                                        string
	ProjectURL, ReleaseURL                                      string
	AdminToken, MasterKey                                       string
	OIDCIssuer, OIDCClientID, OIDCClientSecret, OIDCRedirectURL string
	AdminEmails                                                 []string
	SessionTTL                                                  int64
	TotalQuota                                                  int64
	MaxObjectsPerBucket, MaxPendingUploads                      int64
	BackupInterval                                              int64
	BackupRetention                                             int
	Backend                                                     Backend
}

type Backend struct {
	Endpoint, PublicEndpoint, DownloadEndpoint, Region, Bucket, AccessKey, SecretKey string
	PathStyle                                                                        bool
	DownloadMode, DownloadAuthKey                                                    string
}

func Load() (Config, error) {
	c := Config{
		Listen: env("RVS_LISTEN", "127.0.0.1:8080"), PublicURL: env("RVS_PUBLIC_URL", "http://127.0.0.1:8080"),
		DatabasePath: env("RVS_DATABASE", "rosemary.db"), WebDir: env("RVS_WEB_DIR", "web/dist"),
		DocsDir: env("RVS_DOCS_DIR", "docs"), DownloadDir: env("RVS_DOWNLOAD_DIR", "downloads"),
		ProjectURL: env("RVS_PROJECT_URL", "https://github.com/Foodie05/rosemary-virsree"), ReleaseURL: env("RVS_RELEASE_URL", "https://github.com/Foodie05/rosemary-virsree/releases"),
		AdminToken: os.Getenv("RVS_ADMIN_TOKEN"), MasterKey: os.Getenv("RVS_MASTER_KEY"),
		OIDCIssuer:   strings.TrimRight(env("RVS_OIDC_ISSUER", "https://apiauth.cruty.cn"), "/"),
		OIDCClientID: os.Getenv("RVS_OIDC_CLIENT_ID"), OIDCClientSecret: os.Getenv("RVS_OIDC_CLIENT_SECRET"),
		OIDCRedirectURL: env("RVS_OIDC_REDIRECT_URL", ""), AdminEmails: splitCSV(os.Getenv("RVS_ADMIN_EMAILS")),
		SessionTTL: envInt("RVS_SESSION_TTL_SECONDS", 12*60*60),
		TotalQuota: envInt("RVS_TOTAL_QUOTA", 100<<30), MaxObjectsPerBucket: envInt("RVS_MAX_OBJECTS_PER_BUCKET", 1_000_000), MaxPendingUploads: envInt("RVS_MAX_PENDING_UPLOADS_PER_BUCKET", 1_000),
		BackupInterval: envInt("RVS_BACKUP_INTERVAL_SECONDS", 6*60*60), BackupRetention: int(envInt("RVS_BACKUP_RETENTION", 7)),
		Backend: Backend{Endpoint: os.Getenv("RVS_S3_ENDPOINT"), PublicEndpoint: os.Getenv("RVS_S3_PUBLIC_ENDPOINT"),
			DownloadEndpoint: os.Getenv("RVS_S3_DOWNLOAD_ENDPOINT"),
			Region:           env("RVS_S3_REGION", "us-east-1"), Bucket: os.Getenv("RVS_S3_BUCKET"), AccessKey: os.Getenv("RVS_S3_ACCESS_KEY"),
			SecretKey: os.Getenv("RVS_S3_SECRET_KEY"), PathStyle: envBool("RVS_S3_PATH_STYLE", true)},
	}
	if len(c.AdminToken) < 24 || strings.HasPrefix(c.AdminToken, "change-this") {
		return c, fmt.Errorf("RVS_ADMIN_TOKEN must be a non-placeholder secret of at least 24 characters")
	}
	if len(c.MasterKey) < 32 || strings.HasPrefix(c.MasterKey, "change-this") {
		return c, fmt.Errorf("RVS_MASTER_KEY must be a non-placeholder secret of at least 32 characters")
	}
	if c.TotalQuota <= 0 || c.MaxObjectsPerBucket <= 0 || c.MaxPendingUploads <= 0 {
		return c, fmt.Errorf("quota and object/upload limits must be greater than zero")
	}
	if c.SessionTTL < 300 {
		return c, fmt.Errorf("RVS_SESSION_TTL_SECONDS must be at least 300")
	}
	if c.BackupInterval < 60 || c.BackupRetention < 2 || c.BackupRetention > 30 {
		return c, fmt.Errorf("backup interval must be at least 60 seconds and retention must be between 2 and 30")
	}
	c.PublicURL = strings.TrimRight(c.PublicURL, "/")
	configuredOIDC := c.OIDCClientID != "" || c.OIDCClientSecret != "" || len(c.AdminEmails) > 0
	if configuredOIDC {
		if c.OIDCClientID == "" || c.OIDCClientSecret == "" || len(c.AdminEmails) == 0 {
			return c, fmt.Errorf("RVS_OIDC_CLIENT_ID, RVS_OIDC_CLIENT_SECRET and RVS_ADMIN_EMAILS must be configured together")
		}
		if c.OIDCRedirectURL == "" {
			c.OIDCRedirectURL = c.PublicURL + "/auth/callback"
		}
	}
	urls := map[string]string{"RVS_PUBLIC_URL": c.PublicURL, "RVS_PROJECT_URL": c.ProjectURL, "RVS_RELEASE_URL": c.ReleaseURL}
	if configuredOIDC {
		urls["RVS_OIDC_ISSUER"] = c.OIDCIssuer
		urls["RVS_OIDC_REDIRECT_URL"] = c.OIDCRedirectURL
	}
	for name, raw := range urls {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return c, fmt.Errorf("%s must be an absolute http(s) URL", name)
		}
	}
	c.ProjectURL = strings.TrimRight(c.ProjectURL, "/")
	c.ReleaseURL = strings.TrimRight(c.ReleaseURL, "/")
	if c.Backend.PublicEndpoint == "" {
		c.Backend.PublicEndpoint = c.Backend.Endpoint
	}
	if c.Backend.DownloadEndpoint == "" {
		c.Backend.DownloadEndpoint = c.Backend.PublicEndpoint
	}
	return c, nil
}

func (c Config) OIDCReady() bool {
	return c.OIDCClientID != "" && c.OIDCClientSecret != "" && len(c.AdminEmails) > 0
}

func (c Config) BackendReady() bool {
	return c.Backend.Bucket != "" && c.Backend.AccessKey != "" && c.Backend.SecretKey != ""
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func envInt(k string, d int64) int64 {
	v := os.Getenv(k)
	if v == "" {
		return d
	}
	n, e := strconv.ParseInt(v, 10, 64)
	if e != nil {
		return d
	}
	return n
}
func envBool(k string, d bool) bool {
	v := os.Getenv(k)
	if v == "" {
		return d
	}
	b, e := strconv.ParseBool(v)
	if e != nil {
		return d
	}
	return b
}

func splitCSV(v string) []string {
	var out []string
	for _, item := range strings.Split(v, ",") {
		item = strings.ToLower(strings.TrimSpace(item))
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}
