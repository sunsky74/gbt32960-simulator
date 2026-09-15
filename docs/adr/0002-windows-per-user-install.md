# Windows 安装范围切换为 per-user

状态:accepted(2026-09-14)

Windows 安装包改用 `-installscope user`(Wails v2.15 原生支持),安装到 `%LOCALAPPDATA%\Programs\...` 而非 `Program Files`。原因:一键自更新需要用 rename-aside 替换安装目录中的 exe,而 `Program Files` 需要管理员权限、替换必然失败;per-user 安装免 UAC,使 Windows 与 macOS/Linux 一样具备完整的应用内更新能力。

## Considered Options

- **保持 machine 范围,升级改走"下载 NSIS 安装包静默升级"**:需 UAC 提权、依赖安装器行为与杀软放行,体验割裂、失败面更大;弃。

## Consequences

- v0.1.0 时代以 machine 范围安装的少量旧装机无法应用内自更新,需手动重装一次(v0.1.0 发布仅数日,装机量极小,可接受)。
- 此后安装、升级、卸载路径统一以 per-user 为准。
