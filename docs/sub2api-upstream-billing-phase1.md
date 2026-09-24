# Sub2API 上游成本：阶段一核对结论

状态：阶段一完成，可以进入阶段二。未修改业务代码、生产环境、数据库或远端仓库。

核对时间：2026-09-20。Sub2API 一侧为生产运行中的 PostgreSQL、容器价格目录和匿名
聚合账单；Codex2API 一侧为本机仓库的代码默认价。当前工作区没有运行数据库或管理员
价格覆盖文件，因此不把代码默认价冒充某个已部署实例的自定义价。

Astra 说明：Codex2API 的 Astra 长上下文是代码级计费例外，不是管理员没有打开的开关。
`database/billing.go` 默认规则没有 Astra 长档，`database/model_pricing_override.go`
还会在加载手工或同步价格时清除 Astra 的长档字段，前端也不提供 Astra 长档编辑项。

## 1. 已确定的目标

只针对 Codex2API 内配置为 Sub2API API Key 的上游账号：

```text
upstream_cost = sub2_cost_rule(实际 usage) * 探针 effective_rate_multiplier
```

- 倍率只取该账号调用 `GET /v1/sub2api/billing` 返回的
  `effective_rate_multiplier`；不读取、不保存、也不推断 Sub2API 分组倍率。
- `sub2_cost_rule` 移植 Sub2API 的 OpenAI 计价规则：模型、普通/fast/flex、缓存读写和
  长上下文。
- 新增的上游成本仅用于账号成本观察；保留 Codex2API 既有 `account_billed`、
  `user_billed`、额度和下游收费逻辑。
- 不迁移 Sub2API 分组、套餐、用户、分组定价管理或请求级回执功能。

Codex2API 已有余额展示和上游请求 ID；本方案不重做余额探针，也不把余额差作为单笔成本。

## 2. 两端价格核对

单位均为 USD / 1M token。下表是生产 Sub2API 的基础渠道/价格目录与 Codex2API 代码默认
价的对照；`gpt-5.6-sol` 行保留官方调价前的历史价格，当前生产与仓库均继续使用
`5 / 30 / 0.5`（standard 输入/输出/cache-read），本阶段不切换到新价。

| 模型 | 基础输入/输出/缓存读 | fast/priority | flex | 长上下文 | 结论 |
| --- | --- | --- | --- | --- | --- |
| gpt-5.4 | 2.5 / 15 / 0.25 | 5 / 30 / 0.5 | 1.25 / 7.5 / 生产约 0.13 | 272K 后 5 / 22.5 / 0.5 | 基本一致；Codex2API flex cache-read 为 0.125，存在很小差异 |
| gpt-5.5 | 5 / 30 / 0.5 | 12.5 / 75 / 1.25 | 2.5 / 15 / 0.25 | 272K 后 10 / 45 / 1 | 一致 |
| gpt-5.5-pro | 30 / 180 / 3 | 生产按通用 2x；Codex2API 为 2.5x | 15 / 90 / 1.5 | 272K 后 60 / 270 / 6 | 缓存读和 fast 不一致，不能复用 Codex2API 旧计算器 |
| gpt-5.6-sol | 5 / 30 / 0.5 | 10 / 60 / 1 | 2.5 / 15 / 0.25 | 272K 后输入/缓存侧 2x，输出 1.5x | 保留官方调价前历史价 |
| gpt-5.6-terra | 2 / 12 / 0.2 | 4 / 24 / 0.4 | 1 / 6 / 0.1 | 272K 后输入/缓存侧 2x，输出 1.5x | token 价格一致 |
| gpt-5.6-luna | 0.2 / 1.2 / 0.02 | 0.4 / 2.4 / 0.04 | 0.1 / 0.6 / 0.01 | 272K 后输入/缓存侧 2x，输出 1.5x | token 价格一致 |
| gpt-6-astra | 10 / 50 / 1 | 20 / 100 / 2 | 5 / 25 / 0.5 | 生产实扣为 20 / 75 / 2 | Codex2API 当前完全不启用 Astra 长档，必须修正 |

生产渠道设置的 OpenAI `cache_write_price` 和 `cache_write_1h_price` 如下；生产当前所有
这些 OpenAI 模型的 `cache_write_1h_price` 都为空，所以 Sub2API 两档沿用同一个
`cache_write_price`。这不是“5 分钟和 1 小时各有一套价格但漏配”，而是当前 Sub2API
对 OpenAI 的生效策略。

| 模型 | Sub2API 5m / 1h | Codex2API 现有函数 5m / 1h | 差异 |
| --- | --- | --- | --- |
| gpt-5.4 | 2.5 / 2.5 | 3.125 / 5 | 两档都高估 |
| gpt-5.5 | 5 / 5 | 6.25 / 10 | 两档都高估 |
| gpt-5.6-sol | 6.25 / 6.25 | 6.25 / 10 | 5m 与历史官方价一致；Codex2API 的 1h 通用公式仍高估 |
| gpt-5.6-terra | 2.5 / 2.5 | 2.5 / 4 | 1h 高估 |
| gpt-5.6-luna | 0.25 / 0.25 | 0.25 / 0.4 | 1h 高估 |
| gpt-6-astra | 12.5 / 12.5 | 12.5 / 20 | 1h 高估 |

以上均为 USD / 1M token，且未包含长上下文倍率。长上下文时 Sub2API 会对 cache-write
再乘输入侧长上下文倍率；因此这些差异会继续被放大。生产账单已出现 `gpt-5.6-sol`
和 `gpt-5.6-terra` 的非零 cache-creation token，不能把它视为理论边界。

Sub2API 的 OpenAI 模型目录对 `gpt-5.6-sol/terra/luna` 只定义了
`cache_creation_input_token_cost`、standard/priority/flex 变体，没有
`cache_creation_input_token_cost_above_1hr`；该类 1 小时专用字段只出现在有明确 TTL
价差的其他 Provider 条目中。因此，生产端 OpenAI 的 5m=1h 与当前 Sub2API 模型目录
是一致的，但不能据此宣称这是 OpenAI 公共价目表另行公布的 5m/1h 差异。

## 3. 必须移植的机制

不能直接调用 Codex2API 的 `database.CalculateCostBreakdown`，原因不是基础价格普遍错误，
而是它的计算口径与 Sub2API 不同：

1. Codex2API 仅以 `input_tokens >= 272K` 进入长上下文；Sub2API 使用
   `input + cache_creation + cache_read`。
2. Codex2API 对 Astra 强制不进入长上下文；Sub2API 生产账单中可反推出长档为
   input `$20/M`、output `$75/M`、cache-read `$2/M`。
3. Codex2API 对 `gpt-5.5-pro` 未配置缓存读价，实际会按普通 input 处理；Sub2API 为
   3，长上下文为 6。
4. Codex2API 对 `gpt-5.5-pro` priority 写为 2.5x；生产 Sub2API 的规则为 2x。
5. OpenAI cache-write 不能套用 Codex2API 的 Anthropic 默认 1.25x/2x；必须使用上表的
   Sub2API 单价，并在长上下文时再应用输入侧倍率。

`gpt-5.6-sol/terra/luna` 的长档价格本身与 Codex2API 代码基本一致：阈值 272K，输入
   和 cache-read 2x，输出 1.5x；但触发条件仍不同。Sub2API 判断
   `input + cache_creation + cache_read > 272000`，Codex2API 只判断
   `input >= 272000`。因此恰好 272000 或缓存 token 把总上下文推过阈值时，结果会不同。

因此阶段二应复制 Sub2API 的 OpenAI token 成本计算规则为一个独立的 Codex2API 上游
成本计算器。它不改动、也不复用现有官方成本计算器。

按已确认的范围，不移植 Sub2API 的分组价格覆盖或
`long_context_pricing_enabled` 分组开关；所有已识别的 Sub2API 上游账号统一采用官方
模型规则，再乘其探针倍率。

## 4. 阶段二最小实现范围

1. 新增独立 `Sub2UpstreamCost` 计算器和价格表，处理 standard、fast/priority、flex、
   cache-read、OpenAI 单一 cache-write 价格、272K 长上下文和 Astra；OpenAI 不套用
   Codex2API 现有的 1h=输入价×2 规则。
2. 复用已有账户探针，读取并缓存该 API Key 的 `effective_rate_multiplier`。
3. 在 Codex2API 已取得最终 usage 后，计算并写入：

   ```text
   upstream_cost
   upstream_effective_multiplier
   upstream_billing_observed_at
   ```

4. 在 usage 明细和汇总页增加“上游成本”；不改现有下游收费字段和统计含义。
5. 为上述价格矩阵、长上下文阈值、缓存、倍率和落库路径补充定向测试。

预计改动约 350-650 行，涉及后端计费/usage 落库、迁移、管理查询、前端 usage 展示和
测试；不需要改 Sub2API，也不需要改生产端。

## 5. 阶段二前的验收边界

完成后使用测试上游 API Key 做以下校准：普通请求、fast、flex、272K 前后、cache-read、
cache-write 和 Astra。比较 Codex2API 展示的 `upstream_cost` 与 Sub2API 的同一 Key
消费变化，用于发现模型表更新或 usage 字段不一致。

探针失败不阻断请求：该条 usage 的上游成本标为不可用，不写 0 或伪造 1x。管理界面不
展示 API Key、Authorization 或完整探针响应。

## 6. 阶段一结论

生产端与 Codex2API 的大部分基础 OpenAI 价格一致，但现有 Codex2API 计算器在四类场景
存在实际差异：长上下文触发、Astra、`gpt-5.5-pro` 和 cache-write。

这不是进入阶段二的阻碍，反而明确了阶段二的正确实现方式：新增并调用独立的
Sub2API 规则计算器，最终只乘该上游 API Key 探针返回的
`effective_rate_multiplier`。阶段二可以开始；Sol 固定使用官方调价前的历史价格，不切换到
当前新价。若未来需要跟随官方再次调价，再单独发起价格表更新和校准。
