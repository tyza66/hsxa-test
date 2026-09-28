// Package fofa 把仓库里的 FOFA Skill 接入 eino：知识库索引、Skill 脚本桥接与
// 链式编排都在这里完成。查询语句的生成一律复用 Python 侧已通过 selftest 的实现，
// Go 侧不做第二套判据，避免两套逻辑各自漂移。
package fofa

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// FixedAnswer 与 skill/scripts/lookup.py 的 FIXED_ANSWER 保持一致：
// 需求无法转换时唯一允许写进查询语句的固定串。
const FixedAnswer = "该需求不能直接转换为FOFA搜索语句"

// 答案来源标记，用于日志与结构化输出里区分一条答案是从链路哪一环产出的。
const (
	SourceMechanical = "mechanical" // Skill 机械层直接转换（确定性路径）
	SourceModel      = "model"      // 模型基于知识判据生成（需要模型凭据）
	SourceDeclined   = "declined"   // 机械层不处理且模型不可用
)

// declinedPrefix 是 convert.py 单句模式拒绝转换时打印的前缀，输出契约见其 docstring。
const declinedPrefix = "机械层不处理: "

// lintFailedPrefix 是 convert.py 转换成功但语法体检未通过时的输出前缀。
const lintFailedPrefix = "语法体检未通过: "

// Bundle 汇总一次装载的知识库与脚本桥接，cmd 与 engine 共用同一份实例。
type Bundle struct {
	dir       string
	knowledge *Knowledge
	skill     *Skill
	logger    *slog.Logger
}

// Skill 返回脚本桥接实例。
func (b *Bundle) Skill() *Skill { return b.skill }

// Knowledge 返回知识库索引实例。
func (b *Bundle) Knowledge() *Knowledge { return b.knowledge }

// Skill 是 skill/scripts 目录的进程桥：查询转换与语法体检都调 Python 实现。
type Skill struct {
	python    string        // Python 可执行文件
	scriptDir string        // <repo>/skill/scripts
	workDir   string        // 子进程工作目录，取仓库根，供 paths.py 向上定位工作区
	timeout   time.Duration // 单次脚本调用超时
	logger    *slog.Logger
}

// LoadOptions 描述一次 Skill 装载。
type LoadOptions struct {
	Dir     string        // 仓库根，留空则自动定位（对应 FOFA_SKILL_DIR）
	Python  string        // Python 可执行文件，默认 python3
	Timeout time.Duration // 单次脚本超时，默认 30s
	TopK    int           // 知识检索默认条数，默认 6
	Probe   bool          // 是否预先探测 Python，把失败前移到启动期
	Logger  *slog.Logger
}

// Load 定位 Skill 目录，装配知识库索引与脚本桥接。
func Load(opts LoadOptions) (*Bundle, error) {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	dir := strings.TrimSpace(opts.Dir)
	if dir == "" {
		resolved, err := resolveSkillDir()
		if err != nil {
			return nil, err
		}
		dir = resolved
	}
	python := strings.TrimSpace(opts.Python)
	if python == "" {
		python = "python3"
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	topK := opts.TopK
	if topK <= 0 {
		topK = 6
	}

	skillDir := filepath.Join(dir, "skill")
	if !fileExists(filepath.Join(skillDir, "SKILL.md")) {
		return nil, fmt.Errorf("fofa: %s 下找不到 skill/SKILL.md", dir)
	}

	bundle := &Bundle{
		dir: dir,
		skill: &Skill{
			python:    python,
			scriptDir: filepath.Join(skillDir, "scripts"),
			workDir:   dir,
			timeout:   timeout,
			logger:    logger,
		},
		logger: logger,
	}

	if opts.Probe {
		if err := bundle.skill.probe(); err != nil {
			return nil, err
		}
	}

	kb, err := LoadKnowledge(skillDir, topK)
	if err != nil {
		return nil, err
	}
	bundle.knowledge = kb

	logger.InfoContext(context.Background(), "fofa: Skill 已装载",
		"skill_dir", skillDir,
		"python", python,
		"知识块", len(kb.chunks),
		"top_k", topK,
	)
	return bundle, nil
}

// resolveSkillDir 在常见起点向上查找仓库根（含 skill/SKILL.md 与 docs/题目.txt）。
func resolveSkillDir() (string, error) {
	var starts []string
	if cwd, err := os.Getwd(); err == nil {
		starts = append(starts, cwd)
	}
	if exe, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(exe))
	}
	for _, start := range starts {
		if found := findUp(start); found != "" {
			return found, nil
		}
	}
	return "", errors.New("fofa: 定位不到仓库根（含 skill/SKILL.md 与 docs/题目.txt），" +
		"请用 FOFA_SKILL_DIR 显式指定")
}

// findUp 从 start 向上逐级寻找仓库根。
func findUp(start string) string {
	current := start
	for {
		if fileExists(filepath.Join(current, "skill", "SKILL.md")) &&
			fileExists(filepath.Join(current, "docs", "题目.txt")) {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

// fileExists 报告路径是否存在且不是目录。
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// probe 确认 Python 可用，把"临调用才失败"前移到启动期。
func (s *Skill) probe() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, s.python, "-V").CombinedOutput()
	if err != nil {
		return fmt.Errorf("fofa: Python 不可用（%q %v）: %s",
			s.python, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// runScript 执行 scripts 下的一个 Python 脚本，返回 stdout 与退出码。
// stderr 只进日志：脚本的诊断信息对定位问题有用，但不该污染调用方。
func (s *Skill) runScript(ctx context.Context, name string, args ...string) (string, int, error) {
	if s == nil {
		return "", -1, errors.New("fofa: Skill 桥接未初始化")
	}
	if ctx.Err() != nil {
		return "", -1, ctx.Err()
	}

	scriptPath := filepath.Join(s.scriptDir, name)
	if !fileExists(scriptPath) {
		return "", -1, fmt.Errorf("fofa: 脚本不存在: %s", scriptPath)
	}

	// 超时挂在 ctx 上，由 exec 负责杀掉子进程，避免悬挂的 Python 拖住请求。
	callCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	argv := append([]string{scriptPath}, args...)
	cmd := exec.CommandContext(callCtx, s.python, argv...)
	cmd.Dir = s.workDir
	cmd.Env = os.Environ()

	start := time.Now()
	stdout, err := cmd.Output()
	elapsed := time.Since(start)
	// 退出码语义：进程正常结束（含 exit 0）时 err 为 nil；只有被信号杀掉、
	// 找不到可执行文件这类情况 err 才不是 ExitError。默认 -1 会把成功的调用
	// 也报成"异常退出"，调用方根本分不清脚本是成了还是挂了。
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return "", -1, fmt.Errorf("fofa: 执行 %s 失败: %w", name, err)
		}
		exitCode = exitErr.ExitCode()
	}

	s.logger.DebugContext(ctx, "fofa: 调用 Skill 脚本",
		"script", name,
		"args", len(args),
		"exit_code", exitCode,
		"duration_ms", elapsed.Milliseconds(),
	)

	return string(stdout), exitCode, nil
}

// Conversion 是机械层一次尝试的结构化结果。
type Conversion struct {
	Query      string // 机械层产出的查询语句，拒绝时为空
	Confidence string // 置信度：H 高 / M 中 / L 低
	Note       string // 机械层给出的判定说明
	OK         bool   // 是否完成了转换（固定答案也算转换成功）
}

// Convert 走 Skill 机械层把一句自然语言需求转成查询语句。
//
// 转不了不是错误：convert.py 以退出码 1 加"机械层不处理"前缀表示拒绝，
// 此时返回 OK=false 与说明，由链路决定是交给模型还是直接放弃。
func (s *Skill) Convert(ctx context.Context, text string) (Conversion, error) {
	stdout, exitCode, err := s.runScript(ctx, "convert.py", text)
	if err != nil {
		return Conversion{}, err
	}

	lines := splitLines(stdout)
	if exitCode == 0 {
		conv := Conversion{OK: true}
		for _, line := range lines {
			if conv.Query == "" {
				conv.Query = strings.TrimSpace(line)
				continue
			}
			if conf, note, ok := parseConfidenceLine(line); ok {
				conv.Confidence = conf
				conv.Note = note
			}
		}
		if conv.Query == "" {
			return Conversion{}, fmt.Errorf("fofa: convert.py 退出码 0 但没有输出查询语句: %q", stdout)
		}
		return conv, nil
	}

	for _, line := range lines {
		if note, ok := strings.CutPrefix(line, declinedPrefix); ok {
			return Conversion{Note: strings.TrimSpace(note)}, nil
		}
		if strings.HasPrefix(line, lintFailedPrefix) {
			// 转换成功却体检不过属于链路异常，不能静默当成"拒绝"。
			return Conversion{}, fmt.Errorf("fofa: convert.py 自带语法体检未通过: %s",
				strings.TrimSpace(strings.TrimPrefix(line, lintFailedPrefix)))
		}
	}
	return Conversion{}, fmt.Errorf("fofa: convert.py 异常退出（exit=%d）: %s", exitCode, strings.TrimSpace(stdout))
}

// Lint 对一条候选查询语句跑 Skill 的语法体检，返回的问题列表为空表示通过。
func (s *Skill) Lint(ctx context.Context, query string) ([]string, error) {
	stdout, exitCode, err := s.runScript(ctx, "lookup.py", "lint", query)
	if err != nil {
		return nil, err
	}

	var issues []string
	for _, line := range splitLines(stdout) {
		if issue, ok := strings.CutPrefix(line, "问题: "); ok {
			issues = append(issues, strings.TrimSpace(issue))
		}
	}
	switch exitCode {
	case 0:
		return nil, nil
	case 1:
		if len(issues) == 0 {
			return nil, fmt.Errorf("fofa: lookup.py 报告未通过但未列出问题: %q", stdout)
		}
		return issues, nil
	default:
		return nil, fmt.Errorf("fofa: lookup.py 异常退出（exit=%d）: %s", exitCode, strings.TrimSpace(stdout))
	}
}

// parseConfidenceLine 解析 convert.py 的"置信度: X  说明: Y"行。
func parseConfidenceLine(line string) (confidence, note string, ok bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(line), "置信度: ")
	if !ok {
		return "", "", false
	}
	head, tail, found := strings.Cut(rest, "说明: ")
	if !found {
		return strings.TrimSpace(rest), "", true
	}
	return strings.TrimSpace(head), strings.TrimSpace(tail), true
}

// splitLines 按行切分并去掉首尾空行，兼容 CRLF。
func splitLines(s string) []string {
	raw := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
