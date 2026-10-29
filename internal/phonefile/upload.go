// Package phonefile implements the multi-step file upload used by cloud phones.
//
// The flow mirrors the PMOCK client:
//
//  1. POST /open/api/phone/minio/init/task  -> uploadId + per-part presigned URLs
//  2. PUT each missing part to its presigned URL, collecting ETags
//  3. POST /open/api/phone/completeAndPush  -> merge (OBS) and push to the phone
//
// MinIO merges server-side, so mergeList is empty; OBS requires the ETag list.
package phonefile

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/haidaisiganbei/pmock-cli/internal/client"
)

const (
	// TypeApp marks the upload as an APK/app asset.
	TypeApp = 1
	// TypeFile marks the upload as a temporary file.
	TypeFile = 2

	// MaxFileSize is the documented open API limit.
	MaxFileSize int64 = 1 << 30
	// DefaultChunkSize is the documented minimum chunk size.
	DefaultChunkSize int64 = 30 << 20
	// MaxChunkSize is the documented maximum chunk size.
	MaxChunkSize int64 = 300 << 20
)

// Options describes one upload.
type Options struct {
	PhoneID    int64
	FilePath   string
	UploadType int // TypeApp or TypeFile
	DestDir    string
	ChunkSize  int64
	MD5        string
}

// Plan is the local, network-free part of an upload.
type Plan struct {
	FileName   string
	Size       int64
	MD5        string
	ChunkSize  int64
	ChunkNum   int
	UploadType int
	PhoneID    int64
	DestDir    string
}

// PlanFile validates the file and computes the metadata needed to init a task.
func PlanFile(opts Options) (*Plan, error) {
	if opts.PhoneID <= 0 {
		return nil, fmt.Errorf("phone id is required")
	}
	if strings.TrimSpace(opts.FilePath) == "" {
		return nil, fmt.Errorf("file path is required")
	}

	stat, err := os.Stat(opts.FilePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", opts.FilePath, err)
	}
	if stat.IsDir() {
		return nil, fmt.Errorf("%s is a directory", opts.FilePath)
	}
	if stat.Size() == 0 {
		return nil, fmt.Errorf("%s is empty", opts.FilePath)
	}
	if stat.Size() > MaxFileSize {
		return nil, fmt.Errorf("file is %d bytes; the open API limit is %d bytes (1 GiB)", stat.Size(), MaxFileSize)
	}

	chunkSize := opts.ChunkSize
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	if chunkSize > MaxChunkSize {
		return nil, fmt.Errorf("chunk size %d exceeds the %d byte limit", chunkSize, MaxChunkSize)
	}

	uploadType := opts.UploadType
	if uploadType == 0 {
		uploadType = TypeFile
	}
	if uploadType != TypeApp && uploadType != TypeFile {
		return nil, fmt.Errorf("upload type must be %d (app) or %d (file)", TypeApp, TypeFile)
	}

	sum := opts.MD5
	if sum == "" {
		sum, err = MD5File(opts.FilePath)
		if err != nil {
			return nil, err
		}
	}

	chunkNum := int((stat.Size() + chunkSize - 1) / chunkSize)
	return &Plan{
		FileName:   filepath.Base(opts.FilePath),
		Size:       stat.Size(),
		MD5:        strings.ToLower(strings.TrimSpace(sum)),
		ChunkSize:  chunkSize,
		ChunkNum:   chunkNum,
		UploadType: uploadType,
		PhoneID:    opts.PhoneID,
		DestDir:    opts.DestDir,
	}, nil
}

// MD5File streams the file through MD5.
func MD5File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("cannot open %s: %w", path, err)
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("cannot hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Uploader performs uploads with one API client and one plain HTTP client.
type Uploader struct {
	Client   *client.Client
	HTTP     *http.Client
	Progress func(part, total int)
}

// New builds an Uploader.
func New(c *client.Client) *Uploader {
	return &Uploader{
		Client: c,
		HTTP:   &http.Client{Timeout: 5 * time.Minute},
	}
}

// Upload runs init -> parts -> complete. The returned envelope is the last
// response received, so business failures can still be rendered by the caller.
func (u *Uploader) Upload(ctx context.Context, opts Options) (*client.Envelope, error) {
	plan, err := PlanFile(opts)
	if err != nil {
		return nil, err
	}
	if u.Client == nil {
		return nil, fmt.Errorf("uploader has no API client")
	}
	httpClient := u.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Minute}
	}

	env, err := u.Client.Do(ctx, client.Request{
		Method: "POST",
		Path:   "/open/api/phone/minio/init/task",
		Body: map[string]any{
			"identifier": plan.MD5,
			"totalSize":  plan.Size,
			"chunkSize":  plan.ChunkSize,
			"fileName":   plan.FileName,
			"uploadType": plan.UploadType,
			"phoneId":    plan.PhoneID,
		},
	})
	if err != nil || env == nil {
		return env, err
	}

	var info initResponse
	if err := json.Unmarshal(env.Data, &info); err != nil {
		return nil, fmt.Errorf("failed to parse upload init response: %w", err)
	}
	task := info.TaskRecord
	if task.UploadID == "" {
		return nil, fmt.Errorf("upload init response has no uploadId")
	}

	chunkSize := task.ChunkSize
	if chunkSize <= 0 {
		chunkSize = plan.ChunkSize
	}
	chunkNum := task.ChunkNum
	if chunkNum <= 0 {
		chunkNum = plan.ChunkNum
	}

	existing := make(map[int]bool, len(task.ExitPartList))
	mergeList := make([]map[string]any, 0, chunkNum)
	for _, p := range task.ExitPartList {
		existing[p.PartNumber] = true
		if info.UploadType == "obs" && p.ETag != "" {
			mergeList = append(mergeList, map[string]any{"partNumber": p.PartNumber, "etag": p.ETag})
		}
	}

	if !info.Finished {
		urls := make(map[int]string, len(task.UploadInfo))
		for _, p := range task.UploadInfo {
			urls[p.PartNumber] = p.UploadURL
		}
		for part := 1; part <= chunkNum; part++ {
			if existing[part] {
				continue
			}
			uploadURL := urls[part]
			if uploadURL == "" {
				return nil, fmt.Errorf("init response has no URL for part %d", part)
			}
			offset := int64(part-1) * chunkSize
			if offset >= plan.Size {
				return nil, fmt.Errorf("init response has more parts than the file (chunkNum=%d)", chunkNum)
			}
			size := chunkSize
			if remaining := plan.Size - offset; remaining < size {
				size = remaining
			}
			etag, err := putPart(ctx, httpClient, uploadURL, opts.FilePath, offset, size)
			if err != nil {
				return nil, fmt.Errorf("part %d/%d: %w", part, chunkNum, err)
			}
			if info.UploadType == "obs" {
				if etag == "" {
					return nil, fmt.Errorf("part %d/%d: OBS upload returned no ETag", part, chunkNum)
				}
				mergeList = append(mergeList, map[string]any{"partNumber": part, "etag": etag})
			}
			if u.Progress != nil {
				u.Progress(part, chunkNum)
			}
		}
	}

	name := task.FileName
	if name == "" {
		name = plan.FileName
	}
	complete := map[string]any{
		"phoneId":    plan.PhoneID,
		"uploadId":   task.UploadID,
		"fileName":   name,
		"fileSize":   plan.Size,
		"uploadType": 1, // completeAndPush: 1=normal file, 2=live file
	}
	if plan.DestDir != "" {
		complete["pushDestDir"] = plan.DestDir
	}
	if info.UploadType == "obs" {
		complete["mergeList"] = mergeList
	}

	return u.Client.Do(ctx, client.Request{
		Method: "POST",
		Path:   "/open/api/phone/completeAndPush",
		Body:   complete,
	})
}

type initResponse struct {
	Finished   bool       `json:"finished"`
	UploadType string     `json:"uploadType"`
	TaskRecord uploadTask `json:"taskRecord"`
}

type uploadTask struct {
	UploadID     string        `json:"uploadId"`
	FileName     string        `json:"fileName"`
	TotalSize    int64         `json:"totalSize"`
	ChunkSize    int64         `json:"chunkSize"`
	ChunkNum     int           `json:"chunkNum"`
	UploadInfo   []partURL     `json:"uploadInfo"`
	ExitPartList []partSummary `json:"exitPartList"`
}

type partURL struct {
	PartNumber int    `json:"partNumber"`
	UploadURL  string `json:"uploadUrl"`
}

type partSummary struct {
	PartNumber int    `json:"partNumber"`
	ETag       string `json:"eTag"`
}

func putPart(ctx context.Context, httpClient *http.Client, uploadURL, path string, offset, size int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(attempt) * 500 * time.Millisecond):
			}
		}
		// A new reader per attempt: the previous one is drained on failure.
		section := io.NewSectionReader(f, offset, size)
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, section)
		if err != nil {
			return "", err
		}
		req.ContentLength = size
		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("upload failed with HTTP %d", resp.StatusCode)
			continue
		}
		return strings.Trim(resp.Header.Get("ETag"), `"`), nil
	}
	return "", lastErr
}
