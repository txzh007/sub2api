# 国模定价热更新

TToken 默认从远程模型目录同步价格，但国内模型发布和改名速度较快，远程目录可能暂时没有对应条目。未命中价格的 token 请求无法正确计费，因此生产环境应同时维护本地价格覆盖文件。

默认路径：

```yaml
pricing:
  override_file: "./data/model_pricing_overrides.json"
  hash_check_interval_minutes: 10
```

`data` 目录在 Docker 部署中是持久化卷。覆盖文件新增或修改后，会在下一次检查时自动加载，无需重新构建镜像或重启服务。

## 文件格式

价格单位与 LiteLLM 目录一致，均为 **美元/Token**。例如供应商报价为每百万 Token 2 美元，应填写 `2e-6`。

```json
{
  "qwen3-coder-*": {
    "litellm_provider": "dashscope",
    "mode": "chat",
    "input_cost_per_token": 0.000002,
    "output_cost_per_token": 0.000008,
    "cache_read_input_token_cost": 0.0000002
  },
  "doubao-seed-2.0-pro": {
    "litellm_provider": "volcengine",
    "mode": "chat",
    "input_cost_per_token": 0.000001,
    "output_cost_per_token": 0.000004
  }
}
```

上面的数字仅用于说明格式，生产价格必须按供应商当前官方价填写。

## 匹配规则

- 精确模型名优先，例如 `doubao-seed-2.0-pro`。
- 以 `*` 结尾表示前缀规则，例如 `qwen3-*` 会匹配后续日期版或上下文版模型。
- 多个前缀规则同时命中时，最长前缀优先。例如 `qwen3-coder-*` 优先于 `qwen3-*`。
- 裸 `*` 不生效，避免把图片、视频、语音等非 Token 模型误按文本价格计费。
- 覆盖文件优先于远程目录和仓库内置回退价，可用于新增模型，也可修正已有模型字段。

## 生产保护

渠道中可以开启“限制模型”，只允许定价列表中的模型通过。这样新模型尚未配价时会被拒绝，而不是产生免费调用。需要快速接入一批同价模型时，可在渠道定价或本文件中使用前缀规则。

每个未定价模型在单个进程内只会输出一次明确告警：

```text
[Billing] No pricing available for model "..."; token usage will otherwise be recorded at $0
```

看到该告警后，应添加精确价格或合适的前缀规则，并确认实际模型返回了 token usage。
