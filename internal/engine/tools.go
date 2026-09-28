package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	_ "time/tzdata" // 让容器内没有系统 zoneinfo 时也能解析 IANA 时区名

	"github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"

	"github.com/tyza66/hsxa-test/internal/fofa"
)

// currentTimeInput 是 current_time 工具的入参 schema，
// jsonschema 标签会让 eino 自动生成给模型用的 JSON Schema。
type currentTimeInput struct {
	Timezone string `json:"timezone" jsonschema:"description=IANA timezone name, e.g. Asia/Shanghai. Defaults to UTC."`
}

// timeReport 是工具的返回值，eino 会把它序列化成 JSON 字符串回传给模型。
type timeReport struct {
	Timezone string `json:"timezone"`
	DateTime string `json:"datetime"`
	Date     string `json:"date"`
	Weekday  string `json:"weekday"`
	Unix     int64  `json:"unix"`
}

// newTools 返回引擎暴露给模型的工具集合。
// bundle 非 nil 时追加 FOFA 的 Skill 工具，nil 表示该链路未装载。
func newTools(bundle *fofa.Bundle) ([]tool.BaseTool, error) {
	currentTime, err := newCurrentTimeTool()
	if err != nil {
		return nil, err
	}
	tools := []tool.BaseTool{currentTime}
	if bundle != nil {
		fofaTools, err := fofa.NewTools(bundle)
		if err != nil {
			return nil, fmt.Errorf("engine: 装载 FOFA 工具失败: %w", err)
		}
		tools = append(tools, fofaTools...)
	}
	return tools, nil
}

// newCurrentTimeTool 返回一个返回当前时间的工具。
//
// 模型本身对"今天几号""现在几点"这类问题经常答错，把时间能力交给工具是
// 最低成本的补救，也顺便演示了 eino 自定义工具的最小写法。
func newCurrentTimeTool() (tool.InvokableTool, error) {
	t, err := toolutils.InferTool(
		"current_time",
		"Return the current date and time for a given IANA timezone. "+
			"Use this tool whenever the answer depends on \"now\", \"today\" or a deadline.",
		func(ctx context.Context, in *currentTimeInput) (*timeReport, error) {
			if in == nil {
				in = &currentTimeInput{}
			}

			name := strings.TrimSpace(in.Timezone)
			if name == "" {
				name = "UTC"
			}
			loc, err := time.LoadLocation(name)
			if err != nil {
				return nil, fmt.Errorf("unknown timezone %q: %w", name, err)
			}

			now := time.Now().In(loc)
			return &timeReport{
				Timezone: loc.String(),
				DateTime: now.Format(time.RFC3339),
				Date:     now.Format(time.DateOnly),
				Weekday:  now.Weekday().String(),
				Unix:     now.Unix(),
			}, nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("engine: 构建 current_time 工具失败: %w", err)
	}
	return t, nil
}

// toolInfos 从工具集合中提取喂给模型的 schema.ToolInfo。
func toolInfos(tools []tool.BaseTool) ([]*schema.ToolInfo, error) {
	infos := make([]*schema.ToolInfo, 0, len(tools))
	for _, t := range tools {
		info, err := t.Info(context.Background())
		if err != nil {
			return nil, fmt.Errorf("engine: 读取工具信息失败: %w", err)
		}
		infos = append(infos, info)
	}
	return infos, nil
}
