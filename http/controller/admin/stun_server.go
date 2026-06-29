package admin

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

type StunServer struct{}

func (c *StunServer) List(ctx *gin.Context) {
	var list []model.StunServer
	service.DB.Order("sort asc, row_id asc").Find(&list)
	response.Success(ctx, gin.H{"list": list})
}

func (c *StunServer) Create(ctx *gin.Context) {
	f := &model.StunServer{}
	if err := ctx.ShouldBindJSON(f); err != nil {
		response.Fail(ctx, 101, "参数错误")
		return
	}
	if f.Host == "" || f.Port <= 0 {
		response.Fail(ctx, 101, "地址不能为空")
		return
	}
	service.DB.Create(f)
	response.Success(ctx, nil)
}

func (c *StunServer) Update(ctx *gin.Context) {
	f := &model.StunServer{}
	if err := ctx.ShouldBindJSON(f); err != nil || f.RowId == 0 {
		response.Fail(ctx, 101, "参数错误")
		return
	}
	service.DB.Model(&model.StunServer{}).Where("row_id = ?", f.RowId).Updates(f)
	response.Success(ctx, nil)
}

func (c *StunServer) Delete(ctx *gin.Context) {
	form := &struct{ Id uint `json:"id"` }{}
	if err := ctx.ShouldBindJSON(form); err != nil || form.Id == 0 {
		response.Fail(ctx, 101, "ID不能为空")
		return
	}
	service.DB.Delete(&model.StunServer{}, form.Id)
	response.Success(ctx, nil)
}

func (c *StunServer) Get(ctx *gin.Context) {
	// Return the first enabled STUN server (for client auto-config)
	var srv model.StunServer
	service.DB.Where("enabled = 1").Order("sort asc, row_id asc").First(&srv)
	if srv.RowId == 0 {
		// Return default Google STUN as fallback
		response.Success(ctx, gin.H{"host": "stun.l.google.com", "port": 19302})
		return
	}
	response.Success(ctx, srv)
}

// Test sends a STUN binding request and returns the result
func (c *StunServer) Test(ctx *gin.Context) {
	form := &struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}{}
	if err := ctx.ShouldBindJSON(form); err != nil || form.Host == "" {
		response.Fail(ctx, 101, "参数错误")
		return
	}
	if form.Port <= 0 {
		form.Port = 3478
	}

	// Build STUN binding request (RFC 5389)
	msgType := uint16(0x0001) // Binding Request
	magicCookie := uint32(0x2112A442)
	txID := make([]byte, 12)
	rand.Read(txID)

	buf := &bytes.Buffer{}
	binary.Write(buf, binary.BigEndian, msgType)
	binary.Write(buf, binary.BigEndian, uint16(0)) // length=0
	binary.Write(buf, binary.BigEndian, magicCookie)
	buf.Write(txID)

	// Send via UDP
	addr := fmt.Sprintf("%s:%d", form.Host, form.Port)
	conn, err := net.DialTimeout("udp", addr, 3*time.Second)
	if err != nil {
		response.Fail(ctx, 101, "连接失败: "+err.Error())
		return
	}
	defer conn.Close()

	start := time.Now()
	conn.Write(buf.Bytes())

	respBuf := make([]byte, 4096)
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, err := conn.Read(respBuf)
	if err != nil {
		response.Fail(ctx, 101, "无响应（超时）")
		return
	}
	elapsed := time.Since(start).Milliseconds()

	// Parse response - extract XOR-MAPPED-ADDRESS (attribute type 0x0020)
	if n < 20 {
		response.Fail(ctx, 101, "响应格式错误")
		return
	}

	respType := binary.BigEndian.Uint16(respBuf[0:2])
	if respType != 0x0101 {
		response.Fail(ctx, 101, fmt.Sprintf("非STUN响应(type=0x%04x)", respType))
		return
	}

	// Parse attributes starting at offset 20
	pos := 20
	var mappedIP string
	var mappedPort int
	for pos+4 < n {
		attrType := binary.BigEndian.Uint16(respBuf[pos : pos+2])
		attrLen := binary.BigEndian.Uint16(respBuf[pos+2 : pos+4])
		if pos+4+int(attrLen) > n {
			break
		}
		attrVal := respBuf[pos+4 : pos+4+int(attrLen)]
		if attrType == 0x0020 { // XOR-MAPPED-ADDRESS
			if len(attrVal) >= 8 {
				// Skip family (1 byte) + padding (1 byte)
				xorPort := binary.BigEndian.Uint16(attrVal[2:4])
				mappedPort = int(xorPort ^ uint16(magicCookie>>16))

				ipBytes := make([]byte, 4)
				for i := 0; i < 4; i++ {
					ipBytes[i] = attrVal[4+i] ^ respBuf[4+i] // XOR with magic cookie bytes
				}
				mappedIP = net.IP(ipBytes).String()
			}
		}
		pos += 4 + int(attrLen)
		// Pad to 4-byte boundary
		if attrLen%4 != 0 {
			pos += 4 - int(attrLen%4)
		}
	}

	if mappedIP == "" {
		response.Fail(ctx, 101, "未找到地址信息")
		return
	}

	response.Success(ctx, gin.H{
		"mapped_ip":   mappedIP,
		"mapped_port": mappedPort,
		"response_ms": elapsed,
		"server":      form.Host + ":" + fmt.Sprint(form.Port),
	})
}
