## 更新内容

- 修复站内平台更新在启用 systemd 文件系统保护时无法创建 `/root/.docker` 导致更新失败的问题。
- Docker 客户端配置现在固定使用更新器可写的 `/var/lib/zeuszs-updater/docker-config` 目录，不需要放宽服务沙箱权限。

## 升级说明

- 本版本不包含数据库结构变更。
- 已安装更新器的主机请重新加载 `zeuszs-updater.service`；使用仓库提供的服务单元即可自动使用新的 Docker 配置目录。

## English

- Fixes platform updates failing to create `/root/.docker` when the updater runs with systemd filesystem protection enabled.
- Docker client configuration now uses the updater-writable `/var/lib/zeuszs-updater/docker-config` directory without weakening the service sandbox.

## Upgrade Notes

- This release contains no database schema changes.
- Hosts with an installed updater should reload `zeuszs-updater.service`; the supplied unit file automatically uses the new Docker client configuration directory.
