package phonefile

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/haidaisiganbei/pmock-cli/internal/client"
)

type uploadServer struct {
	*httptest.Server
	mu       sync.Mutex
	initBody map[string]any
	complete map[string]any
	parts    map[string][]byte
	partCall int
}

func newUploadServer(t *testing.T, info map[string]any, partStatus int) *uploadServer {
	t.Helper()
	us := &uploadServer{parts: map[string][]byte{}}

	us.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/open/api/phone/minio/init/task":
			body, _ := io.ReadAll(r.Body)
			var m map[string]any
			_ = json.Unmarshal(body, &m)
			us.mu.Lock()
			us.initBody = m
			us.mu.Unlock()

			// Point parts at this same server.
			tr := info["taskRecord"].(map[string]any)
			chunkNum := tr["chunkNum"].(int)
			list := make([]map[string]any, 0, chunkNum)
			for i := 1; i <= chunkNum; i++ {
				list = append(list, map[string]any{
					"partNumber": i,
					"uploadUrl":  us.URL + "/part/" + string(rune('0'+i)),
				})
			}
			tr["uploadInfo"] = list
			out, _ := json.Marshal(map[string]any{"traceId": "init", "code": 0, "msg": "success", "data": info})
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(out)

		case strings.HasPrefix(r.URL.Path, "/part/"):
			us.mu.Lock()
			us.partCall++
			us.mu.Unlock()
			if partStatus != http.StatusOK {
				w.WriteHeader(partStatus)
				return
			}
			data, _ := io.ReadAll(r.Body)
			part := strings.TrimPrefix(r.URL.Path, "/part/")
			us.mu.Lock()
			us.parts[part] = data
			us.mu.Unlock()
			w.Header().Set("ETag", `"etag-`+part+`"`)
			w.WriteHeader(http.StatusOK)

		case r.URL.Path == "/api/open/api/phone/completeAndPush":
			body, _ := io.ReadAll(r.Body)
			var m map[string]any
			_ = json.Unmarshal(body, &m)
			us.mu.Lock()
			us.complete = m
			us.mu.Unlock()
			_, _ = w.Write([]byte(`{"traceId":"complete","code":0,"msg":"success","data":{"phoneId":123,"envFileId":77,"taskId":88}}`))

		default:
			http.NotFound(w, r)
		}
	}))
	return us
}

func newTestClient(t *testing.T, base string) *client.Client {
	t.Helper()
	return client.New(base+"/api", "AK", "SK", 5*time.Second)
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "payload.bin")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestUploadOBS(t *testing.T) {
	info := map[string]any{
		"finished":   false,
		"uploadType": "obs",
		"taskRecord": map[string]any{
			"uploadId":   "u1",
			"fileName":   "payload.bin",
			"chunkSize":  4,
			"chunkNum":   3,
			"uploadInfo": []any{},
			"exitPartList": []any{
				map[string]any{"partNumber": 1, "eTag": "etag-1"},
			},
		},
	}
	us := newUploadServer(t, info, http.StatusOK)
	defer us.Close()

	path := writeTempFile(t, "abcdefghij") // 10 bytes -> 4,4,2
	up := New(newTestClient(t, us.URL))
	var progress []int
	up.Progress = func(part, total int) { progress = append(progress, part) }

	env, err := up.Upload(context.Background(), Options{
		PhoneID:    123,
		FilePath:   path,
		UploadType: TypeFile,
		ChunkSize:  4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !env.OK() {
		t.Fatalf("envelope = %+v", env)
	}

	us.mu.Lock()
	initBody, complete, parts := us.initBody, us.complete, us.parts
	us.mu.Unlock()

	if got := int(initBody["chunkSize"].(float64)); got != 4 {
		t.Errorf("init chunkSize = %d", got)
	}
	if initBody["identifier"] == "" {
		t.Error("init identifier (md5) is empty")
	}
	if got := int(initBody["uploadType"].(float64)); got != TypeFile {
		t.Errorf("init uploadType = %d", got)
	}

	// Part 1 is already present; parts 2 and 3 are uploaded.
	if _, ok := parts["1"]; ok {
		t.Error("part 1 was uploaded although it was in exitPartList")
	}
	if string(parts["2"]) != "efgh" || string(parts["3"]) != "ij" {
		t.Errorf("parts = %q / %q", parts["2"], parts["3"])
	}
	if len(progress) != 2 {
		t.Errorf("progress calls = %v", progress)
	}

	if complete["uploadId"] != "u1" {
		t.Errorf("complete uploadId = %v", complete["uploadId"])
	}
	merge, ok := complete["mergeList"].([]any)
	if !ok || len(merge) != 3 {
		t.Fatalf("mergeList = %#v", complete["mergeList"])
	}
	if merge[1].(map[string]any)["etag"] != "etag-2" {
		t.Errorf("merge[1] = %#v", merge[1])
	}
}

func TestUploadMinIOFinished(t *testing.T) {
	info := map[string]any{
		"finished":   true,
		"uploadType": "minio",
		"taskRecord": map[string]any{
			"uploadId":     "u2",
			"fileName":     "payload.bin",
			"chunkSize":    4,
			"chunkNum":     1,
			"uploadInfo":   []any{},
			"exitPartList": []any{map[string]any{"partNumber": 1, "eTag": "etag-1"}},
		},
	}
	us := newUploadServer(t, info, http.StatusOK)
	defer us.Close()

	up := New(newTestClient(t, us.URL))
	if _, err := up.Upload(context.Background(), Options{
		PhoneID: 123, FilePath: writeTempFile(t, "abcd"), UploadType: TypeApp, ChunkSize: 4,
	}); err != nil {
		t.Fatal(err)
	}

	us.mu.Lock()
	partCalls, complete := us.partCall, us.complete
	us.mu.Unlock()
	if partCalls != 0 {
		t.Errorf("part calls = %d, want 0 for finished upload", partCalls)
	}
	if _, ok := complete["mergeList"]; ok {
		t.Errorf("minio complete must not send mergeList: %#v", complete)
	}
}

func TestUploadPartFailure(t *testing.T) {
	info := map[string]any{
		"finished":   false,
		"uploadType": "minio",
		"taskRecord": map[string]any{
			"uploadId": "u3", "fileName": "payload.bin", "chunkSize": 4, "chunkNum": 1,
			"uploadInfo": []any{}, "exitPartList": []any{},
		},
	}
	us := newUploadServer(t, info, http.StatusForbidden)
	defer us.Close()

	up := New(newTestClient(t, us.URL))
	_, err := up.Upload(context.Background(), Options{
		PhoneID: 123, FilePath: writeTempFile(t, "abcd"), ChunkSize: 4,
	})
	if err == nil || !strings.Contains(err.Error(), "part 1/1") {
		t.Fatalf("err = %v", err)
	}
	us.mu.Lock()
	complete := us.complete
	us.mu.Unlock()
	if complete != nil {
		t.Error("completeAndPush must not run after a part failure")
	}
}

func TestPlanFile(t *testing.T) {
	path := writeTempFile(t, "abcdefghij")
	plan, err := PlanFile(Options{PhoneID: 1, FilePath: path, ChunkSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	if plan.ChunkNum != 3 || plan.Size != 10 || plan.UploadType != TypeFile {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.MD5 != "a925576942e94b2ef57a066101b48876" {
		t.Fatalf("md5 = %s", plan.MD5)
	}

	if _, err := PlanFile(Options{PhoneID: 1, FilePath: path, ChunkSize: MaxChunkSize + 1}); err == nil {
		t.Fatal("expected an error for an oversized chunk")
	}
	if _, err := PlanFile(Options{PhoneID: 0, FilePath: path}); err == nil {
		t.Fatal("expected an error for a missing phone id")
	}
}
