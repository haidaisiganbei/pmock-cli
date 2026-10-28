# pmock-cli

PMOCK 开放 API 的独立命令行客户端。用团队 **Access Key / Secret Key** 直接调用
`https://user.pmock.com/api/open/api/*`，不依赖 PMOCK 桌面客户端、不依赖登录态，可运行在
CI、服务器和第三方开发者机器上。

> 与随客户端安装的 `pmock` 命令区分：`pmock` 走本机 MCP / 登录态，只服务本机；
> `pmock-cli` 走开放 API + AK/SK，是可独立分发的二进制。

- 语言：Go 1.24+
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
| `phone file upload` 一体化上传 | ⏳ P1 |
| `--wait` 异步任务轮询 | ⏳ P1 |
| MCP Server / Skills | ⏳ P3 |
| npm / Homebrew / GoReleaser 发布 | ⏳ P3 |

---

## 安装

从源码：

```bash
git clone <repo-url> pmock-cli
cd pmock-cli
make build
./bin/pmock-cli --version
```

或：

```bash
go install github.com/pmock/pmock-cli@latest
```

> `module` 路径目前是占位值 `github.com/pmock/pmock-cli`，确定正式仓库后修改
> `go.mod`、`Makefile` 与 `.goreleaser.yaml` 中的模块路径即可。

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

# 开关机（返回 taskId；轮询用 phone task-status）
pmock-cli phone start --id 123
pmock-cli phone stop --id 123
pmock-cli phone task-status --phone-id 123 --operate-type powerOn

# 看屏与 UI 自动化（动作类用 code，不是 id）
pmock-cli phone screen --id 123
pmock-cli phone action click --code A1 --pos-type 2 --pos 100,200
pmock-cli phone action input --code A1 --text "hello"

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

`--format`：

- `json`（默认）：原样信封，可 `| jq`
- `pretty`：人类可读摘要
- `table`：列表类响应渲染为表格（列序固定、中文按显示宽度对齐）

退出码：

| 码 | 含义 |
| --- | --- |
| 0 | 成功 |
| 1 | 业务失败（API `code != 0`，信封仍输出到 stdout） |
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

1. **不会漂移**：命令树、参数、help、将来的 MCP action 都来自同一份 spec；
   `internal/spec/catalog_test.go` 强制 60 个 operation 与 overlay 一一对应。
2. **新增接口只改两处**：spec 更新 + overlay 加一条映射，`go test` 会提示漏项。
3. **没有 2,700 行复制粘贴**：请求编码、类型转换、必填校验只有一份实现。
4. **保留人工控制**：命令命名、风险级别、示例由 overlay 决定，不是机械推导。

`overlay.yaml` 中 `internal: true` 的 operation 仍然可调用，但不出现在 help 里，
用于后续封装 `phone file upload` 这类多步流程。

---

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
- `cmd`：请求组装、`--json` 合并、必填校验
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

- **P1**：`phone file upload` 一体化（init → 分片 PUT → completeAndPush）、`--wait` 轮询、
  批量封装、字段级 `--json` schema 提示
- **P2**：RPA 任务的 add/update/retry 高层封装、文件下载、推流地址、远程连接
- **P3**：`pmock-cli mcp`（复用同一 catalog 生成 action）、Skills 文档生成、
  GoReleaser + npm wrapper + Homebrew + Docker
