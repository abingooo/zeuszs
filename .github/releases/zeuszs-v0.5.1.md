## 更新内容

- 组织事件现已作为“组织”类别整合到“使用日志”，不再占用独立的侧边栏入口。
- 精简组织日志界面，仅保留日期和日志类别筛选，并移除请求 ID、发起人等不必要的技术细节。
- 保留服务端完整组织审计记录、角色范围控制和 ID 脱敏，不影响现有审计安全性。
- 发布流程现在强制校验每个稳定版本的说明文件，并将说明自动写入 GitHub Release。

## 升级说明

- 本版本不包含数据库结构变更。
- 可由平台管理员在“平台更新”页面检查并手动更新。

## English

- Organization events now appear as the **Organization** category inside **Usage Logs**, with no separate sidebar entry.
- The organization-log UI is intentionally concise: date and category filters remain, while request IDs and other low-level details are omitted.
- Full server-side audit records, role scoping, and ID redaction remain unchanged.
- Stable releases now require a non-empty release-note file, which is published automatically to GitHub Releases.
