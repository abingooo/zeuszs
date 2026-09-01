## 更新内容

- 为宙斯智算首页和关于页增加品牌优先的搜索元数据、规范链接、社交分享信息，以及 Organization / WebSite 结构化数据。
- 为不执行 JavaScript 的搜索爬虫提供可读的首页与关于页内容，同时对登录、控制台、组织管理等非公开页面明确设置 `noindex`。
- 发布真实的 `robots.txt` 与 `sitemap.xml`，只列出官网首页和关于页，并加入 IndexNow 站点验证文件。
- 在公开仓库的中英文 README 中增加 `zeuszs.ai` 官方站点链接，帮助搜索引擎确认品牌与域名关系。

## 升级说明

- 本版本不包含数据库结构变更。
- 可由平台管理员在“平台更新”页面检查并手动更新。
- 部署后可向 IndexNow、Google Search Console、Bing Webmaster 和百度搜索资源平台提交站点与 sitemap。

## English

- Adds ZeusZS-first search metadata, canonical links, social previews, and Organization / WebSite structured data to the public home and About pages.
- Provides crawlable home and About content when JavaScript is unavailable, while marking sign-in, dashboard, organization management, and other private pages as `noindex`.
- Publishes real `robots.txt` and `sitemap.xml` resources containing only the public home and About pages, together with an IndexNow verification file.
- Adds an official `zeuszs.ai` link to the public English and Simplified Chinese READMEs to strengthen the brand-domain association.

## Upgrade Notes

- This release contains no database schema changes.
- Platform administrators can detect and install this release manually from the **Platform Update** page.
- After deployment, the site and sitemap can be submitted to IndexNow, Google Search Console, Bing Webmaster, and Baidu Search Resource Platform.
