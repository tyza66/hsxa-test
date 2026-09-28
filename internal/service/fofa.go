package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tyza66/hsxa-test/internal/fofa"
)

// ErrEmptyQuestion 是 FOFA 请求缺少自然语言需求时返回的错误。
var ErrEmptyQuestion = errors.New("问题不能为空")

// FofaRequest 是 FOFA 转换接口的请求体。
//
// 字段名与 Answer 的输出字段保持一致（中文），请求与响应对读时不用来回翻译。
type FofaRequest struct {
	Question string `json:"问题"`
}

// FofaService 对外提供"自然语言需求 -> 结构化 FOFA 答案"的能力。
type FofaService struct {
	chain *fofa.Chain
}

// NewFofaService 创建 FOFA 服务。
func NewFofaService(chain *fofa.Chain) *FofaService {
	return &FofaService{chain: chain}
}

// Convert 跑完整条链路：知识库索引、机械层优先、模型兜底、语法体检收口。
func (s *FofaService) Convert(ctx context.Context, req *FofaRequest) (*fofa.Answer, error) {
	if req == nil || strings.TrimSpace(req.Question) == "" {
		return nil, ErrEmptyQuestion
	}

	answer, err := s.chain.Run(ctx, req.Question)
	if err != nil {
		return nil, fmt.Errorf("FOFA 链路执行失败: %w", err)
	}
	return answer, nil
}
