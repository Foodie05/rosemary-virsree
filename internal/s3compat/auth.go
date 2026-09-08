package s3compat

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"rosemary-virsree/internal/service"
)

type proof struct {
	access, scope, signed, signature, amzDate, payload string
	expires                                            int64
	presigned                                          bool
}

func (g *Gateway) authenticate(r *http.Request, permission, bucket string) (service.Credential, int64, error) {
	p, e := parseProof(r)
	if e != nil {
		return service.Credential{}, 0, e
	}
	k, b, e := g.svc.DB.AccessByAK(r.Context(), p.access)
	if e != nil || k.Revoked {
		return service.Credential{}, 0, errors.New("invalid access key")
	}
	secret, e := g.svc.Box.Open(k.SecretCipher)
	if e != nil {
		return service.Credential{}, 0, e
	}
	if b.Slug != bucket {
		return service.Credential{}, 0, errors.New("bucket not granted")
	}
	if !permissionOK(k.Permissions, permission) {
		return service.Credential{}, 0, errors.New("permission denied")
	}
	if e = verifyRequest(r, p, secret); e != nil {
		return service.Credential{}, 0, e
	}
	return service.Credential{AccessKey: k.AK, SecretKey: secret, Bucket: b, Key: k}, p.expires, nil
}
func permissionOK(csv, want string) bool {
	for _, p := range strings.Split(csv, ",") {
		if p == want {
			return true
		}
	}
	return false
}
func parseProof(r *http.Request) (proof, error) {
	if q := r.URL.Query(); q.Get("X-Amz-Algorithm") == "AWS4-HMAC-SHA256" {
		cred := q.Get("X-Amz-Credential")
		parts := strings.SplitN(cred, "/", 2)
		if len(parts) != 2 {
			return proof{}, errors.New("bad credential")
		}
		ex, e := strconv.ParseInt(q.Get("X-Amz-Expires"), 10, 64)
		if e != nil || ex < 1 || ex > 604800 {
			return proof{}, errors.New("invalid X-Amz-Expires")
		}
		return proof{access: parts[0], scope: parts[1], signed: q.Get("X-Amz-SignedHeaders"), signature: q.Get("X-Amz-Signature"), amzDate: q.Get("X-Amz-Date"), payload: "UNSIGNED-PAYLOAD", expires: ex, presigned: true}, nil
	}
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "AWS4-HMAC-SHA256 ") {
		return proof{}, errors.New("missing SigV4 authorization")
	}
	m := map[string]string{}
	for _, v := range strings.Split(strings.TrimPrefix(h, "AWS4-HMAC-SHA256 "), ",") {
		kv := strings.SplitN(strings.TrimSpace(v), "=", 2)
		if len(kv) == 2 {
			m[kv[0]] = kv[1]
		}
	}
	parts := strings.SplitN(m["Credential"], "/", 2)
	if len(parts) != 2 {
		return proof{}, errors.New("bad credential")
	}
	ex, _ := strconv.ParseInt(r.URL.Query().Get("rvs-expires"), 10, 64)
	if ex == 0 {
		ex, _ = strconv.ParseInt(r.Header.Get("X-RVS-Expires-In"), 10, 64)
	}
	return proof{access: parts[0], scope: parts[1], signed: m["SignedHeaders"], signature: m["Signature"], amzDate: r.Header.Get("X-Amz-Date"), payload: r.Header.Get("X-Amz-Content-Sha256"), expires: ex}, nil
}
func verifyRequest(r *http.Request, p proof, secret string) error {
	if p.signature == "" || p.scope == "" || p.amzDate == "" {
		return errors.New("incomplete SigV4 authorization")
	}
	t, e := time.Parse("20060102T150405Z", p.amzDate)
	if e != nil {
		return errors.New("invalid X-Amz-Date")
	}
	if p.presigned {
		if time.Now().UTC().After(t.Add(time.Duration(p.expires) * time.Second)) {
			return errors.New("presigned request expired")
		}
	} else if d := time.Since(t); d > 15*time.Minute || d < -15*time.Minute {
		return errors.New("request time skewed")
	}
	canonical := r.Method + "\n" + canonicalURI(r.URL) + "\n" + canonicalQuery(r.URL, p.presigned) + "\n" + canonicalHeaders(r, p.signed) + "\n" + p.signed + "\n" + payload(p.payload)
	sum := sha256.Sum256([]byte(canonical))
	sts := "AWS4-HMAC-SHA256\n" + p.amzDate + "\n" + p.scope + "\n" + hex.EncodeToString(sum[:])
	sc := strings.Split(p.scope, "/")
	if len(sc) != 4 || sc[2] != "s3" || sc[3] != "aws4_request" {
		return errors.New("invalid credential scope")
	}
	dateKey := mac([]byte("AWS4"+secret), sc[0])
	region := mac(dateKey, sc[1])
	serviceKey := mac(region, sc[2])
	signing := mac(serviceKey, sc[3])
	want := hex.EncodeToString(mac(signing, sts))
	if subtle.ConstantTimeCompare([]byte(want), []byte(p.signature)) != 1 {
		return errors.New("signature mismatch")
	}
	return nil
}
func payload(v string) string {
	if v == "" {
		return "UNSIGNED-PAYLOAD"
	}
	return v
}
func mac(key []byte, v string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(v))
	return h.Sum(nil)
}
func canonicalURI(u *url.URL) string {
	p := u.EscapedPath()
	if p == "" {
		return "/"
	}
	return p
}
func canonicalQuery(u *url.URL, dropSig bool) string {
	q := u.Query()
	if dropSig {
		q.Del("X-Amz-Signature")
	}
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []string
	for _, k := range keys {
		vs := q[k]
		sort.Strings(vs)
		for _, v := range vs {
			out = append(out, awsEscape(k)+"="+awsEscape(v))
		}
	}
	return strings.Join(out, "&")
}
func awsEscape(v string) string { return strings.ReplaceAll(url.QueryEscape(v), "+", "%20") }
func canonicalHeaders(r *http.Request, signed string) string {
	var b strings.Builder
	for _, name := range strings.Split(signed, ";") {
		var v string
		if name == "host" {
			v = r.Host
		} else {
			v = strings.Join(r.Header.Values(http.CanonicalHeaderKey(name)), ",")
		}
		b.WriteString(name)
		b.WriteByte(':')
		b.WriteString(strings.Join(strings.Fields(v), " "))
		b.WriteByte('\n')
	}
	return b.String()
}
