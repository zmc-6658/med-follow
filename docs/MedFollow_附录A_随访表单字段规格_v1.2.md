# 附录A：随访表单字段规格 v1.2

> 本文档定义 MedFollow 系统各病种随访表单的完整字段规格，包括 A/B/C 分类、取值范围、触发式追问规则及预警阈值。
> 本文档是 schema.json、Pydantic 模型、数据库建表、前端表单渲染、规则引擎校验的唯一真相来源（Source of Truth）。
>
> **V1.2 变更**：修正 sbp/dbp 分类（原稿误标为A类，实为B类）；补充数据保留期分层说明；补充患者撤回同意与删除权说明。

---

## 一、字段分类体系

### A类 — 系统带出，不问患者

此类字段由系统从 HIS/健康档案/历史随访中自动带出，随访对话中不向患者提问，仅做展示与核对。

| 字段键 | 字段名 | 类型 | 来源 | 备注 |
|---|---|---|---|---|
| followup_date | 随访日期 | date | 系统当前日期 | 默认今天，可手动修改 |
| patient_id | 患者ID | string | HIS主索引 | 系统内部使用 |
| patient_name_masked | 患者姓名（脱敏） | string | HIS | 展示用，如"张*三" |
| gender | 性别 | enum | HIS | male/female |
| age | 年龄 | int | HIS | |
| height | 身高 | float(cm) | 健康档案 | 只读展示 |
| weight | 体重 | float(kg) | 患者自报/档案 | 可选，用于算BMI |
| diagnose_date | 确诊日期 | date | HIS | 只读展示 |
| disease_duration | 病程 | int(年) | 计算得出 | 当前年-确诊年 |
| last_bp | 上次血压 | string | 上次随访记录 | 用于趋势比对 |
| last_fbg | 上次空腹血糖 | float | 上次随访记录 | 用于趋势比对 |
| current_meds | 当前用药方案 | array | HIS医嘱 | 作为"核对基准" |
| allergy_history | 过敏史 | string | 健康档案 | 只读展示 |
| complication_history | 并发症史 | string | 健康档案 | 影响追问逻辑 |
| doctor_name | 管床医生 | string | 档案 | 用于任务分派 |
| clinic_name | 所属卫生室 | string | 档案 | |
| followup_round | 随访轮次 | int | 系统计数 | 年内第几次 |
| followup_channel | 随访方式 | enum | 系统 | sms/phone/face/wechat/self |
| followup_target | 随访对象 | enum | 患者选择 | 本人/家属/监护人/知情人 |
| proxy | 是否代填 | bool | 自动标记 | followup_target!=本人 时=true |

### B类 — 必须问（Agent 的主战场）

此类字段需要通过对话向患者采集，是槽位填充 Agent 的核心目标。

#### 通用B类字段（所有病种共有）

| 字段键 | 字段名 | 类型 | 必填 | 追问逻辑（触发条件 → 追加问题） |
|---|---|---|---|---|
| symptoms | 症状 | multi-select | 是 | 多选：头痛头晕/恶心呕吐/眼花耳鸣/呼吸困难/心悸胸闷/鼻衄/四肢发麻/下肢水肿/其他。**任选≥1项 → 追问"持续多久了？有没有加重？"** |
| sbp | 收缩压 | int(70-250) | 是 | >180 → 立即追问症状+是否已就医；<90 → 追问头晕/乏力 |
| dbp | 舒张压 | int(40-150) | 是 | 同上；若 sbp<=dbp → 标记"数值可疑" |
| bp_measure_method | 测量方式 | enum | 是 | 自测/卫生室测/医院测/未测（未测 → 提醒去测） |
| med_adherence | 服药依从性 | enum | 是 | 规律/间断/不服药。**间断或不服药 → 追问原因**（忘记/费用高/副作用/觉得自己好了/买不到药），原因进独立字段 reason_not_take |
| med_side_effect | 药物不良反应 | bool | 是 | true → 追问具体表现（干咳/脚踝水肿/低血糖/皮疹/头晕等） |
| med_compliance_check | 实际用药vs医嘱 | compare | 是 | 不一致 → 标记"自行调药"，高危 |
| smoke_status | 吸烟 | enum | 是 | 否/已戒/吸烟（支数/天） |
| alcohol_status | 饮酒 | enum | 是 | 否/偶尔/经常（两/天） |
| exercise_days | 运动天数 | int(周) | 选填 | 每周运动天数 |
| exercise_minutes | 运动时长 | int(分钟) | 选填 | 每次运动分钟数 |
| salt_intake | 摄盐情况 | enum | 是 | 清淡/一般/偏咸/很重 |
| mental_status | 心理状态 | enum | 是 | 良好/一般/焦虑/抑郁（筛出焦虑→追问睡眠） |
| sleep_quality | 睡眠质量 | enum | 选填 | 良好/一般/差/失眠（mental_status=焦虑/抑郁时触发） |
| other_complaints | 其他不适 | text | 否 | 开放文本，Agent 尝试结构化 |

#### 高血压病种特有B类字段

| 字段键 | 字段名 | 类型 | 必填 | 追问逻辑 |
|---|---|---|---|---|
| leg_edema | 下肢水肿 | enum | 是 | 无/轻度/中度/重度/未注意 |
| limb_numbness | 四肢发麻 | bool | 是 | true → 追问部位和持续时间 |
| nosebleed | 鼻衄 | bool | 是 | true → 追问频率和量 |
| vision_problem | 眼花/视力模糊 | bool | 是 | true → 追问持续时间 |
| chest_pain | 胸痛 | bool | 是 | true → 立即触发红级预警+终止对话 |
| breathing_difficulty | 呼吸困难 | bool | 是 | true → 立即触发红级预警+终止对话 |

#### 糖尿病病种特有B类字段

| 字段键 | 字段名 | 类型 | 必填 | 追问逻辑 |
|---|---|---|---|---|
| fbg | 空腹血糖 | float(2-30) | 是 | >16.7 或 <3.9 → 危急；追问有无心慌出汗手抖 |
| pbg_2h | 餐后2h血糖 | float(2-30) | 选填 | 同上 |
| hba1c | 糖化血红蛋白 | float(4-15) | 选填 | >9% 或 <5% 标记异常 |
| hypo_reaction | 低血糖反应 | enum | 是 | 无/偶尔/频繁。**频繁 → 追问最近一次发生时间、处理方式** |
| feet_pulse | 足背动脉搏动 | enum | 是 | 正常/减弱/消失/未查 |
| foot_ulcer | 足部破溃 | bool | 是 | true → 立即触发红级预警 |
| vision_blur | 视力模糊 | bool | 是 | true → 追问，提示并发症筛查 |
| hand_foot_numbness | 手足麻木 | bool | 是 | true → 追问持续时间 |
| staple_food_amount | 主食量 | int(克/天) | 选填 | 估算每日主食摄入量 |
| insulin_type | 胰岛素种类 | string | 选填 | 如"诺和灵N"，配合用量字段 |
| insulin_dose | 胰岛素用量 | string | 选填 | 如"12U/早餐前" |

#### 老年人(65+)病种特有B类字段

| 字段键 | 字段名 | 类型 | 必填 | 追问逻辑 |
|---|---|---|---|---|
| adl_eating | 进餐能力 | enum | 是 | 自理/需帮助/完全依赖（ADL五项之一） |
| adl_grooming | 梳洗能力 | enum | 是 | 同上 |
| adl_dressing | 穿衣能力 | enum | 是 | 同上 |
| adl_toileting | 如厕能力 | enum | 是 | 同上 |
| adl_mobilizing | 活动能力 | enum | 是 | 同上 |
| fall_history | 跌倒史 | enum | 是 | 无/近半年1次/近半年≥2次（≥2次 → 标记高危） |
| cognition_status | 认知功能 | enum | 是 | 正常/轻度障碍/中度障碍/重度障碍 |
| gds_score | GDS-15抑郁筛查得分 | int(0-15) | 选填 | ≥5分 → 标记抑郁倾向 |
| chinese_constitution | 中医体质辨识 | enum | 选填 | 平和/气虚/阳虚/阴虚/痰湿/湿热/血瘀/气郁/特禀 |
| vision_hearing | 视力听力 | string | 选填 | 描述现状 |
| waist_circumference | 腰围 | float(cm) | 选填 | |

#### 严重精神障碍病种特有B类字段（仅限家属/监护人填报）

| 字段键 | 字段名 | 类型 | 必填 | 追问逻辑 |
|---|---|---|---|---|
| danger_level | 危险性评估 | enum(0-5级) | 是 | 0级(无)~5级(正在实施)。≥3级 → 立即报精防医生 |
| symptom_hallucination | 幻觉 | bool | 是 | true → 追问类型(幻听/幻视等)和频率 |
| symptom_suspicion | 猜疑妄想 | bool | 是 | true → 追问内容 |
| symptom_violence | 伤人毁物 | bool | 是 | true → 追问次数和严重程度 |
| symptom_despair | 悲观厌世 | bool | 是 | true → 立即触发红级预警+自杀风险评估 |
| symptom_self_harm | 自伤自杀 | bool | 是 | true → 立即终止随访流程，提示拨打120/报警 |
| insight_level | 自知力 | enum | 是 | 完全/不全/缺失 |
| social_func_work | 社会功能-工作 | enum | 是 | 正常/轻度障碍/中度障碍/重度障碍 |
| social_func_social | 社会功能-社交 | enum | 是 | 同上 |
| social_func_selfcare | 社会功能-自理 | enum | 是 | 同上 |
| social_func_law | 社会功能-法律 | enum | 是 | 同上 |
| social_func_family | 社会功能-家庭 | enum | 是 | 同上 |
| danger_behavior_count | 危险行为次数 | int | 是 | 近30天内 |
| locked_status | 关锁情况 | enum | 是 | 无/偶尔/经常（经常 → 标记违规） |
| hospitalization | 住院情况 | enum | 是 | 无/近3月内住院1次/近3月内住院≥2次 |
| rehab_measures | 康复措施 | string | 选填 | 描述当前康复措施 |
| guardian_name | 监护人姓名 | string | 是 | |
| guardian_phone | 监护人电话 | string | 是 | |
| guardian_relation | 与患者关系 | enum | 是 | 配偶/父母/子女/其他 |

### C类 — 计算得出，不问

此类字段由系统根据A类和B类字段自动计算，不向患者提问。

| 字段键 | 字段名 | 计算方式 |
|---|---|---|
| bmi | BMI | 体重(kg) / 身高(m)^2 |
| bp_category | 血压分级 | <140/90 正常；140-159/90-99 1级；160-179/100-109 2级；≥180/110 3级 |
| followup_category | 本次随访分类 | 控制满意/控制不满意/出现不良反应/出现并发症（规则引擎判定） |
| trend_marker | 趋势标记 | 近3次同向变化 → "持续上升/下降/波动/稳定" |
| alert_level | 预警级别 | 见下方预警分层规则 |
| next_followup_date | 下次随访日期 | 控制满意→90天后；不满意→14天内；危急→立即转诊（规则出，医生可改） |
| completeness_score | 完整度得分 | 必填字段填充率（质控用，0-100） |
| confidence_flags | 置信度标记 | ASR/抽取置信度低的字段标黄 |
| ai_extracted_fields | AI抽取字段数 | 统计AI自动填充的字段数 |
| human_modified_fields | 人工修改字段数 | 统计diff_log中的修改数 |

---

## 二、预警分层规则

### 高血压预警规则

| 级别 | 触发条件 | 系统动作 |
|---|---|---|
| **红（危急）** | SBP≥180 或 DBP≥110；胸痛/呼吸困难；疑似卒中症状（口角歪斜/肢体无力/言语不清）；血糖<3.9 或 >16.7 伴症状 | 立即弹窗 + 短信通知医生 + 建议当日转诊 + 患者端给出就医提示 |
| **黄（警告）** | 连续2次控制不满意；服药依从性=不服药；出现新并发症症状；体重快速上升(>5kg/月)；自行调药 | 登记入异常列表，24h内处理；推送健康建议 |
| **绿（提示）** | 单项数据缺失；数值边界；生活方式不良（盐重/吸烟/缺乏运动）；血压处于正常高值 | 生成健康建议推送患者，不入异常列表 |

### 糖尿病预警规则

| 级别 | 触发条件 | 系统动作 |
|---|---|---|
| **红（危急）** | 血糖<3.9 伴心慌出汗手抖症状；血糖>16.7 伴烂苹果味/深大呼吸（酮症酸中毒可疑）；足部破溃 | 立即弹窗 + 短信通知医生 + 建议当日转诊 |
| **黄（警告）** | 连续2次空腹血糖≥7.0；HbA1c>9%；低血糖频繁；足背动脉减弱/消失 | 登记入异常列表，24h内处理 |
| **绿（提示）** | 空腹血糖偏快(6.1-7.0)；主食量超标；缺乏运动；体重超标 | 生成健康建议推送患者 |

### 老年人预警规则

| 级别 | 触发条件 | 系统动作 |
|---|---|---|
| **红（危急）** | ADL评分≥4分（轻度依赖以上）；近半年跌倒≥2次；GDS≥5分（抑郁倾向）；认知重度障碍 | 立即通知医生 + 建议上门评估 |
| **黄（警告）** | ADL评分≥2分；跌倒史；认知轻度障碍；GDS 3-4分 | 登记异常，安排随访 |
| **绿（提示）** | 腰围超标；视力听力下降；中医体质偏颇 | 推送健康管理建议 |

### 严重精神障碍预警规则

| 级别 | 触发条件 | 系统动作 |
|---|---|---|
| **红（危急）** | 危险性≥3级；悲观厌世/自伤自杀未遂；关锁经常；正在实施危险行为 | 立即报精防医生 + 公安联动提示 + 终止自助流程 |
| **黄（警告）** | 危险性2级；幻觉/妄想新发或加重；社会功能中度障碍；近3月住院≥2次 | 通知精防医生，缩短随访间隔 |
| **绿（提示）** | 危险性1级但服药不规律；社会功能轻度障碍 | 提醒监护人关注，推送康复指导 |

---

## 三、追问策略总览

### 通用追问优先级

Agent 按以下顺序推进对话（每个病种可在 questions.yaml 中微调）：

1. **症状询问**（安全优先）：有无不适 → 具体症状 → 持续时间 → 是否加重
2. **体征测量**：血压/血糖数值 → 测量方式 → 与前次对比
3. **用药情况**：是否规律服药 → 如有问题追问原因 → 不良反应
4. **生活方式**：吸烟/饮酒/运动/饮食/心理/睡眠
5. **病种专属**：按 disease_type 加载对应字段

### 兜底策略

| 场景 | 处理逻辑 |
|---|---|
| 患者说"不知道/忘了/没注意" | 记为 unknown，不重复追问同一槽位超过 2 次，跳过 |
| 患者答非所问 | 换一种问法重问一次，仍不回答则记 unknown |
| 数值明显离谱（如血压250） | 二次确认："您说的是185还是85？"，仍无法确认则标记"数值可疑" |
| 否定句（"没有头晕"） | Few-shot 强化否定句式识别，抽取时取反，复述确认 |
| 连续追问死循环 | 最多2次 → 记 unknown → 跳过 → 进入下一项 |
| ASR/抽取置信度低 | 字段标黄 + 复述确认 + 医生端标记"需复核" |

### 安全终止规则

出现以下情况立即终止对话，不生成任何健康建议：

1. 患者描述胸痛、呼吸困难、意识改变
2. 疑似卒中症状（口角歪斜、肢体无力、言语不清）
3. 自伤自杀表述
4. 严重精神障碍患者危险性≥3级
5. 患者明确表示"不想继续"

终止后展示：「感谢您的配合。根据您描述的情况，建议您尽快前往医院就诊。如有紧急情况请拨打120。」

---

## 四、数据结构示例（schema.json 骨架）

```json
{
  "disease": "hypertension",
  "version": "1.2",
  "fields": [
    {
      "key": "sbp",
      "label": "收缩压",
      "type": "integer",
      "unit": "mmHg",
      "range": [70, 250],
      "required": true,
      "ask_order": 2,
      "category": "B",
      "enum_options": null,
      "validation": {
        "warning_if": "sbp >= 140",
        "danger_if": "sbp >= 180 || sbp < 90",
        "must_gt_field": "dbp"
      },
      "repeat_confirm": true
    },
    {
      "key": "dbp",
      "label": "舒张压",
      "type": "integer",
      "unit": "mmHg",
      "range": [40, 150],
      "required": true,
      "ask_order": 3,
      "category": "B",
      "enum_options": null,
      "validation": {
        "warning_if": "dbp >= 90",
        "danger_if": "dbp >= 110 || dbp < 40",
        "must_lt_field": "sbp"
      },
      "repeat_confirm": true
    },
    {
      "key": "med_adherence",
      "label": "服药依从性",
      "type": "enum",
      "unit": null,
      "range": null,
      "required": true,
      "ask_order": 4,
      "category": "B",
      "enum_options": ["规律", "间断", "不服药"],
      "validation": null,
      "follow_up": {
        "if": "med_adherence in ['间断', '不服药']",
        "ask": "reason_not_take"
      },
      "repeat_confirm": true
    }
  ],
  "computed_fields": ["bmi", "bp_category", "followup_category", "trend_marker", "alert_level"],
  "alert_rules_ref": "rules.json"
}
```

---

## 五、随访对象与可信度标记规范

| 随访对象 | proxy标记 | 可信度评级 | 校验策略 |
|---|---|---|---|
| 本人 | false | 标准 | 正常校验 |
| 家属 | true | 中等 | 数值字段允许区间放宽10% |
| 监护人 | true | 中等 | 同上 |
| 知情人 | true | 较低 | 仅作为参考，医生端显著标注 |

所有非本人填报的数据，医生端必须展示代述人身份标识，如"（女儿代述）"。

---

## 附录B：Skill 包契约

### 目录结构

每个病种技能包必须包含以下文件，缺一不可不予加载：

```
skills/
  hypertension/
    SKILL.md          # 技能说明（渐进式披露入口）
    schema.json        # 字段定义（JSON Schema）
    questions.yaml     # 追问策略与顺序
    rules.json         # 校验与预警规则
    kb/                # 指南片段目录（RAG用）
      guideline_v1/
        chapter1.md
        chapter2.md
  diabetes/
    ...
  elderly/
    ...
  mental/
    ...
```

### 各文件规范

#### SKILL.md

```markdown
# {病种名称}随访技能

## 适用对象
{适用人群描述}

## 必填字段
见 schema.json。共 N 项，核心 X 项：...

## 对话策略
1. ...
2. ...

## 禁忌
- 不得...
- 不得...

## 安全终止条件
- ...
```

#### schema.json

必须包含 fields 数组，每个字段定义：key、label、type、unit、range、required、ask_order、category(A/B/C)、enum_options、validation、follow_up。

#### questions.yaml

定义追问顺序、触发条件、话术模板。格式：

```yaml
order:
  - field: symptoms
    question_template: "最近有没有{symptom_list}的感觉？"
    trigger: always
  - field: sbp
    question_template: "最近有量过血压吗？数值是多少？"
    trigger: after(symptoms)

triggers:
  - name: single_bp_value
    when: "sbp_provided and dbp_not_provided"
    question: "另一个数（舒张压）记得吗？"
    field: dbp
```

#### rules.json

定义校验规则和预警规则，格式见本文档"预警分层规则"章节。

#### kb/

存放指南文献切片，按版本分目录。每个 .md 文件包含：标题、正文、来源、版本号、适用病种。

### 加载契约

SkillLoader 启动时：
1. 扫描 skills/ 目录下所有子目录
2. 验证每个子目录包含全部5个必需文件
3. 解析 schema.json 并动态生成 Pydantic 模型
4. 缓存到内存，支持热更新（文件变更检测）
5. 未知病种返回友好错误，不 fallback 到通用模板

---

## 附录C：失败案例集（Failure Mode Library）

### 已识别的失败模式

| 失败类型 | 示例 | 对应防御机制 |
|---|---|---|
| ASR术语错 | "氨氯地平"→"安绿地平" | 医疗热词表 + 药物名字典兜底 |
| 否定句误解 | "没有头晕"→记成有头晕 | Few-shot强化否定句式 + 复述确认 |
| 数值口述歧义 | "一百五过九十" | 单位归一化 + 范围校验当场反问 |
| 答非所问 | 问血压答吃饭 | 槽位置信度阈值 + 换问法重问 |
| 追问死循环 | 患者一直说"不知道" | 最多2次 → 记unknown → 跳过 |
| 幻觉建议 | AI建议加药 | 禁止性Prompt + 输出审核 + 只允许引用指南原文 |
| 方言识别差 | 方言词汇无法识别 | 前端键盘输入兜底 + 第二联系人辅助 |
| 多轮上下文丢失 | 忘记前面问过什么 | SessionManager滑动窗口 + 已问记录 |
| 敏感信息泄露 | Prompt中包含真实姓名 | 脱敏中间件（正则+字典双通道） |
| 注入攻击 | 患者输入"忽略之前指令..." | 输入视为不可信 + 系统Prompt优先级最高 |

### 已知未解决项

| 未解决问题 | 当前处理 | 备注 |
|---|---|---|
| 重度方言识别 | 降级为键盘输入 | 需接入专业方言ASR模型 |
| 精神障碍患者自述可信度 | 仅限家属/监护人填报 | 业务边界设计，非技术问题 |
| 极端罕见病种 | 不在V1范围内 | 需人工配置Skill包 |

---

## 附录D：数据埋点详细规范

### 事件命名规范

采用 snake_case，格式：{动作}_{对象}，如 sms_sent、link_clicked、field_filled。

### 事件属性规范

| 属性 | 类型 | 说明 |
|---|---|---|
| event_id | string | 唯一事件ID（UUID） |
| timestamp | datetime | 事件发生时间（UTC） |
| task_id | string | 随访任务ID（哈希） |
| patient_id_hash | string | 患者ID哈希（SHA-256） |
| disease_type | string | 病种类型 |
| session_id | string | 会话ID |
| extra | object | 事件特定属性 |

### 隐私合规要求

1. 所有埋点不得携带患者姓名、身份证号、手机号等PII信息。
2. patient_id 必须经过单向哈希处理。
3. 录音文件单独加密存储，埋点仅记录文件索引。
4. 埋点数据保留期与业务数据一致，到期自动清理。
5. 导出埋点数据需审批留痕。

---

## 附录E：数据保留与患者权利（V1.2 新增）

### 数据保留期分层

| 数据类型 | 保留期限 | 清理方式 | 依据 |
|---|---|---|---|
| 录音文件 | 默认6个月 | 自动清理任务 | 《个人信息保护法》 |
| 对话原文 | 默认6个月 | 自动清理任务 | 《个人信息保护法》 |
| 结构化随访记录 | 长期（按病历管理要求） | 不自动清理 | 《医疗机构病历管理规定》 |
| 预警与处置记录 | 长期（按病历管理要求） | 不自动清理 | 《医疗机构病历管理规定》 |
| 埋点/日志数据 | 默认3年 | 归档后清理 | 等保要求 |

> **重要**：过程数据（录音、对话原文）与业务数据（结构化记录）不得混同清理。过程数据到期清理后，不影响结构化记录的长期保留与趋势比对。

### 患者权利

| 权利 | 法律依据 | 系统实现 |
|---|---|---|
| 撤回同意 | 《个保法》第15条 | 患者端提供"撤回随访同意"入口，撤回后停止数据采集并标记历史数据为"已撤回同意" |
| 个人信息删除/更正 | 《个保法》第47条 | 提供申请通道，受理后15日内完成处理并留痕 |
| 知情权 | 《个保法》第7条 | 首次进入H5时弹窗告知数据用途、存储期限、权利行使方式 |

### 合规检查清单

- [ ] 数据采集前是否取得用户明示同意？
- [ ] 是否提供撤回同意入口？
- [ ] 个人信息删除/更正申请是否在15日内处理？
- [ ] 过程数据是否按保留期自动清理？
- [ ] 审计日志是否独立存储且不可篡改？
- [ ] 是否定期进行数据安全评估？
