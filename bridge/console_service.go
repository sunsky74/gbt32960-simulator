package bridge

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"gbt32960-simulator/internal/engine"
)

// ConsoleService 控制台导出与清空。事件镜像存于 Forwarder 的环形缓冲,
// 导出在 Go 侧完成,避免大数组过 IPC。
type ConsoleService struct {
	fwd *Forwarder
	ctx context.Context
}

// NewConsoleService 创建服务。ctx 由 app.startup 经 WireContexts 注入(原生对话框需要)。
func NewConsoleService(fwd *Forwarder) *ConsoleService {
	return &ConsoleService{fwd: fwd}
}

// ExportResult 导出结果。
type ExportResult struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
}

// ExportConsole 弹原生保存对话框并按格式导出。kinds 为空导出全部。
func (s *ConsoleService) ExportConsole(format string, kinds []string) (*ExportResult, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "csv"
	}
	events := s.fwd.Snapshot(kinds)
	if len(events) == 0 {
		return nil, fmt.Errorf("没有可导出的事件")
	}

	ext := map[string]string{"csv": "csv", "log": "log", "json": "json"}[format]
	if ext == "" {
		return nil, fmt.Errorf("不支持的格式: %s (可选 csv/log/json)", format)
	}

	path, err := runtime.SaveFileDialog(s.ctx, runtime.SaveDialogOptions{
		Title:           "导出控制台事件",
		DefaultFilename: fmt.Sprintf("gbt32960-console-%s.%s", time.Now().Format("20060102-150405"), ext),
		Filters: []runtime.FileFilter{
			{DisplayName: strings.ToUpper(format) + " 文件", Pattern: "*." + ext},
		},
	})
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, nil // 用户取消
	}

	var data []byte
	switch format {
	case "csv":
		data, err = formatEventsCSV(events)
	case "json":
		data, err = formatEventsJSON(events)
	default:
		data, err = formatEventsLog(events)
	}
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nil, err
	}
	return &ExportResult{Path: path, Count: len(events)}, nil
}

// ClearConsole 清空 Go 侧事件镜像。
func (s *ConsoleService) ClearConsole() {
	s.fwd.Clear()
}

// GetParamSpecs 返回指定协议版本的参数定义表(2016 表B.12 / 2025 表B.8),
// 供控制台的 0x80 参数查询应答按 ID 定长、定值域渲染与校验。
func (s *ConsoleService) GetParamSpecs(version string) []engine.ParamSpec {
	return engine.ParamSpecs(parseVersion(version))
}

// GetResponseCodes 返回指定协议版本的应答码列表(2016/2025 表4),
// 供控制台的 0x80/0x81/0x82 应答按版本选择标准应答码。
func (s *ConsoleService) GetResponseCodes(version string) []engine.ResponseCode {
	return engine.ResponseCodes(parseVersion(version))
}

func formatEventsCSV(events []engine.Event) ([]byte, error) {
	var sb strings.Builder
	w := csv.NewWriter(&sb)
	if err := w.Write([]string{"时间", "类型", "命令", "字节数", "HEX", "消息", "解码JSON"}); err != nil {
		return nil, err
	}
	for _, e := range events {
		decoded := ""
		if e.Decoded != nil {
			if b, err := json.Marshal(e.Decoded); err == nil {
				decoded = string(b)
			}
		}
		if err := w.Write([]string{
			e.Time.Format("2006-01-02 15:04:05.000"),
			string(e.Kind),
			e.Cmd,
			fmt.Sprint(e.Bytes),
			e.Hex,
			e.Message,
			decoded,
		}); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return []byte("\xEF\xBB\xBF" + sb.String()), nil // BOM 便于 Excel 打开中文表头
}

func formatEventsLog(events []engine.Event) ([]byte, error) {
	var sb strings.Builder
	for _, e := range events {
		sb.WriteString(fmt.Sprintf("%s [%s] %s %s\n",
			e.Time.Format("15:04:05.000"), e.Kind, e.Cmd, e.Message))
		if e.Hex != "" {
			sb.WriteString("  HEX: " + e.Hex + "\n")
		}
		if e.Decoded != nil {
			if b, err := json.Marshal(e.Decoded); err == nil {
				sb.WriteString("  DECODED: " + string(b) + "\n")
			}
		}
	}
	return []byte(sb.String()), nil
}

func formatEventsJSON(events []engine.Event) ([]byte, error) {
	return json.MarshalIndent(events, "", "  ")
}
