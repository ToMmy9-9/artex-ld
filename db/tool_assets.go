// tool_assets.go：外部工具资产（上传的攻防二进制）与其执行审计的存取层。
//
// 与 tools 表的分工：
//   tools       —— 平台"能调用什么"的抽象（kind=shell/mcp/http…）
//   tool_assets —— 磁盘上真实存在的外部二进制及其受管契约
//
// 权限模型：contract_limits 是平台/管理员定义的"最大权限"，contract 是
// agent 认领后给出的"实际契约"，二者都要校验 contract ⊆ limits。
package db

import (
	"encoding/json"
	"time"
)

// ToolAsset 是上传到受管目录的一个外部工具。
type ToolAsset struct {
	ID           int64           `json:"id"`
	Name         string          `json:"name"`
	DisplayName  string          `json:"display_name"`
	Version      string          `json:"version"`
	SHA256       string          `json:"sha256"`
	SizeBytes    int64           `json:"size_bytes"`
	Platform     string          `json:"platform"`
	RelPath      string          `json:"rel_path"`
	Category     string          `json:"category"`
	RiskLevel    string          `json:"risk_level"`
	Summary      string          `json:"summary"`
	UsageExample string          `json:"usage_example"`
	Limits       json.RawMessage `json:"contract_limits"`
	Contract     json.RawMessage `json:"contract"`
	ProbeOutput  string          `json:"probe_output"`
	State        string          `json:"state"`
	ManagedBy    string          `json:"managed_by"`
	Note         string          `json:"note"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// ToolAssetRun 是外部工具的一次受管执行记录（审计用）。
type ToolAssetRun struct {
	ID         int64           `json:"id"`
	AssetID    int64           `json:"asset_id"`
	TaskID     int64           `json:"task_id"`
	AgentKey   string          `json:"agent_key"`
	ToolName   string          `json:"tool_name"`
	Argv       json.RawMessage `json:"argv"`
	Targets    json.RawMessage `json:"targets"`
	ExitCode   int             `json:"exit_code"`
	DurationMS int             `json:"duration_ms"`
	CreatedAt  time.Time       `json:"created_at"`
}

const toolAssetCols = `id,name,display_name,version,sha256,size_bytes,platform,rel_path,category,risk_level,summary,usage_example,contract_limits,contract,probe_output,state,managed_by,note,created_at,updated_at`

func scanToolAsset(row interface{ Scan(...any) error }) (*ToolAsset, error) {
	var a ToolAsset
	err := row.Scan(&a.ID, &a.Name, &a.DisplayName, &a.Version, &a.SHA256, &a.SizeBytes,
		&a.Platform, &a.RelPath, &a.Category, &a.RiskLevel, &a.Summary, &a.UsageExample,
		&a.Limits, &a.Contract, &a.ProbeOutput, &a.State, &a.ManagedBy, &a.Note,
		&a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if len(a.Limits) == 0 {
		a.Limits = json.RawMessage(`{}`)
	}
	if len(a.Contract) == 0 {
		a.Contract = json.RawMessage(`{}`)
	}
	return &a, nil
}

// ListToolAssets returns every uploaded tool asset, newest first.
func (d *DB) ListToolAssets() ([]*ToolAsset, error) {
	rows, err := d.Query(`SELECT ` + toolAssetCols + ` FROM tool_assets ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ToolAsset
	for rows.Next() {
		a, err := scanToolAsset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ToolAssetByID loads one asset by id.
func (d *DB) ToolAssetByID(id int64) (*ToolAsset, error) {
	a, err := scanToolAsset(d.QueryRow(`SELECT ` + toolAssetCols + ` FROM tool_assets WHERE id=$1`, id))
	if err != nil {
		return nil, err
	}
	return a, nil
}

// ToolAssetByName loads one asset by its tool name (and platform when given).
func (d *DB) ToolAssetByName(name, platform string) (*ToolAsset, error) {
	if platform != "" {
		a, err := scanToolAsset(d.QueryRow(
			`SELECT `+toolAssetCols+` FROM tool_assets WHERE name=$1 AND platform=$2`, name, platform))
		if err != nil {
			return nil, err
		}
		return a, nil
	}
	a, err := scanToolAsset(d.QueryRow(
		`SELECT `+toolAssetCols+` FROM tool_assets WHERE name=$1 ORDER BY created_at DESC LIMIT 1`, name))
	if err != nil {
		return nil, err
	}
	return a, nil
}

// InsertToolAsset records a freshly uploaded binary. The (name, platform)
// unique index makes re-uploading the same tool a conflict the caller can
// surface as "already exists" rather than a silent duplicate.
func (d *DB) InsertToolAsset(a *ToolAsset) (int64, error) {
	var id int64
	err := d.QueryRow(`
INSERT INTO tool_assets(name,display_name,version,sha256,size_bytes,platform,rel_path,state)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
RETURNING id`,
		a.Name, a.DisplayName, a.Version, a.SHA256, a.SizeBytes, a.Platform, a.RelPath, a.State).Scan(&id)
	if err != nil {
		return 0, err
	}
	a.ID = id
	return id, nil
}

// UpdateToolAssetMeta updates the descriptive fields an agent fills in during
// the claim step. It deliberately does NOT touch contract_limits: the limits
// are the platform's ceiling and only an administrator may change them.
func (d *DB) UpdateToolAssetMeta(id int64, category, riskLevel, summary, usage, probe, managedBy string) error {
	_, err := d.Exec(`
UPDATE tool_assets
SET category=$2, risk_level=$3, summary=$4, usage_example=$5, probe_output=$6,
    managed_by=$7, state=CASE WHEN $8 THEN 'managed' ELSE state END
WHERE id=$1`,
		id, category, riskLevel, summary, usage, probe, managedBy, managedBy != "")
	return err
}

// UpdateToolAssetState moves an asset between lifecycle states
// (uploaded/probing/managed/rejected/quarantined).
func (d *DB) UpdateToolAssetState(id int64, state string) error {
	_, err := d.Exec(`UPDATE tool_assets SET state=$2 WHERE id=$1`, id, state)
	return err
}

// SetToolAssetContract stores the agent-produced contract after validating it
// against the platform's limits.
func (d *DB) SetToolAssetContract(id int64, contract json.RawMessage) error {
	_, err := d.Exec(`UPDATE tool_assets SET contract=$2, state='managed' WHERE id=$1`, id, contract)
	return err
}

// SetToolAssetLimits replaces the platform-defined permission ceiling.
func (d *DB) SetToolAssetLimits(id int64, limits json.RawMessage) error {
	_, err := d.Exec(`UPDATE tool_assets SET contract_limits=$2 WHERE id=$1`, id, limits)
	return err
}

// DeleteToolAsset removes an asset row. The file on disk is removed by the
// caller (the server owns the managed directory).
func (d *DB) DeleteToolAsset(id int64) error {
	_, err := d.Exec(`DELETE FROM tool_assets WHERE id=$1`, id)
	return err
}

// InsertToolAssetRun appends one execution record for auditing.
func (d *DB) InsertToolAssetRun(r *ToolAssetRun) (int64, error) {
	var id int64
	err := d.QueryRow(`
INSERT INTO tool_asset_runs(asset_id,task_id,agent_key,tool_name,argv,targets,exit_code,duration_ms)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
RETURNING id`,
		r.AssetID, r.TaskID, r.AgentKey, r.ToolName, r.Argv, r.Targets, r.ExitCode, r.DurationMS).Scan(&id)
	if err != nil {
		return 0, err
	}
	r.ID = id
	return id, nil
}

// ListToolAssetRuns returns the audit trail, most recent first.
func (d *DB) ListToolAssetRuns(assetID int64, limit int) ([]*ToolAssetRun, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := d.Query(`
SELECT id,asset_id,task_id,agent_key,tool_name,argv,targets,exit_code,duration_ms,created_at
FROM tool_asset_runs
WHERE ($1 = 0 OR asset_id = $1)
ORDER BY created_at DESC
LIMIT $2`, assetID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ToolAssetRun
	for rows.Next() {
		var r ToolAssetRun
		if err := rows.Scan(&r.ID, &r.AssetID, &r.TaskID, &r.AgentKey, &r.ToolName,
			&r.Argv, &r.Targets, &r.ExitCode, &r.DurationMS, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}

// ReplaceToolAssetBinary overwrites an existing asset with a freshly uploaded
// binary of the same (name, platform).
//
// The audit history (tool_asset_runs) is intentionally NOT touched: past runs
// referenced the old binary, and that record stays valid. What IS reset is the
// claim conclusion — a new binary may be a different build with different
// flags, so the previous contract and notes no longer apply and the keeper
// must re-claim it.
func (d *DB) ReplaceToolAssetBinary(id int64, a *ToolAsset) error {
	_, err := d.Exec(`
UPDATE tool_assets
SET display_name=$2, version=$3, sha256=$4, size_bytes=$5, rel_path=$6,
    category='', risk_level='', summary='', usage_example='', contract='{}'::jsonb,
    probe_output='', state='uploaded', managed_by='', contract_limits=$7
WHERE id=$1`,
		id, a.DisplayName, a.Version, a.SHA256, a.SizeBytes, a.RelPath, a.Limits)
	return err
}
