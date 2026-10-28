package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// 工具定义与分发：恰好 6 个工具（读4 + 写2），不做注册框架，switch-case 分发。
// 明确不实现：submit_followup（须人工确认）、delete_record、bulk_generate。

type toolDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"inputSchema"`
}

func toolDefs() []toolDef {
	obj := func(props map[string]any, required []string) map[string]any {
		return map[string]any{"type": "object", "properties": props, "required": required}
	}
	str := map[string]any{"type": "string"}
	return []toolDef{
		{
			Name:        "query_patient",
			Description: "查询患者档案（不含 id_card 等证件号）",
			InputSchema: obj(map[string]any{"patient_id": str}, []string{"patient_id"}),
		},
		{
			Name:        "list_tasks",
			Description: "查询随访任务列表，status 可选（created/scheduled/submitted/confirmed）",
			InputSchema: obj(map[string]any{"status": str}, nil),
		},
		{
			Name:        "query_history_trend",
			Description: "查询患者近 N 次随访记录的结构化关键值，limit 默认 3，无记录返回空数组",
			InputSchema: obj(map[string]any{
				"patient_id": str,
				"limit":      map[string]any{"type": "integer", "default": 3},
			}, []string{"patient_id"}),
		},
		{
			Name:        "get_skill_config",
			Description: "按病种返回 skill 配置（SKILL.md / schema.json / questions.yaml / rules.json）",
			InputSchema: obj(map[string]any{"disease_type": str}, []string{"disease_type"}),
		},
		{
			Name:        "save_followup",
			Description: "保存 AI 随访结果为草稿记录（status 恒为 draft、source 恒为 ai，提交须人工确认）",
			InputSchema: obj(map[string]any{
				"task_id":         str,
				"structured_data": map[string]any{"type": "object"},
				"ai_raw":          map[string]any{"type": "object"},
				"confidence":      map[string]any{"type": "number"},
			}, []string{"task_id", "structured_data"}),
		},
		{
			Name:        "push_alert",
			Description: "为任务推送预警告警（status 恒为 open；随访记录未生成时 record_id 为空）",
			InputSchema: obj(map[string]any{
				"task_id": str,
				"level":   str,
				"rule_id": str,
				"reason":  str,
			}, []string{"task_id", "level", "rule_id", "reason"}),
		},
	}
}

// execTool 工具调用入口；写工具额外落一条 audit_logs
func (s *Server) execTool(name string, args json.RawMessage) (any, error) {
	switch name {
	case "query_patient":
		return s.queryPatient(args)
	case "list_tasks":
		return s.listTasks(args)
	case "query_history_trend":
		return s.queryHistoryTrend(args)
	case "get_skill_config":
		return s.getSkillConfig(args)
	case "save_followup":
		return s.writeTool(name, args, s.saveFollowup)
	case "push_alert":
		return s.writeTool(name, args, s.pushAlert)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

// writeTool 写工具包装：执行成功后写一条 audit_logs（caller 身份 + 工具名 + 入参 + 结果）
func (s *Server) writeTool(name string, args json.RawMessage, fn func(json.RawMessage) (any, error)) (any, error) {
	result, err := fn(args)
	if err != nil {
		return nil, err
	}
	if auditErr := insertAuditLog(s.pool, s.caller, name, args, result); auditErr != nil {
		return nil, fmt.Errorf("audit log: %w", auditErr)
	}
	return result, nil
}

// getSkillConfig 读取 skill 目录下的四个配置文件
func (s *Server) getSkillConfig(args json.RawMessage) (any, error) {
	var p struct {
		DiseaseType string `json:"disease_type"`
	}
	if err := json.Unmarshal(args, &p); err != nil || p.DiseaseType == "" {
		return nil, fmt.Errorf("disease_type required")
	}

	dir := filepath.Join(s.cfg.SkillsRoot, p.DiseaseType)
	files := map[string]string{
		"skill_md":       "SKILL.md",
		"schema_json":    "schema.json",
		"questions_yaml": "questions.yaml",
		"rules_json":     "rules.json",
	}
	result := map[string]any{"disease_type": p.DiseaseType}
	for key, filename := range files {
		data, err := os.ReadFile(filepath.Join(dir, filename))
		if err != nil {
			return nil, fmt.Errorf("read skill config: %w", err)
		}
		result[key] = string(data)
	}
	return result, nil
}
