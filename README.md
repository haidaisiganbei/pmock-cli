[![CI](https://github.com/haidaisiganbei/pmock-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/haidaisiganbei/pmock-cli/actions/workflows/ci.yml)
[![Release](https://github.com/haidaisiganbei/pmock-cli/actions/workflows/release.yml/badge.svg)](https://github.com/haidaisiganbei/pmock-cli/actions/workflows/release.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/haidaisiganbei/pmock-cli.svg)](https://pkg.go.dev/github.com/haidaisiganbei/pmock-cli)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

# pmock-cli

PMOCK 开放 API 的独立命令行客户端。用团队 **Access Key / Secret Key** 直接调用
`https://user.pmock.com/api/open/api/*`，不依赖 PMOCK 桌面客户端、不依赖登录态，可运行在
CI、服务器和第三方开发者机器上。

> 与随客户端安装的 `pmock` 命令区分：`pmock` 走本机 MCP / 登录态，只服务本机；
> `pmock-cli` 走开放 API + AK/SK，是可独立分发的二进制。

- 语言：Go 1.25+
- 命令框架：Cobra
- API 规范：[pmock-openapi.yaml](openapi/pmock-openapi.yaml)（60 个 operation，129 个 schema）
- 许可：Apache-2.0

---

## 状态（P0）

| 能力 | 状态 |
| --- | --- |
| 60 个开放 API operation 生成命令 | ✅ |
| AK/SK 签名、环境变量 / 配置文件 | ✅ |
| `config init` / `config show` / `auth status` | ✅ |
| JSON / pretty / table 输出 | ✅ |
| 稳定退出码（0/1/2/3） | ✅ |
| `--dry-run`、破坏性操作 `--yes` | ✅ |
| 429 / 5xx 重试（GET，POST 默认不重试） | ✅ |
| `--json` 嵌套请求体 | ✅ |
| `phone file upload` 一体化上传（MD5 → 分片 → 合并推送） | ✅ |
| `--wait` 异步任务轮询（开关机 / UI 动作） | ✅ |
| `pmock-cli mcp`（12 个分组 tool / 63 个 action） | ✅ |
| GoReleaser / Release workflow / 本地打包 | ✅ |
| npm 平台包（构建 + 本地安装验证） | ✅ |
| Homebrew formula | ✅ |
| npm 正式发布 / Skills 文档 | 需 npm 登录；Skills 为后续增强 |


---

## 安装

一键安装（macOS / Linux，无需 npm）：

```bash
curl -fsSL https://raw.githubusercontent.com/haidaisiganbei/pmock-cli/main/scripts/install.sh | sh
```

npm（已发布）：

```bash
npm install -g pmock-cli@latest
```

> macOS / Linux 已验证可用。Windows 平台包 `pmock-cli-win32-x64` 仍被 npm
> 的 spam 审核拦截，Windows 暂时请下载 Release 中的 `.zip`。

预编译二进制（macOS / Linux / Windows）：

```bash
# 以 v0.1.2 darwin-arm64 为例
curl -L -o pmock-cli.tar.gz \
  https://github.com/haidaisiganbei/pmock-cli/releases/download/v0.1.2/pmock-cli_0.1.2_darwin_arm64.tar.gz
tar -xzf pmock-cli.tar.gz
./pmock-cli --version
```

完整产物见 [Releases](https://github.com/haidaisiganbei/pmock-cli/releases)。

Homebrew（本地 formula，v0.1.2 已带 checksum）：

```bash
brew install --formula ./Formula/pmock-cli.rb
```

从源码：

```bash
git clone https://github.com/haidaisiganbei/pmock-cli.git
cd pmock-cli
make build
./bin/pmock-cli --version
```

或：

```bash
go install github.com/haidaisiganbei/pmock-cli@latest
```

---

## 配置

```bash
# 交互式，Secret 输入不回显
pmock-cli config init

# 非交互式
pmock-cli config init --access-key "AK..." --secret-key "SK..."
```

也可以完全用环境变量（CI 推荐）：

```bash
export PMOCK_ACCESS_KEY="AK..."
export PMOCK_SECRET_KEY="SK..."
# 可选
export PMOCK_BASE_URL="https://user.pmock.com/api"
export PMOCK_TIMEOUT="30s"
```

优先级：命令行 flag > 环境变量 > `~/.pmock-cli/config.json` > 内置默认值。

配置文件以 `0600` 写入，`config show` 只显示掩码。`PMOCK_CLI_CONFIG` 可指定其他路径，
便于多套凭证或测试隔离。

签名规则（与官方《对接须知》一致）：

```text
timestamp    = 当前秒级时间戳向下取整到分钟
stringToSign = accessKey + ":" + timestamp
sign         = base64(HMAC-SHA256(secretKey, stringToSign))
headers      = pmock-access-key, pmock-access-token
```

验证：

```bash
pmock-cli auth status
```

---

## 快速开始

```bash
# 连通性与凭证
pmock-cli auth status

# 商品、手机列表
pmock-cli product list
pmock-cli phone list --page-size 10
pmock-cli phone list --status 1 --format table

# 开关机（默认返回 taskId；--wait 等到终态并在 data.wait 里带回状态）
pmock-cli phone start --id 123 --wait
pmock-cli phone stop --id 123 --wait --wait-timeout 3m

# 看屏与 UI 自动化（动作类用 code，不是 id）
pmock-cli phone screen --id 123
pmock-cli phone action click --code A1 --pos-type 2 --pos 100,200 --wait
pmock-cli phone action input --code A1 --text "hello" --wait

# 文件上传：自动算 MD5、分片、合并并推送到手机
pmock-cli phone file upload --id 123 --file ./video.mp4
pmock-cli phone file upload --id 123 --file ./app.apk --type app
pmock-cli phone file upload --id 123 --file ./video.mp4 --dest /sdcard/Movies

# 应用
pmock-cli app list --id 123 --page-size 20
pmock-cli app install --id 123 --file-id 456 --apk-pkg com.example.app \
  --apk-name Example --apk-version-name 1.0 --apk-version-code 1

# RPA
pmock-cli rpa template list
pmock-cli rpa task list --status 1
pmock-cli rpa task add --template-id 9 --name daily --json @task.json
```

查看某条命令的完整参数：

```bash
pmock-cli docs phone start
pmock-cli phone start --help
```

机器可读命令目录：

```bash
pmock-cli commands | jq '.[] | select(.risk=="destructive") | .path'
```

---

## 文件上传

`phone file upload` 把 `minio/init/task`、分片 PUT、`completeAndPush` 串成一条命令：

```bash
pmock-cli phone file upload --id 123 --file ./video.mp4
```

- 自动计算 MD5 作为 `identifier`（可用 `--md5` 跳过）；
- 默认分片 30 MiB，可用 `--chunk-size-mb` 调整（30–300）；
- `--type app` 用于 APK，返回的 `data.envFileId` 可直接传给 `app install --file-id`；
- 默认推送到 `/sdcard/Download`，可用 `--dest` 覆盖；
- 断点续传：服务端返回已上传分片时会跳过；
- `--dry-run` 只打印文件大小、MD5、分片数与目标，不发请求。

## 异步等待

`phone start/stop/restart/reset/new`、`phone action *` 和 `rpa task add` 支持：

```bash
pmock-cli phone start --id 123 --wait
pmock-cli phone action click --code A1 --pos-type 2 --pos 100,200 --wait
pmock-cli rpa task add --template-id 9 --name daily --json @task.json --wait
```

等待结果会写进同一个 JSON 信封的 `data.wait`：

```json
{
  "taskId": 1,
  "wait": { "ok": true, "taskStatus": 2, "phoneStatus": 1, "attempts": 2, "waitedMs": 1200 }
}
```

- 开机成功 = `taskStatus=2` 且 `phoneStatus=1`；关机成功 = `phoneStatus=4`；
- 动作成功 = `taskStatus=2`，失败 = `3`（部分后端也用 `-1`）；
- RPA 任务成功 = `status=2`，失败 = `3/4`；
- 超时退出码为 3，任务失败退出码为 1，信封仍输出；
- `--wait-timeout` 默认 3 分钟，`--poll-interval` 默认 2 秒。

## 诊断

```bash
pmock-cli doctor
```

检查配置文件权限、AK/SK 是否存在、base URL 是否合法、一次轻量 API 调用的延迟与 traceId。
`data.ok=false` 时：缺凭证退出 2，网络/鉴权失败退出 3。

## MCP Server

`pmock-cli mcp` 以 stdio 方式启动 MCP Server，复用同一份命令目录：

```json
{
  "mcpServers": {
    "pmock": {
      "command": "pmock-cli",
      "args": ["mcp"],
      "env": {
        "PMOCK_ACCESS_KEY": "AK...",
        "PMOCK_SECRET_KEY": "SK..."
      }
    }
  }
}
```

- 12 个分组 tool：`pmock_phone`、`pmock_phone_action`、`pmock_phone_file`、`pmock_app`、`pmock_network`、`pmock_rpa_template`、`pmock_rpa_task`、`pmock_product`、`pmock_brand`、`pmock_team`、`pmock_task`、`pmock_account`
- 63 个 action，覆盖 CLI 的全部可见命令
- `pmock_help` 工具返回任意命令的完整 flag 列表
- 调用方式：`action` 选动作，`params` 里的 key 就是去掉 `--` 的 flag
- 嵌套请求体放在 `params.json` 中：
  ```json
  { "action": "add", "params": { "template-id": 9, "name": "daily", "json": { "taskList": [{ "phoneId": 123 }] } } }
  ```
- 破坏性 action 需要 `params.yes = true`；失败结果带 `isError` 并附命令帮助
- MCP 与 CLI 共用同一 `spec.Catalog`，不会出现 geelark-cli 那种“CLI 有、MCP 没有”的漂移

## 复杂请求体

标量字段都有独立 flag；嵌套对象 / 对象数组用 `--json`，flag 会覆盖 JSON 中的同名 key：

```bash
pmock-cli phone create --spu-id 1 --sku-id 2 --num 1 \
  --json '{"tagIds":[1,2],"realResolution":true}'

# 从文件读取
pmock-cli rpa task add --template-id 9 --name daily --json @task.json

# 从 stdin 读取
cat task.json | pmock-cli rpa task add --template-id 9 --name daily --json -
```

先看请求、不发送：

```bash
pmock-cli rpa task add --template-id 9 --name daily --json @task.json --dry-run
```

---

## 输出与退出码

默认 stdout 是**单个 JSON 信封**，诊断信息一律走 stderr：

```json
{
  "traceId": "...",
  "code": 0,
  "msg": "success",
  "data": {},
  "status": "finish"
}
```

`code = 0` 和 `code = 200` 都表示成功（测试环境返回 200）；其他 code 为失败。

分页接口（`phone/list`、`app/list`、`rpa/task/list` 等）后端把 `total`/`rows`
放在顶层，CLI 统一折叠为：

```json
{ "data": { "total": 6, "rows": [ ... ] } }
```

`password`、`adbPassword`、`accessKey`、`token` 等字段在输出前自动替换为 `****`。

`--format`：

- `json`（默认）：原样信封，可 `| jq`
- `pretty`：人类可读摘要
- `table`：列表类响应渲染为表格（列序固定、中文按显示宽度对齐）

退出码：

| 码 | 含义 |
| --- | --- |
| 0 | 成功 |
| 1 | 业务失败（API `code` 不是 0/200，信封仍输出到 stdout） |
| 2 | 用法 / 配置错误（未发请求） |
| 3 | 运行时失败（网络、超时、5xx、AK/SK 被拒） |

`--allow-business-error` 可让业务失败也返回 0，适合只关心 HTTP 可达性的场景。

---

## 设计：spec 驱动，而不是手写 60 个命令

```text
openapi/pmock-openapi.yaml   （唯一事实源）
openapi/overlay.yaml         （命令名、别名、风险、示例、隐藏标记）
        │
        ▼
internal/spec  ── 解析 / 校验 / 生成 flags
        │
        ▼
internal/client ── AK/SK 签名、重试、信封解析
        │
        ▼
cmd            ── Cobra 命令树（运行时生成）
```

为什么不照抄 geelark-cli 的“一命令一文件”：

1. **不会漂移**：命令树、参数、help、MCP action 都来自同一份 spec；
   `internal/spec/catalog_test.go` 强制 60 个 operation 与 overlay 一一对应。
2. **新增接口只改两处**：spec 更新 + overlay 加一条映射，`go test` 会提示漏项。
3. **没有 2,700 行复制粘贴**：请求编码、类型转换、必填校验只有一份实现。
4. **保留人工控制**：命令命名、风险级别、示例由 overlay 决定，不是机械推导。

`overlay.yaml` 中 `internal: true` 的 operation 仍然可调用，但不出现在 help 里；
`phone file upload` 就是基于 `upload-init` / `upload-push` 封装的多步命令。

---

## 发布

本地生成跨平台产物（不依赖 GoReleaser）：

```bash
VERSION=0.1.0 make package
```

输出在 `dist/`：

```text
pmock-cli_0.1.0_darwin_arm64.tar.gz
pmock-cli_0.1.0_darwin_amd64.tar.gz
pmock-cli_0.1.0_linux_amd64.tar.gz
pmock-cli_0.1.0_linux_arm64.tar.gz
pmock-cli_0.1.0_windows_amd64.zip
checksums.txt
```

正式发布走 GitHub Actions：

```bash
git tag v0.1.0
git push origin v0.1.0
```

[.github/workflows/release.yml](.github/workflows/release.yml) 会调用 GoReleaser 生成同样的产物并创建 GitHub Release。

npm 包（平台包 + wrapper）：

```bash
VERSION=0.1.2 make npm
```

发布前在仓库 Secrets 中配置 `NPM_TOKEN`；发布 GitHub Release 时
[.github/workflows/npm.yml](.github/workflows/npm.yml) 会自动构建并发布 npm 包。
也可以本地手动发布。

产物在 `npm/build/`，包含 `pmock-cli` wrapper 和 5 个平台包：
`pmock-cli-darwin-arm64`、`pmock-cli-darwin-x64`、`pmock-cli-linux-x64`、
`pmock-cli-linux-arm64`、`pmock-cli-win32-x64`。

> Homebrew formula 需要正式仓库 URL 与 release checksum，仓库地址确认后生成。

## 开发

```bash
make test        # go test -race ./...
make vet
make fmt         # 检查 gofmt
make build
make snapshot    # GoReleaser 本地快照（需安装 goreleaser）
```

测试不访问真实 API：

- `auth`：签名固定向量
- `client`：`httptest` 校验签名头、查询编码、重试、业务错误
- `spec`：60 个 operation / overlay 覆盖 / 关键命令 flag
- `cmd`：请求组装、`--json` 合并、必填校验、`--wait`、MCP 工具/参数、doctor
- `phonefile`：MD5、分片、MinIO/OBS mergeList、断点续传
- `output`：JSON 转义、表格、中文宽度

真实 API 冒烟（需要有效 AK/SK，建议只读）：

```bash
export PMOCK_ACCESS_KEY=...
export PMOCK_SECRET_KEY=...
pmock-cli auth status
pmock-cli phone list --page-size 1
```

---

## 安全约定

- 配置文件 `0600`，目录 `0700`；不要把 AK/SK 提交到仓库或写进 shell history。
- CI 用 `PMOCK_ACCESS_KEY` / `PMOCK_SECRET_KEY` 注入；`--access-key` 会出现在 `ps` 中。
- `remove` / `reset` / `new` / RPA 删除类命令要求 `--yes`；`create` / `renew` 会打印花钱警告。
- CLI 不提供任意 shell、任意后端路径代理、凭证回显。
- 建议对生产团队使用独立的只读或低权限密钥（若后端支持）。

---

## Roadmap

- **P1**：批量封装（批量开关机/安装）、按名称或 code 唯一匹配、字段级 `--json` schema 提示、
  真实 AK/SK 只读冒烟
- **P2**：RPA 任务的 add/update/retry 高层封装、文件下载、推流地址、远程连接
- **P3**：Skills 文档生成、Docker 镜像、Windows npm 包审核跟进
