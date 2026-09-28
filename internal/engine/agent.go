package engine

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
)

// newAgent 构建一个带工具的 ReAct 智能体。
func (e *Engine) newAgent(ctx context.Context) (*react.Agent, error) {
	agent, err := react.NewAgent(ctx, &react.AgentConfig{
		// 智能体内部会基于 ToolsConfig 自行调用 WithTools 绑定工具，
		// 因此这里传未绑定的原始模型即可。
		ToolCallingModel: e.base,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: e.tools,
		},
		MessageModifier: react.NewPersonaModifier(e.cfg.Agent.Persona),
		MaxStep:         e.cfg.Agent.MaxStep,
		GraphName:       "hsxa-react-agent",
	})
	if err != nil {
		return nil, fmt.Errorf("engine: 构建 ReAct 智能体失败: %w", err)
	}
	return agent, nil
}
