<!--
Release notes read this file from the custom branch (codex2api-custom).
Upstream sync keeps this file. Edit the sections below when maintained
behavior changes; do not put these notes only in README.md, because main
tracks upstream's README.
-->

## Custom maintenance

- **Upstream multiplier discovery:** Accounts connected to Sub2API can opt into multiplier discovery. Manual probing and periodic probing are supported, with strict response validation and the last valid value retained for temporary probe failures.
- **Multiplier-aware billing:** Normal accounts use the official model price. When a valid upstream multiplier is available, user billing and upstream cost estimation keep the official base price and apply the multiplier separately. The account, API key and usage views expose the multiplier and the related cost details.
- **Channel health monitoring:** Channel availability, probe status, response time and recent failures can be checked from the administration console without waiting for a user request.
- **Mobile usage details:** Usage cost details, tooltips and account information remain readable and usable on small screens as well as desktop screens.
- **Pricing coverage:** The maintained pricing mapping covers current upstream model aliases, standard and long-context boundaries, and the fallback path used when a model is not returned by an upstream pricing probe.

## 自定义维护内容

- **上游倍率探查：** 接入 Sub2API 的账号可以选择开启倍率探查，支持手动探查和定时探查，并对响应协议进行严格校验。临时探查失败时保留最后一次有效倍率。
- **倍率计费：** 普通账号按照官方模型价格计算。探查到有效上游倍率后，用户费用和上游成本估算以官方基础价格为依据，再单独应用倍率。账号、API Key 和使用统计页面会显示倍率及对应费用信息。
- **渠道健康监控：** 管理后台可以主动检查渠道可用性、探查状态、响应时间和近期失败记录，不需要等到用户请求发生后才发现问题。
- **移动端用量展示：** 费用明细、提示信息和账号信息在小屏幕上保持清晰可读，移动端点击提示也能正常使用。
- **模型价格覆盖：** 维护版本补齐当前上游模型别名、标准价格、长上下文边界，以及上游价格探查不可用时的回退路径。
