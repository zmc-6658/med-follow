package sync

import (
	"context"
	"time"
)

// HisDiagnosis HIS 侧诊断（共病患者在此扩展病种）
type HisDiagnosis struct {
	DiseaseType   string `yaml:"disease_type"`
	DiagnosisDate string `yaml:"diagnosis_date"`
}

// Medication 当前用药
type Medication struct {
	DrugName  string `yaml:"drug_name"`
	Dosage    string `yaml:"dosage"`
	Frequency string `yaml:"frequency"`
}

// HisPatient HIS 患者档案（主档 + 诊断 + 当前用药），字段与 testdata/patients/*.yaml 对应
type HisPatient struct {
	HisPatientID       string         `yaml:"his_patient_id"`
	Name               string         `yaml:"name"`
	Gender             string         `yaml:"gender"`
	BirthDate          string         `yaml:"birth_date"`
	IDCard             string         `yaml:"id_card"`
	Phone              string         `yaml:"phone"`
	PhoneAlt           string         `yaml:"phone_alt"`
	Address            string         `yaml:"address"`
	OrgName            string         `yaml:"org_name"`
	DeptName           string         `yaml:"dept_name"`
	ManagingDoctorName string         `yaml:"managing_doctor_name"`
	DiseaseType        string         `yaml:"disease_type"`
	DiagnosisDate      string         `yaml:"diagnosis_date"`
	ExtraDiagnoses     []HisDiagnosis `yaml:"extra_diagnoses"`
	Medications        []Medication   `yaml:"medications"`
	UpdateTime         time.Time      `yaml:"update_time"`
}

// SyncAdapter 数据源适配器契约：生产接入时按医院对接方式新增实现，同步管道零改动
type SyncAdapter interface {
	// FetchSince 拉取 update_time 晚于 since 的患者档案，按 update_time 升序返回
	FetchSince(ctx context.Context, since time.Time) ([]HisPatient, error)
}
