# Gemini 生图兼容与二开升级

本功能基于当前 Sub2API 分支增加两条链路：

- OpenAI Images 请求 → 独立生图上游，默认 OpenAI 协议；也可使用 Gemini 原生 `generateContent` 适配。
- OpenAI 分组的 Codex HTTP Responses 请求 → 原有文本模型选择图片工具 → 网关调用独立生图接口 → 返回 `image_generation_call` 图片结果。

第二条由网关执行图片工具，不需要客户端执行名为 `sub2api_generate_image` 的函数。它不改变 Codex 账号本身具备的权限，也不能控制客户端在本地单独执行的插件工具。

## 创建密钥时选择生图模型

在「API 密钥 → 创建密钥 / 编辑」中，主分组选择 **OpenAI 平台**的文字模型分组，例如 GPT - Pro，再在「生图桥接模型」中选择图片模型。仅 OpenAI 分组显示并启用桥接，Gemini、Claude、Grok、Composite 等分组不启用；切换到非 OpenAI 分组时清除原选择，后端也拒绝为非 OpenAI 分组保存桥接模型。保存后，OpenAI Images 请求和 Codex HTTP Responses 的图片工具使用这个选择，文字请求仍由主分组处理。

模型列表来自名称为 **生图** 的分组，要求该分组启用、允许图片生成，且当前用户有权使用。候选项取自分组内可调度的 OpenAI、Gemini、Grok 账号的生图模型映射；启用了分组模型列表配置时，再取交集。不会根据主分组或全局价格表补充候选项。账号映射应包含具体模型名；只有通配符的映射不会展开为候选模型。

生图分组使用 Composite 平台且多个平台暴露同名模型时，需配置 `images` 端点的明确路由。使用 OpenAI 兼容生图上游时，将 `gemini-3.1-flash-image` 等模型路由到 OpenAI 平台；只有原生 Gemini 上游才路由到 Gemini，协议由接口配置决定，不由模型前缀决定。

- **选择模型**：生图请求的 `model` 会替换为所选模型，因此客户端仍传 `gpt-image-2` 即可。Codex 的文字 `model` 保持原值，图片工具使用所选模型。
- **不桥接**：该 Key 不执行这项内部工具桥接，Images 按主分组原有处理逻辑转发。
- **跟随服务默认配置**：保留旧 Key 的全局配置和显式工具模型行为，见下文。

图片使用生图分组的账号、价格、用户分组倍率和订阅额度；文字使用主分组对应配置。二者仍计入同一 Key 的额度和用量。每次桥接都会重新检查生图分组权限和模型列表，禁用分组或撤销权限后不会继续使用旧授权。

管理 API：`GET /api/v1/keys/image-bridge-models` 返回 `{group_id, group_name, models}`，创建与编辑 Key 的请求增加 `image_bridge_model`。模型字符串表示指定模型，`""` 表示不桥接，`null` 表示跟随默认；编辑时省略此字段保留原选择。新建页面默认不桥接，旧 Key 迁移后为 `null`。

## 独立生图接口管理

后台「生图接口」（`/admin/image-providers`）独立维护生图上游的名称、Base URL、API Key、模型映射、并发和启停状态，复用已有账号存储与调度，不额外增加数据库表。新增接口仅绑定“生图”分组。编辑时密钥留空保留原值，模型及地址可以单独调整。

列表支持「停用」和「删除」。删除需确认，调用原有账号删除接口，删除对应账号及全部分组关联；它不只是从“生图”分组解绑。如果所选模型没有其他可用接口承接，原 Key 的生图调用将无法继续。临时暂停使用可选择停用。

新增时填写地址和密钥，点击「从远端获取模型」后在多选列表中勾选；编辑时自动使用服务端保存的密钥获取。模型以已选标签展示，可直接移除；默认按名称筛出生图相关模型，支持搜索、取消筛选查看全部和勾选添加。远端模型名称不代表已验证支持生图，具体能力以上游为准。获取本身不修改配置，保留已有映射，点击保存后才生效；「高级设置」默认收起，需要时可展开手动填写 `别名=上游模型`。读取失败会显示错误，不用本地配置冒充远端结果。

管理接口 `POST /api/v1/admin/accounts/models/image-preview` 接收 `platform`、`base_url`、`api_key`，编辑时可传 `account_id` 复用原密钥，返回 `{models: [...]}`。它复用现有 OpenAI / Gemini 模型列表请求、URL 校验和代理设置，只读取模型 ID，不同步账号能力快照或写入模型映射。

新增接口默认 **OpenAI** 协议，访问上游 `/v1/images/generations`、`/v1/images/edits`。OpenAI 兼容接口可以直接使用 Gemini 图片模型名，无需增加 `gpt` 前缀的代理别名。选择 Gemini 协议时才执行原生 Gemini 转换；上游本身仍需支持所选模型及协议。接口协议创建后保持不变，切换协议可新建对应接口。

首次使用需在分组管理中启用“生图”分组并允许图片生成。只接 OpenAI 协议时分组使用 OpenAI 平台；需要混合协议时使用 Composite 并配置模型路由。分组模型列表已启用时，新增模型还需在该列表中开放，才能成为 Key 的桥接候选项。

## OpenAI Images 接入

推荐先在 Key 上选择生图桥接模型。也兼容原有方式：使用 Gemini 分组的 Key，或同时包含相应 Gemini 账号的 Composite 分组 Key，开启该分组的「允许图片生成」，确认账号可调度且模型映射指向实际可用的 Gemini 生图模型。Composite 分组可以配置 `images` 端点的模型路由；自定义别名也可通过该路由映射。

生成示例：

```bash
curl "$SUB2API_BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"model":"gemini-3.1-flash-image","prompt":"画一只坐在窗边的橘猫","size":"1024x1024","response_format":"b64_json"}'
```

编辑示例：

```bash
curl "$SUB2API_BASE_URL/v1/images/edits" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -F 'model=gemini-3.1-flash-image' \
  -F 'prompt=把背景改成蓝色，保留主体' \
  -F 'image=@reference.png'
```

也支持不带 `/v1` 的 `/images/generations` 和 `/images/edits`。JSON 编辑使用 `images: [{"image_url":"data:image/png;base64,..."}]`，多图上传使用多个 `image[]` 字段。参考图需携带实际图片字节；远程 URL、客户端本地路径、OpenAI `file_id` 不会被网关自动读取。

以下是 **Gemini 原生适配**的参数范围；OpenAI 协议接口按原有 Images 转发处理，具体支持以上游为准：

| 参数 | 行为 |
| --- | --- |
| `model` | Gemini 模型名或分组可解析的别名；Gemini 分组省略时默认 `gemini-3.1-flash-image`。Composite 分组应明确传模型，以供入口路由。 |
| `n` | 仅支持 `1`；多张图分别请求。 |
| `size` | `auto`、`1K`、`2K`、`4K`，或宽高，如 `1536x1024`。转换为 Gemini 比例与分辨率档位，实际像素由上游决定；模型支持的比例和档位仍以上游为准。 |
| `response_format` | 默认 `b64_json`；`url` 返回 **data URL**，不创建公网图片地址。 |
| `stream` | 支持 SSE，等待期间发送注释保活，最终发送完整图片事件；不提供渐进预览图。 |
| `output_format` | 当前仅接受省略或 `png`，但不强制转码。实际编码以上游返回的 `mime_type` 为准；实测部分上游返回 JPEG，应按真实格式保存。 |
| `quality` | 省略或 `auto`；其他值明确报错，分辨率使用 `size` 指定。 |
| `mask`、透明背景、非零 `partial_images` | 暂不支持，返回参数错误。 |

调用复用原有 Gemini 网关的调度、内容审查、限流和用量记录，同时共享现有图片并发限制。Gemini 返回纯文本或拒绝生成时，Images 接口返回错误，不把该结果兜底算成一张图片；上游已消耗的 token 仍按现有规则记录。

大型图片还受现有 `gateway.upstream_response_read_max_bytes` 限制（示例配置为 8 MiB）；按实际需要调整。桥接内部响应另外有 64 MiB 上限。现有 `/images/.../async` 任务接口未加入这条 Gemini 链路。

## Codex 内部图片工具由 Gemini 执行

推荐保留原来的 GPT 编程模型与主分组，直接为 Key 选择 Gemini 生图桥接模型，不需要修改全局配置或重启。

全局默认仅对选择「跟随服务默认配置」的 **OpenAI 分组 Key** 生效，图片同样从独立“生图”分组选择。在实际加载的配置中增加：

```yaml
gateway:
  codex_gemini_image_model: "gemini-3.1-flash-image"
```

也可向服务进程传入环境变量 `GATEWAY_CODEX_GEMINI_IMAGE_MODEL`。Docker Compose 的 `.env` 不会自动把任意变量传入容器，需要在服务的 `environment` 中声明，或挂载修改后的 `config.yaml`。修改后重启服务。

配置为空且 Key 没有指定模型时，不自动改写普通 Codex 请求，原有 `codex_image_generation_bridge_enabled` 逻辑保持原样。OpenAI Key 显式传入 `tools: [{"type":"image_generation","model":"gemini-..."}]` 仍可从“生图”分组选择 Gemini 模型；这是本网关对 OpenAI 工具参数的扩展。非 OpenAI 分组的 Responses 按原有协议处理。

这版桥接覆盖 POST `/v1/responses`、`/responses`、`/backend-api/codex/responses`。客户端需使用 HTTP Responses；例如在用户级 Codex 配置中为自定义 provider 设置：

```toml
model = "gpt-5.4"
model_provider = "sub2api"

[model_providers.sub2api]
name = "Sub2API"
base_url = "https://你的网关地址/v1"
wire_api = "responses"
env_key = "SUB2API_API_KEY"
supports_websockets = false
```

启动 Codex 的环境中提供 `SUB2API_API_KEY`，模型名换成分组实际支持的编程模型。已使用真实上游验证 Images 文生图、参考图编辑和 `gpt-5.5` 的 HTTP Responses/SSE 内部生图链路；实际 Codex App 的界面集成仍需单独验证。

开启后的行为与边界：

- 网关将客户端的 `image_generation` 或 `image_gen` 工具声明替换为内部图片函数。没有声明工具的 Codex 请求也会增加该函数，由文本模型决定是否调用。
- 只有实际调用图片工具时才检查生图接口的可用性；生图接口不可用不会阻止普通文字和其他工具请求。
- 生图调用成功后返回原生形状的 `image_generation_call`，内部函数调用不会交给客户端执行。通常消耗一次文本决策请求、一次 Gemini 生图请求、一次文本续写请求；各调用使用独立计费请求 ID，复用现有计费。响应 `usage` 合并两次文本调用，Gemini 用量单独记录在服务端。
- 文本续写失败时仍交付已生成的图片。`tool_choice: {"type":"image_generation"}` 强制生图时直接使用输入提示词，不执行文本决策与续写。
- 当前先聚合内部请求，再发送 Responses 事件；因此开启后文本也不是逐 token 实时返回。普通客户端函数调用仍通过 Responses 工具事件交付。
- 连续对话需要发送完整历史，包括图片结果。网关把自己的历史图片转换成视觉输入。暂不支持 `previous_response_id` 和 `background:true`，也不提供合成响应 ID 的服务端检索。
- WebSocket、Responses 的 `/compact` 等子路径不执行本桥接。客户端本地 `image_gen` 插件若通过其他连接单独请求，不受这项配置控制。
- 本地文件作为参考图时，需要客户端先上传为图片输入，网关不会按提示中的路径读取文件。

协议参考：[OpenAI 图片工具](https://platform.openai.com/docs/guides/tools-image-generation)、[Gemini 图片生成](https://ai.google.dev/gemini-api/docs/image-generation)、[Codex 配置参考](https://developers.openai.com/codex/config-reference)。

## 后续升级 Sub2API

二开代码保留在 `feat/gemini-image-bridge` 分支，起点为 `ab99d56e9`（仓库版本 `0.2.1`）。**升级方式是合并新版本源码、验证、重新构建自己的镜像或二进制。直接换回发布方镜像/二进制不会包含本功能。**

先将本功能及后续修改提交到自己的仓库；不要长期仅保留工作区未提交差异。`upstream` 应指向当前这份代码实际跟随的发行来源：如果基于某个 Sub2API fork，先跟随该 fork，避免无意间切换产品分支。下面假设已配置该 remote，标签名需替换为实际目标版本：

```bash
git switch feat/gemini-image-bridge
git status --short
# 确认修改已提交、工作区干净后，保存升级前分支
git branch backup/gemini-before-upgrade
git fetch upstream --tags
git merge <目标版本的标签或提交>
```

备份分支名每次升级换一个。解决冲突时重点核对以下接入点，不要直接整文件选择任意一侧：

| 现有文件 | 需要保留的接入 |
| --- | --- |
| `backend/internal/server/routes/gateway.go` | Gemini Images 分发、共享图片限流器注入、三个 Responses 路径包装。 |
| `backend/internal/server/routes/user.go`、`backend/internal/handler/api_key_handler.go` | 生图候选模型接口及 Key 创建、编辑字段。 |
| `backend/internal/service/api_key*`、`backend/internal/repository/api_key_repo.go`、DTO、`wire.go` | 每个 Key 的桥接选择、认证缓存字段、源分组权限检查和服务依赖。 |
| `backend/ent/schema/api_key.go`、迁移 `235_api_key_image_bridge_model.sql` | 可空的 `image_bridge_model` 字段；合并 schema 后重新生成 Ent 与 Wire。 |
| `frontend/src/views/user/KeysView.vue`、`components/keys/ImageBridgeModelSelect.vue`、API、类型和翻译 | 创建、编辑选择器及已选模型显示。 |
| `frontend/src/views/admin/ImageProvidersView.vue`、前端路由与侧栏 | 独立生图接口管理，默认 OpenAI 协议。 |
| `backend/internal/service/openai_images.go` | OpenAI Images 校验允许 Gemini 等图片模型名。 |
| `backend/internal/handler/gateway_handler.go` | 图片网关字段。 |
| `backend/internal/config/config.go`、`deploy/config.example.yaml` | `codex_gemini_image_model` 配置及空默认值。 |
| `backend/internal/service/openai_gateway_service.go` | 内部执行 Gemini 工具时禁止重复注入 OpenAI 生图工具。 |
| `backend/internal/service/gemini_image_output_accounting.go` | Images 桥接请求使用实际输出图数，禁止空图按模型名兜底计费。 |

核心实现与回归用例放在新增的 `gemini_images*`、`codex_gemini_images.go`、`external_image_tool.go` 文件中。即使 Git 没有报告冲突，也要验证新上游是否更改了 Gemini 原生处理、工具注入、Responses 事件、调度和计费接口。

在 `backend` 目录使用项目要求的 Go 版本执行：

```bash
go test -tags=unit ./internal/handler ./internal/service ./internal/server/routes -run 'GeminiImages|GeminiImage|APIKey.*(ImageBridge|Auth|Cache|Update)|CodexImageGenerationBridge|OpenAIImages|GatewayRoutes|CompositeTarget' -count=1
go test ./internal/config
CGO_ENABLED=0 go build -o /tmp/sub2api-gemini-check ./cmd/server
```

然后按目标上游版本要求执行其完整发布检查，使用测试 Key 验证生成、参考图编辑、普通文本、Codex 工具调用和用量记录，再从仓库根目录构建自有镜像：

```bash
docker build -t sub2api:gemini-local .
```

部署配置使用自己的镜像标签，保留升级前的镜像。按 Key 选择生图模型增加了迁移 `235_api_key_image_bridge_model.sql`，升级前需备份数据库和持久化数据。迁移只新增可空列，旧 Key 默认不指定桥接模型；当前仅 OpenAI 分组执行内部桥接。未来合并上游时检查迁移编号冲突和字段变化，不要修改已经执行过的迁移。回滚旧二进制后，按 Key 桥接功能不再生效。

现有插件宿主主要开放出站传输能力（见 `PLUGIN_DEVELOPMENT.md`），尚不能独立承载本功能的路由、协议转换和内部工具执行。当前保留少量宿主接入更直接；未来上游开放完整插件接口或原生支持同等功能后，可以移除对应补丁。
