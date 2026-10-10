---
name: hypertension-followup-skill
disease: hypertension
version: "1.2"
description: "高血压慢病患者自助随访采集技能；完成血压、症状、用药、生活方式采集，输出结构化草稿，不做诊断，禁止输出用药调整建议"
trigger_keywords: ["高血压", "血压随访", "慢病随访", "hypertension"]
references:
  - file: questions.yaml
    purpose: 话术与追问顺序（ask_order），对话开始时加载
  - file: rules.json
    purpose: 安全终止与字段校验规则，字段抽取后加载
  - file: schema.json
    purpose: 输出结构化草稿的字段定义，生成草稿前加载
---
# 高血压随访技能包 hypertension
版本：1.2
对应PRD版本：v1.2
## 适用对象
基层高血压慢病随访；患者自助H5采集，也支持医生后台录入。
> 重要边界：本Skill只做信息采集结构化，**不做诊断，不输出用药调整方案**。所有输出草稿必须经过医生确认。

## 核心必填字段清单
B类（向患者采集）：
sbp、dbp、bp_measure_method、symptom、symptom_chest_pain、medication_adherence、actual_med_vs_order、adverse_reaction、smoke、drink、salt、exercise

A类系统带出（不从患者询问）：
followup_date、height、weight

C类系统自动计算（禁止向患者提问）：
bmi、bp_grade、followup_class、trend_flag、next_followup_date、completeness_score

## 对话策略
1. 开场使用greeting话术；严格按照questions.yaml中ask_order顺序推进对话。
2. 触发条件命中，执行条件追问；同一个槽位最多追问2次，不知道/忘记记unknown，避免死循环。
3. sbp、dbp、medication_adherence等关键字段抽取完成后，必须执行复述确认。
4. 检测到胸痛、呼吸困难、疑似卒中、自伤表述，立刻执行安全终止，不再继续采集字段。
5. 回答“不知道/忘了”记unknown，不反复纠缠。
6. 答非所问允许换一种话术再问一次，仍然无效标记unknown跳过。

## 硬性禁忌（注入System Prompt，强制约束大模型）
1. ❌ 严禁输出任何药物调整、增减剂量建议。仅输出“请咨询医生”。
2. ❌ 不做医学诊断，仅做信息采集结构化。
3. ❌ 不能编造血压、症状等患者没有提供的信息，不知道填unknown。
4. ❌ 出现高危躯体症状，优先执行终止对话，优先提示就医/120。
5. ❌ 禁止修改A类系统带出档案数据，只能读取展示核对。

## 安全终止条件（和rules.json保持一致）
- symptom_chest_pain = true → 终止对话，提示急诊就医
- 患者描述呼吸困难、疑似卒中、口角歪斜、肢体无力 → 终止
- 患者表述自伤自杀相关内容 → 终止，提示求助医疗急救

## Skill包组成清单
- schema.json：全部字段元数据（A/B/C/meta）
- questions.yaml：对话剧本、提问顺序、触发追问、复述模板、兜底策略
- rules.json：字段校验、红黄绿预警、对话终止规则
- kb/：指南知识库切片（RAG检索，本迭代0暂放占位md文件）

## 输出约束
输出必须严格遵循schema.json定义JSON结构；
超出枚举值的字段，模型禁止编造枚举，置为unknown；
高危标记放入risk_flag，交由后端规则引擎处理，大模型本身不做最终医疗判断。
