// tool_env_preps.go：外部工具「环境准备」的审计存取层。
//
// 环境准备 = 平台从一个 URL 下载工具包、解压到受管目录、登记成工具资产。
// 它**不执行任何东西**（不跑安装脚本、不跑 setup.py、不跑 postinst），
// 所以这里记录的是「什么东西被放到了这台机器上」。
//
// 这条审计存在的理由：agent 可以自主为工具下载依赖，等于让模型决定
// 「往宿主磁盘写什么」。不执行是一道闸门，可追溯是第二道——出问题时
// 能回答：哪个 URL、什么时候、谁要求的、落了哪些文件。
package db

import "time"

// ToolEnvPrep 是一次环境准备的记录（无论成功失败都留痕）。
type ToolEnvPrep struct {
	ID         int64     `json:"id"`
	ToolName   string    `json:"tool_name"`
	URL        string    `json:"url"`
	FinalURL   string    `json:"final_url"`   // 跟随重定向后的实际地址
	Archive    string    `json:"archive"`     // zip / tar.gz / tar / gz / binary
	SHA256     string    `json:"sha256"`      // 下载内容的哈希
	Downloaded int64     `json:"downloaded"`  // 下载字节数
	FileCount  int       `json:"file_count"`  // 解压出的文件数
	TotalBytes int64     `json:"total_bytes"` // 解压后总字节数
	TargetDir  string    `json:"target_dir"`  // 落盘的相对目录（受管目录内）
	EntryBin   string    `json:"entry_bin"`   // 识别出的主可执行文件（相对路径）
	AssetID    int64     `json:"asset_id"`    // 登记出的 tool_assets id（0 = 未登记）
	State      string    `json:"state"`       // ok / failed
	Error      string    `json:"error"`
	By         string    `json:"requested_by"` // keeper / human:<user> / api
	CreatedAt  time.Time `json:"created_at"`
}

const toolEnvPrepCols = `id,tool_name,url,final_url,archive,sha256,downloaded,file_count,total_bytes,target_dir,entry_bin,asset_id,state,error,requested_by,created_at`

// InsertToolEnvPrep records one environment-preparation attempt.
func (d *DB) InsertToolEnvPrep(p *ToolEnvPrep) (int64, error) {
	var id int64
	err := d.QueryRow(`
INSERT INTO tool_env_preps(tool_name,url,final_url,archive,sha256,downloaded,file_count,
                           total_bytes,target_dir,entry_bin,asset_id,state,error,requested_by)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
RETURNING id`,
		p.ToolName, p.URL, p.FinalURL, p.Archive, p.SHA256, p.Downloaded, p.FileCount,
		p.TotalBytes, p.TargetDir, p.EntryBin, p.AssetID, p.State, p.Error, p.By).Scan(&id)
	if err != nil {
		return 0, err
	}
	p.ID = id
	return id, nil
}

// ListToolEnvPreps returns the audit trail, newest first.
func (d *DB) ListToolEnvPreps(limit int) ([]*ToolEnvPrep, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := d.Query(`SELECT `+toolEnvPrepCols+` FROM tool_env_preps
		ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ToolEnvPrep
	for rows.Next() {
		var p ToolEnvPrep
		if err := rows.Scan(&p.ID, &p.ToolName, &p.URL, &p.FinalURL, &p.Archive, &p.SHA256,
			&p.Downloaded, &p.FileCount, &p.TotalBytes, &p.TargetDir, &p.EntryBin,
			&p.AssetID, &p.State, &p.Error, &p.By, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

// ListToolEnvPrepsByName returns the preparations recorded for one tool.
func (d *DB) ListToolEnvPrepsByName(name string, limit int) ([]*ToolEnvPrep, error) {
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	rows, err := d.Query(`SELECT `+toolEnvPrepCols+` FROM tool_env_preps
		WHERE tool_name = $1 ORDER BY created_at DESC LIMIT $2`, name, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ToolEnvPrep
	for rows.Next() {
		var p ToolEnvPrep
		if err := rows.Scan(&p.ID, &p.ToolName, &p.URL, &p.FinalURL, &p.Archive, &p.SHA256,
			&p.Downloaded, &p.FileCount, &p.TotalBytes, &p.TargetDir, &p.EntryBin,
			&p.AssetID, &p.State, &p.Error, &p.By, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}
