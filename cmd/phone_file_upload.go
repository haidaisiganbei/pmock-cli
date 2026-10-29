package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/pmock/pmock-cli/internal/build"
	"github.com/pmock/pmock-cli/internal/client"
	"github.com/pmock/pmock-cli/internal/exitcode"
	"github.com/pmock/pmock-cli/internal/phonefile"
)

const phoneFileUploadLong = `Upload a local file to a cloud phone: init task -> upload missing parts -> merge and push.

--type file (default) pushes a normal file to /sdcard/Download unless --dest is set.
--type app marks the upload as an APK; the returned data.envFileId can be passed to
pmock-cli app install --file-id.

The MD5 used as the upload identifier is computed locally unless --md5 is given.`

func newPhoneFileUploadCmd(g *globals) *cobra.Command {
	var (
		phoneID  int64
		filePath string
		kind     string
		dest     string
		chunkMB  int64
		md5sum   string
	)

	cmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload a local file or APK to a cloud phone",
		Long:  phoneFileUploadLong,
		Example: `  pmock-cli phone file upload --id 123 --file ./video.mp4
  pmock-cli phone file upload --id 123 --file ./app.apk --type app
  pmock-cli phone file upload --id 123 --file ./video.mp4 --dest /sdcard/Movies
  pmock-cli phone file upload --id 123 --file ./video.mp4 --dry-run`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if phoneID <= 0 {
				return exitcode.Usagef("--id is required")
			}
			if filePath == "" {
				return exitcode.Usagef("--file is required")
			}
			if chunkMB < 30 || chunkMB > 300 {
				return exitcode.Usagef("--chunk-size-mb must be between 30 and 300")
			}

			uploadType := phonefile.TypeFile
			switch kind {
			case "", "file":
			case "app":
				uploadType = phonefile.TypeApp
			default:
				return exitcode.Usagef("invalid --type %q (want file or app)", kind)
			}

			opts := phonefile.Options{
				PhoneID:    phoneID,
				FilePath:   filePath,
				UploadType: uploadType,
				DestDir:    dest,
				ChunkSize:  chunkMB << 20,
				MD5:        md5sum,
			}

			if g.dryRun {
				plan, err := phonefile.PlanFile(opts)
				if err != nil {
					return exitcode.Usagef("%v", err)
				}
				return printUploadPlan(cmd, plan)
			}

			cfg, err := g.config()
			if err != nil {
				return err
			}
			c := client.New(cfg.BaseURL, cfg.AccessKey, cfg.SecretKey, cfg.Timeout)
			c.UserAgent = "pmock-cli/" + build.Version
			uploader := phonefile.New(c)

			showProgress := g.verbose || term.IsTerminal(int(os.Stderr.Fd()))
			if showProgress && opts.MD5 == "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "hashing %s...\n", filePath)
			}
			uploader.Progress = func(part, total int) {
				if showProgress {
					fmt.Fprintf(cmd.ErrOrStderr(), "uploaded part %d/%d\n", part, total)
				}
			}

			// A 1 GiB upload over a slow link can take much longer than the
			// normal API timeout, so use a dedicated budget.
			ctx, cancel := context.WithTimeout(cmd.Context(), 45*time.Minute)
			defer cancel()

			env, err := uploader.Upload(ctx, opts)
			return finish(cmd, g, env, err)
		},
	}

	cmd.Flags().Int64Var(&phoneID, "id", 0, "cloud phone ID (required)")
	cmd.Flags().StringVar(&filePath, "file", "", "local file to upload (required)")
	cmd.Flags().StringVar(&kind, "type", "file", "upload purpose: file | app")
	cmd.Flags().StringVar(&dest, "dest", "", "destination directory on the phone (default /sdcard/Download)")
	cmd.Flags().Int64Var(&chunkMB, "chunk-size-mb", 30, "chunk size in MiB (30-300)")
	cmd.Flags().StringVar(&md5sum, "md5", "", "precomputed MD5 of the file (skips local hashing)")
	return cmd
}

func printUploadPlan(cmd *cobra.Command, plan *phonefile.Plan) error {
	kind := "file"
	if plan.UploadType == phonefile.TypeApp {
		kind = "app"
	}
	out := map[string]any{
		"phoneId":    plan.PhoneID,
		"file":       plan.FileName,
		"size":       plan.Size,
		"md5":        plan.MD5,
		"chunkSize":  plan.ChunkSize,
		"chunkNum":   plan.ChunkNum,
		"uploadType": kind,
	}
	if plan.DestDir != "" {
		out["dest"] = plan.DestDir
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return exitcode.Runtimef("failed to render upload plan: %v", err)
	}
	return nil
}

// extraCommandInfos describes commands that are not generated from a single
// OpenAPI operation. They still appear in `pmock-cli commands`.
func extraCommandInfos() []commandInfo {
	return []commandInfo{{
		Path:     "doctor",
		Method:   "GET",
		HTTPPath: "/open/api/phone/list?pageNum=1&pageSize=1",
		Summary:  "Diagnose config, credentials and API connectivity",
		Risk:     "read",
	}, {
		Path:     "phone file upload",
		Method:   "POST",
		HTTPPath: "/open/api/phone/minio/init/task -> /open/api/phone/completeAndPush",
		Summary:  "Upload a local file or APK to a cloud phone",
		Risk:     "write",
		HasBody:  true,
		Async:    "",
		Flags: []flagInfo{
			{Name: "id", Type: "integer", Field: "phoneId", In: "body", Required: true},
			{Name: "file", Type: "string", Field: "file", In: "local", Required: true},
			{Name: "type", Type: "string", Field: "type", In: "local"},
			{Name: "dest", Type: "string", Field: "pushDestDir", In: "body"},
			{Name: "chunk-size-mb", Type: "integer", Field: "chunkSize", In: "local"},
			{Name: "md5", Type: "string", Field: "identifier", In: "local"},
		},
	}}
}
