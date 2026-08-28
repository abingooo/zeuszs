## 更新内容

- 重新设计默认公开首页，采用可切换深浅主题的轨道地球视觉，并将首页收敛为单屏沉浸式体验。
- 使用原版透明 ZeusZS Logo 与清晰的中英文品牌文字，移除自绘 Logo 和自绘艺术字。
- 引入高清昼夜地球与云层纹理；桌面端支持 WebGL 动态场景，移动端、节流设备和不支持 WebGL 的环境自动使用静态海报。
- 增加场景性能分级、按需加载、页面不可见时暂停渲染、WebGL 上下文丢失回退和减少动态效果支持。
- 优化沉浸式导航栏、主题切换与移动端布局，并仅保留简体中文和英文界面选项。
- 默认首页不再显示统计、功能介绍、使用流程、行动区和页脚；管理员配置的自定义首页 URL、HTML 或 Markdown 不受影响。

## 升级说明

- 本版本不包含数据库结构变更。
- 地球素材及相关第三方许可证声明已随源码和发布产物提供。
- 可由平台管理员在“平台更新”页面检查并手动更新。

## English

- Redesigns the default public home page as a single-screen orbital Earth experience with coordinated light and dark themes.
- Restores the original transparent ZeusZS logo and uses clean localized typography instead of custom-drawn logo or wordmark geometry.
- Adds high-resolution day, night, and cloud textures, with an interactive WebGL scene on capable desktops and static posters on mobile, constrained, or unsupported devices.
- Adds adaptive scene performance, lazy loading, render suspension while hidden, WebGL context-loss fallback, and reduced-motion support.
- Refines the immersive header, theme controls, and mobile layout while limiting interface choices to Simplified Chinese and English.
- Removes the lower default-home sections and footer; administrator-configured URL, HTML, and Markdown home pages remain unchanged.

## Upgrade Notes

- This release contains no database schema changes.
- Earth asset attribution and third-party license notices are included with the source and release artifacts.
- Platform administrators can detect and install this release manually from the **Platform Update** page.
