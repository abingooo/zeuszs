## 更新内容

- API 密钥“复制连接信息”、CC Switch 导入链接和模型详情代码示例现在优先使用 API 信息列表中的首个有效 API 地址。
- 保留未配置 API 地址时对 `ServerAddress`、当前域名和示例地址的兼容回退逻辑，不修改站点全局 `ServerAddress`。
- API 信息面板的端点测速颜色调整为：低于 1 秒绿色、1 至 3 秒橙色、3 秒及以上红色。
- 增加 API 地址解析和测速阈值边界测试。

## 升级说明

- 本版本不包含数据库结构变更。
- 可由平台管理员在“平台更新”页面检查并手动更新。

## English

- API-key connection info, CC Switch import links, and model-detail code samples now prefer the first valid address configured in the API information list.
- Preserves the `ServerAddress`, current-origin, and example-URL fallbacks when no API address is configured; the global site `ServerAddress` is unchanged.
- Updates endpoint latency colors to green below 1 second, orange from 1 to under 3 seconds, and red at 3 seconds or above.
- Adds boundary tests for API URL resolution and latency thresholds.

## Upgrade Notes

- This release contains no database schema changes.
- Platform administrators can detect and install this release manually from the **Platform Update** page.
