package sync

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// 本域全部 SQL：sync_state（增量水位）、sync_log（批次留痕）、patients（档案 upsert）

func loadLastSync(ctx context.Context, pool *pgxpool.Pool) (time.Time, error) {
	var last time.Time
	err := pool.QueryRow(ctx, `SELECT last_sync_at FROM sync_state WHERE id = 1`).Scan(&last)
	return last, err
}

func saveLastSync(ctx context.Context, pool *pgxpool.Pool, t time.Time) error {
	_, err := pool.Exec(ctx,
		`UPDATE sync_state SET last_sync_at = $1, updated_at = now() WHERE id = 1`, t)
	return err
}

func insertSyncLog(ctx context.Context, pool *pgxpool.Pool, batchID string, fetched int) (int64, error) {
	var id int64
	err := pool.QueryRow(ctx,
		`INSERT INTO sync_log (batch_id, started_at, fetched) VALUES ($1, now(), $2) RETURNING id`,
		batchID, fetched).Scan(&id)
	return id, err
}

func finishSyncLog(ctx context.Context, pool *pgxpool.Pool, logID int64, res *SyncResult) error {
	_, err := pool.Exec(ctx,
		`UPDATE sync_log SET finished_at = now(), inserted = $2, updated = $3, skipped = $4, error_summary = $5 WHERE id = $1`,
		logID, res.Inserted, res.Updated, res.Skipped, joinSummary(res.ErrorSummary))
	return err
}

// upsertPatient 以 his_patient_id 为幂等键，先 UPDATE 后 INSERT；返回是否为新增。
// phone_source='manual' 的电话不被覆盖（人工修正优先于 HIS）
func upsertPatient(ctx context.Context, pool *pgxpool.Pool, p *HisPatient) (bool, error) {
	tag, err := pool.Exec(ctx,
		`UPDATE patients SET name=$2, gender=$3, birth_date=$4, age=$5,
		   phone     = CASE WHEN phone_source = 'manual' THEN phone     ELSE $6 END,
		   phone_alt = CASE WHEN phone_source = 'manual' THEN phone_alt ELSE $7 END,
		   diagnosis_date=$8, current_medications=$9, managing_doctor=$10, health_center=$11,
		   updated_at=now()
		 WHERE his_patient_id=$1`,
		p.HisPatientID, p.Name, p.Gender, parseDate(p.BirthDate), calcAge(p.BirthDate),
		p.Phone, p.PhoneAlt, parseDate(p.DiagnosisDate),
		renderMedications(p.Medications), p.ManagingDoctorName, p.OrgName)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() > 0 {
		return false, nil
	}

	_, err = pool.Exec(ctx,
		`INSERT INTO patients (patient_id, name, gender, birth_date, age, phone, phone_alt,
		   disease_type, diagnosis_date, current_medications, managing_doctor, health_center,
		   his_patient_id, phone_source)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$1,'his')`,
		p.HisPatientID, p.Name, p.Gender, parseDate(p.BirthDate), calcAge(p.BirthDate),
		p.Phone, p.PhoneAlt, p.DiseaseType, parseDate(p.DiagnosisDate),
		renderMedications(p.Medications), p.ManagingDoctorName, p.OrgName)
	if err != nil {
		return false, err
	}
	return true, nil
}

// ---- 以下为 HIS 字段 → patients 列 的映射函数，仅供 upsertPatient 使用 ----

func parseDate(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return &t
}

func renderMedications(meds []Medication) string {
	parts := make([]string, 0, len(meds))
	for _, m := range meds {
		parts = append(parts, fmt.Sprintf("%s %s %s", m.DrugName, m.Dosage, m.Frequency))
	}
	return strings.Join(parts, "；")
}

func calcAge(birth string) int {
	t, err := time.Parse("2006-01-02", birth)
	if err != nil {
		return 0
	}
	now := time.Now()
	age := now.Year() - t.Year()
	if now.YearDay() < t.YearDay() {
		age--
	}
	return age
}

func joinSummary(items []string) string {
	return strings.Join(items, "; ")
}
