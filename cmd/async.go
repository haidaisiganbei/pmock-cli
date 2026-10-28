package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/spf13/cobra"

	"github.com/pmock/pmock-cli/internal/client"
	"github.com/pmock/pmock-cli/internal/exitcode"
	"github.com/pmock/pmock-cli/internal/spec"
)

type waitInfo struct {
	OK              bool   `json:"ok"`
	TimedOut        bool   `json:"timedOut"`
	TaskStatus      *int   `json:"taskStatus,omitempty"`
	PhoneStatus     *int   `json:"phoneStatus,omitempty"`
	TaskStatusName  string `json:"taskStatusName,omitempty"`
	PhoneStatusName string `json:"phoneStatusName,omitempty"`
	Attempts        int    `json:"attempts"`
	WaitedMs        int64  `json:"waitedMs"`
	Error           string `json:"error,omitempty"`
}

// waitForTask polls the task endpoint until a terminal state or timeout and
// annotates the accepted envelope with a `wait` object. The envelope keeps the
// API's own code; wait failures are reported through the process exit code.
func waitForTask(cmd *cobra.Command, c *client.Client, sc *spec.Command, env *client.Envelope) (err error) {
	timeout, _ := cmd.Flags().GetDuration("wait-timeout")
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}
	interval, _ := cmd.Flags().GetDuration("poll-interval")
	if interval <= 0 {
		interval = 2 * time.Second
	}
	if interval < 50*time.Millisecond {
		interval = 50 * time.Millisecond
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()

	start := time.Now()
	info := &waitInfo{}
	defer func() {
		info.WaitedMs = time.Since(start).Milliseconds()
		if aerr := annotateWait(env, info); aerr != nil && err == nil {
			err = aerr
		}
	}()

	switch sc.Async {
	case "phone":
		return waitPhone(ctx, cmd, c, sc, env, info, interval)
	case "action":
		return waitAction(ctx, cmd, c, env, info, interval)
	default:
		return exitcode.Usagef("command %s has no async policy", sc.FullPath())
	}
}

func waitPhone(ctx context.Context, cmd *cobra.Command, c *client.Client, sc *spec.Command, env *client.Envelope, info *waitInfo, interval time.Duration) error {
	phoneID, _ := cmd.Flags().GetInt64("id")
	if phoneID <= 0 {
		if v, ok := int64FromData(env.Data, "id"); ok {
			phoneID = v
		}
	}
	if phoneID <= 0 {
		info.Error = "cannot determine phone id"
		return exitcode.Usagef("--wait needs a phone id")
	}
	if failed, _ := boolFromData(env.Data, "failed"); failed {
		info.Error = "phone operation reported failure"
		return exitcode.New(exitcode.Business, fmt.Errorf("phone operation failed before polling"))
	}

	for {
		res, err := c.Do(ctx, client.Request{
			Method: "GET",
			Path:   "/open/api/phone/task/status",
			Query: url.Values{
				"phoneId":     {fmt.Sprint(phoneID)},
				"operateType": {sc.OperateType},
			},
		})
		info.Attempts++
		if err != nil {
			info.Error = err.Error()
			return exitcode.Runtimef("failed to poll phone status: %v", err)
		}

		var st struct {
			TaskStatus      *int   `json:"taskStatus"`
			TaskStatusName  string `json:"taskStatusName"`
			PhoneStatus     *int   `json:"phoneStatus"`
			PhoneStatusName string `json:"phoneStatusName"`
		}
		if err := json.Unmarshal(res.Data, &st); err != nil {
			info.Error = "invalid phone status payload"
			return exitcode.Runtimef("failed to parse phone status: %v", err)
		}
		if st.TaskStatus != nil {
			v := *st.TaskStatus
			info.TaskStatus = &v
		}
		if st.PhoneStatus != nil {
			v := *st.PhoneStatus
			info.PhoneStatus = &v
		}
		info.TaskStatusName = st.TaskStatusName
		info.PhoneStatusName = st.PhoneStatusName

		if st.TaskStatus != nil {
			switch *st.TaskStatus {
			case 3:
				info.Error = "task failed"
				return exitcode.New(exitcode.Business,
					fmt.Errorf("phone task failed (taskStatus=3, phoneStatus=%s)", formatIntPtr(st.PhoneStatus)))
			case 2:
				if phoneSettled(sc.OperateType, st.PhoneStatus) {
					info.OK = true
					return nil
				}
			}
		}

		if ctx.Err() != nil {
			info.TimedOut = true
			info.Error = "timeout"
			return exitcode.Runtimef("timed out waiting for %s (last taskStatus=%s, phoneStatus=%s)",
				sc.FullPath(), formatIntPtr(info.TaskStatus), formatIntPtr(info.PhoneStatus))
		}
		if !sleep(ctx, interval) {
			info.TimedOut = true
			info.Error = "timeout"
			return exitcode.Runtimef("timed out waiting for %s", sc.FullPath())
		}
	}
}

func waitAction(ctx context.Context, cmd *cobra.Command, c *client.Client, env *client.Envelope, info *waitInfo, interval time.Duration) error {
	code, _ := cmd.Flags().GetString("code")
	if code == "" {
		code = stringFromJSONFlag(cmd, "code")
	}
	if code == "" {
		info.Error = "cannot determine phone code"
		return exitcode.Usagef("--wait needs --code")
	}

	taskID, ok := taskIDFromData(env.Data)
	if !ok || taskID <= 0 {
		info.Error = "response has no taskId"
		return exitcode.Runtimef("the API response does not contain a task id; cannot wait")
	}
	// Normalize a numeric RLong body into an object so the wait annotation has
	// a stable place to live.
	env.Data = json.RawMessage(fmt.Sprintf(`{"taskId":%d}`, taskID))

	for {
		res, err := c.Do(ctx, client.Request{
			Method: "POST",
			Path:   "/open/api/phone/action/task/status",
			Body:   map[string]any{"code": code, "taskId": taskID},
		})
		info.Attempts++
		if err != nil {
			info.Error = err.Error()
			return exitcode.Runtimef("failed to poll action status: %v", err)
		}

		var st struct {
			TaskStatus *int `json:"taskStatus"`
		}
		if err := json.Unmarshal(res.Data, &st); err != nil {
			info.Error = "invalid action status payload"
			return exitcode.Runtimef("failed to parse action status: %v", err)
		}
		if st.TaskStatus != nil {
			v := *st.TaskStatus
			info.TaskStatus = &v
			// The public endpoint returns 1=running, 2=success, 3=fail in
			// practice (the OpenAPI enum description says -1/0/1/2); accept
			// both failure encodings.
			switch v {
			case 2:
				info.OK = true
				return nil
			case 3, -1:
				info.Error = "action task failed"
				return exitcode.New(exitcode.Business,
					fmt.Errorf("action task failed (taskStatus=%d)", v))
			}
		}

		if ctx.Err() != nil {
			info.TimedOut = true
			info.Error = "timeout"
			return exitcode.Runtimef("timed out waiting for action task %d (last taskStatus=%s)",
				taskID, formatIntPtr(info.TaskStatus))
		}
		if !sleep(ctx, interval) {
			info.TimedOut = true
			info.Error = "timeout"
			return exitcode.Runtimef("timed out waiting for action task %d", taskID)
		}
	}
}

func phoneSettled(operateType string, phoneStatus *int) bool {
	switch operateType {
	case "powerOn":
		return phoneStatus != nil && *phoneStatus == 1
	case "powerOff":
		return phoneStatus != nil && *phoneStatus == 4
	default:
		// restart / reset / renew: taskStatus=2 is the terminal signal; the
		// phone may still be transitioning through 41, which is fine.
		return true
	}
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func annotateWait(env *client.Envelope, info *waitInfo) error {
	if env == nil {
		return nil
	}
	data := map[string]any{}
	if len(env.Data) > 0 && string(env.Data) != "null" {
		if err := json.Unmarshal(env.Data, &data); err != nil {
			var v any
			if uerr := json.Unmarshal(env.Data, &v); uerr == nil {
				data["result"] = v
			}
		}
	}
	data["wait"] = info
	b, err := json.Marshal(data)
	if err != nil {
		return exitcode.Runtimef("failed to annotate wait result: %v", err)
	}
	env.Data = b
	return nil
}

func int64FromData(raw json.RawMessage, field string) (int64, bool) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return 0, false
	}
	v, ok := m[field]
	if !ok {
		return 0, false
	}
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case json.Number:
		n, err := t.Int64()
		return n, err == nil
	}
	return 0, false
}

func boolFromData(raw json.RawMessage, field string) (bool, bool) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return false, false
	}
	v, ok := m[field].(bool)
	return v, ok
}

func taskIDFromData(raw json.RawMessage) (int64, bool) {
	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n, true
	}
	return int64FromData(raw, "taskId")
}

func stringFromJSONFlag(cmd *cobra.Command, field string) string {
	if !cmd.Flags().Changed("json") {
		return ""
	}
	raw, err := cmd.Flags().GetString("json")
	if err != nil {
		return ""
	}
	m, err := parseJSONObject(raw)
	if err != nil {
		return ""
	}
	if v, ok := m[field].(string); ok {
		return v
	}
	return ""
}

func formatIntPtr(v *int) string {
	if v == nil {
		return "?"
	}
	return fmt.Sprint(*v)
}
