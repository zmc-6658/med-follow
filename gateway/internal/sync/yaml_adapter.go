package sync

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

// YamlMockAdapter 本地 YAML 种子数据源（MVP Mock，见 testdata/patients/）
type YamlMockAdapter struct {
	dir string
}

func NewYamlMockAdapter(dir string) *YamlMockAdapter {
	return &YamlMockAdapter{dir: dir}
}

func (a *YamlMockAdapter) FetchSince(_ context.Context, since time.Time) ([]HisPatient, error) {
	entries, err := os.ReadDir(a.dir)
	if err != nil {
		return nil, err
	}

	var out []HisPatient
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(a.dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var p HisPatient
		if err := yaml.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
		if p.UpdateTime.After(since) {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdateTime.Before(out[j].UpdateTime) })
	return out, nil
}
