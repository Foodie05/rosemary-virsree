package s3compat

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestS3ErrorUsesLanguageAndTraceID(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/s3/example/missing", nil)
	r.Header.Set("Accept-Language", "zh-CN")
	w := httptest.NewRecorder()
	w.Header().Set("X-Request-ID", "req_test")
	s3error(w, r, http.StatusNotFound, "NoSuchKey", "对象不存在。", "Object not found.")
	var got struct {
		Code, Message, RequestID string
	}
	if err := xml.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Code != "NoSuchKey" || got.Message != "对象不存在。" || got.RequestID != "req_test" || w.Header().Get("X-VirSree-Error-Code") != "NoSuchKey" {
		t.Fatalf("unexpected S3 error: %#v", got)
	}
}
