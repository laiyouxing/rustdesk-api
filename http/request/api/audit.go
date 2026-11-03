package api

import (
	"encoding/json"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"strconv"
)

type AuditConnForm struct {
	Action    string   `json:"action"`
	ConnId    int64    `json:"conn_id"`
	Id        string   `json:"id"`
	Peer      []string `json:"peer"`
	Ip        string   `json:"ip"`
	SessionId float64  `json:"session_id"`
	Type      int      `json:"type"`
	Uuid      string   `json:"uuid"`
	// Version 被控端版本；FromVersion 控制端版本。老客户端不传，服务端会用 peers 表兜底。
	Version     string `json:"version"`
	FromVersion string `json:"from_version"`
	// OpId 控制端「一次操作」的标识（1.7.32+），同一次点击的所有重试都相同；
	// 老客户端不传（0），服务端回退到时间窗推断。
	OpId uint64 `json:"op_id"`
}

func (a *AuditConnForm) ToAuditConn() *model.AuditConn {
	fp := ""
	fn := ""
	if len(a.Peer) >= 1 {
		fp = a.Peer[0]
		if len(a.Peer) == 2 {
			fn = a.Peer[1]
		}
	}
	ssid := strconv.FormatFloat(a.SessionId, 'f', -1, 64)
	return &model.AuditConn{
		Action:      a.Action,
		ConnId:      a.ConnId,
		PeerId:      a.Id,
		FromPeer:    fp,
		FromName:    fn,
		Ip:          a.Ip,
		SessionId:   ssid,
		Type:        a.Type,
		Uuid:        a.Uuid,
		Version:     a.Version,
		FromVersion: a.FromVersion,
		OpId:        a.OpId,
	}
}

type AuditFileForm struct {
	Id     string `json:"id"`
	Info   string `json:"info"`
	IsFile bool   `json:"is_file"`
	Path   string `json:"path"`
	PeerId string `json:"peer_id"`
	Type   int    `json:"type"`
	Uuid   string `json:"uuid"`
	// Version 被控端版本；FromVersion 控制端版本，老客户端不传则由服务端用 peers 表兜底。
	Version     string `json:"version"`
	FromVersion string `json:"from_version"`
}
type AuditFileInfo struct {
	Ip   string `json:"ip"`
	Name string `json:"name"`
	Num  int    `json:"num"`
}

func (a *AuditFileForm) ToAuditFile() *model.AuditFile {
	fi := &AuditFileInfo{}
	err := json.Unmarshal([]byte(a.Info), fi)
	if err != nil {
		global.Logger.Warn("ToAuditFile", err)
	}

	return &model.AuditFile{
		PeerId:   a.Id,
		Info:     a.Info,
		IsFile:   a.IsFile,
		FromPeer: a.PeerId,
		Path:     a.Path,
		Type:     a.Type,
		Uuid:     a.Uuid,
		FromName: fi.Name,
		Ip:       fi.Ip,
		Num:      fi.Num,
		// 版本：新版客户端上报，老客户端为空，由 controller 用 peers 表兜底。
		Version:     a.Version,
		FromVersion: a.FromVersion,
	}
}
