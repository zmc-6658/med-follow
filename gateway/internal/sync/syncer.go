package sync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// 手机号校验：11 位大陆手机号（仅用于脏数据提示，不影响入库）
var phoneRe = regexp.MustCompile(`^1[3-9]\d{9}$`)

// SyncResult 一次同步批次的结果（对应 sync_log 一行）
type SyncResult struct {
	BatchID      string   `json:"batch_id"`
	Fetched      int      `json:"fetched"`
	Inserted     int      `json:"inserted"`
	Updated      int      `json:"updated"`
	Skipped      int      `json:"skipped"`
	ErrorSummary []string `json:"error_summary,omitempty"`
}

// Syncer 同步管道编排：增量拉取（update_time）→ 幂等 upsert（his_patient_id）→ sync_log 留痕
// SQL 全部在 store.go
type Syncer struct {
	pool    *pgxpool.Pool
	adapter SyncAdapter
}

func NewSyncer(pool *pgxpool.Pool, adapter SyncAdapter) *Syncer {
	return &Syncer{pool: pool, adapter: adapter}
}

func newBatchID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return time.Now().Format("20060102150405") + "-" + hex.EncodeToString(b)
}

func (s *Syncer) Run(ctx context.Context) (*SyncResult, error) {
	res := &SyncResult{BatchID: newBatchID()}

	lastSync, err := loadLastSync(ctx, s.pool)
	if err != nil {
		return nil, fmt.Errorf("read sync_state: %w", err)
	}

	patients, err := s.adapter.FetchSince(ctx, lastSync)
	if err != nil {
		return nil, fmt.Errorf("fetch from adapter: %w", err)
	}
	res.Fetched = len(patients)

	logID, err := insertSyncLog(ctx, s.pool, res.BatchID, res.Fetched)
	if err != nil {
		return nil, fmt.Errorf("insert sync_log: %w", err)
	}

	var maxUpdate time.Time
	for i := range patients {
		p := &patients[i]
		if p.HisPatientID == "" || p.Name == "" || p.DiseaseType != "hypertension" {
			res.Skipped++
			res.ErrorSummary = append(res.ErrorSummary,
				fmt.Sprintf("%s: missing required fields or unsupported disease_type", p.HisPatientID))
			continue
		}
		if p.Phone != "" && !phoneRe.MatchString(p.Phone) {
			// 脏数据容错：档案照常入库（读以 HIS 为准），问题留痕待人工处理
			res.ErrorSummary = append(res.ErrorSummary, p.HisPatientID+": invalid phone format")
		}
		inserted, err := upsertPatient(ctx, s.pool, p)
		if err != nil {
			res.Skipped++
			res.ErrorSummary = append(res.ErrorSummary, p.HisPatientID+": "+err.Error())
			continue
		}
		if inserted {
			res.Inserted++
		} else {
			res.Updated++
		}
		if p.UpdateTime.After(maxUpdate) {
			maxUpdate = p.UpdateTime
		}
	}

	// 水位只前进不后退：取本批已同步记录的 max(update_time)
	if maxUpdate.After(lastSync) {
		if err := saveLastSync(ctx, s.pool, maxUpdate); err != nil {
			return nil, fmt.Errorf("update sync_state: %w", err)
		}
	}

	if err := finishSyncLog(ctx, s.pool, logID, res); err != nil {
		return nil, fmt.Errorf("update sync_log: %w", err)
	}

	log.Printf("[sync] batch=%s fetched=%d inserted=%d updated=%d skipped=%d",
		res.BatchID, res.Fetched, res.Inserted, res.Updated, res.Skipped)
	return res, nil
}
