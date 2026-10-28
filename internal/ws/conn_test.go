package ws

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"
)

// newReader wraps a connection into the buffered reader used by Conn.
func newReader(conn net.Conn) *bufio.Reader {
	return bufio.NewReaderSize(conn, 4096)
}

// equalFoldTrim compares two header values ignoring case and surrounding spaces.
func equalFoldTrim(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), b)
}

// buildMaskedFrame builds a client style (masked) frame for the tests.
func buildMaskedFrame(opcode byte, payload []byte, fin bool) []byte {
	frame := []byte{}
	first := opcode
	if fin {
		first |= 0x80
	}
	frame = append(frame, first)

	length := len(payload)
	switch {
	case length < 126:
		frame = append(frame, byte(0x80|length))
	case length <= 0xFFFF:
		frame = append(frame, 0x80|126)
		var ext [2]byte
		binary.BigEndian.PutUint16(ext[:], uint16(length))
		frame = append(frame, ext[:]...)
	default:
		frame = append(frame, 0x80|127)
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(length))
		frame = append(frame, ext[:]...)
	}

	mask := []byte{0x12, 0x34, 0x56, 0x78}
	frame = append(frame, mask...)
	masked := make([]byte, length)
	for i, b := range payload {
		masked[i] = b ^ mask[i%4]
	}
	return append(frame, masked...)
}

func TestReadMessageUnmasksClientFrame(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	conn := &Conn{conn: server, br: newReader(server)}
	go func() {
		client.Write(buildMaskedFrame(OpText, []byte("你好 WebSocket"), true))
	}()

	opcode, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("读取消息失败: %v", err)
	}
	if opcode != OpText {
		t.Fatalf("操作码应为 OpText，实际: %d", opcode)
	}
	if string(payload) != "你好 WebSocket" {
		t.Fatalf("消息内容不正确: %s", payload)
	}
}

func TestReadMessageReassemblesFragments(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	conn := &Conn{conn: server, br: newReader(server)}
	go func() {
		client.Write(buildMaskedFrame(OpText, []byte("前半段"), false))
		client.Write(buildMaskedFrame(OpContinuation, []byte("后半段"), true))
	}()

	opcode, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("读取分片消息失败: %v", err)
	}
	if opcode != OpText {
		t.Fatalf("分片消息的操作码应为 OpText，实际: %d", opcode)
	}
	if string(payload) != "前半段后半段" {
		t.Fatalf("分片重组结果不正确: %s", payload)
	}
}

func TestReadMessageHandlesLargePayload(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	conn := &Conn{conn: server, br: newReader(server)}
	large := bytes.Repeat([]byte("A"), 70000)
	go func() {
		client.Write(buildMaskedFrame(OpBinary, large, true))
	}()

	opcode, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("读取大消息失败: %v", err)
	}
	if opcode != OpBinary {
		t.Fatalf("操作码应为 OpBinary，实际: %d", opcode)
	}
	if len(payload) != len(large) {
		t.Fatalf("负载长度不正确: %d != %d", len(payload), len(large))
	}
}

func TestReadMessageRejectsUnmaskedFrame(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	conn := &Conn{conn: server, br: newReader(server)}
	go func() {
		// 未掩码的帧，违反 RFC 6455 对客户端帧的要求
		client.Write([]byte{0x81, 0x05, 'h', 'e', 'l', 'l', 'o'})
	}()

	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("未掩码的帧应被拒绝")
	}
}

func TestWriteMessageProducesUnmaskedFrame(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	conn := &Conn{conn: server, br: newReader(server)}
	go func() {
		_ = conn.WriteText([]byte("服务端消息"))
	}()

	_ = client.SetReadDeadline(time.Now().Add(3 * time.Second))
	header := make([]byte, 2)
	if _, err := client.Read(header[:]); err != nil {
		t.Fatalf("读取帧头失败: %v", err)
	}
	if header[0]&0x80 == 0 {
		t.Fatal("服务端帧的 FIN 位应置位")
	}
	if header[0]&0x0F != OpText {
		t.Fatal("帧操作码应为 OpText")
	}
	if header[1]&0x80 != 0 {
		t.Fatal("服务端帧不应带掩码")
	}
	length := int(header[1] & 0x7F)
	payload := make([]byte, length)
	if _, err := client.Read(payload); err != nil {
		t.Fatalf("读取负载失败: %v", err)
	}
	if string(payload) != "服务端消息" {
		t.Fatalf("负载内容不正确: %s", payload)
	}
}

func TestHubTracksMultiDeviceConnections(t *testing.T) {
	hub := NewHub()

	first := &Client{hub: hub, UserID: "u1"}
	second := &Client{hub: hub, UserID: "u1"}
	hub.Register(first)
	hub.Register(second)

	if hub.OnlineCount() != 1 {
		t.Fatalf("在线用户数应为 1，实际: %d", hub.OnlineCount())
	}
	if hub.ConnectionCount() != 2 {
		t.Fatalf("连接数应为 2，实际: %d", hub.ConnectionCount())
	}
	if len(hub.ClientsOf("u1")) != 2 {
		t.Fatal("同一账号应保留多个连接")
	}

	hub.Unregister(first)
	if !hub.IsOnline("u1") {
		t.Fatal("仍有连接时用户应保持在线")
	}
	hub.Unregister(second)
	if hub.IsOnline("u1") {
		t.Fatal("全部断开后用户应离线")
	}
}

func TestIsWebSocketUpgrade(t *testing.T) {
	cases := []struct {
		connection string
		upgrade    string
		expected   bool
	}{
		{"Upgrade", "websocket", true},
		{"keep-alive, Upgrade", "WebSocket", true},
		{"keep-alive", "websocket", false},
		{"Upgrade", "h2c", false},
	}
	for _, c := range cases {
		got := tokenListContains(c.connection, "upgrade") &&
			equalFoldTrim(c.upgrade, "websocket")
		if got != c.expected {
			t.Fatalf("connection=%q upgrade=%q 期望 %v，实际 %v", c.connection, c.upgrade, c.expected, got)
		}
	}
}
