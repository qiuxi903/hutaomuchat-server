// Package ws implements a minimal but complete RFC 6455 WebSocket server.
//
// The package is written against the Go standard library only so that the
// server builds without any external module. It supports the handshake,
// fragmented messages, ping/pong keep alive and the closing handshake.
package ws

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// WebSocket opcodes.
const (
	OpContinuation byte = 0x0
	OpText         byte = 0x1
	OpBinary       byte = 0x2
	OpClose        byte = 0x8
	OpPing         byte = 0x9
	OpPong         byte = 0xA
)

// Close status codes.
const (
	CloseNormal          uint16 = 1000
	CloseGoingAway       uint16 = 1001
	CloseProtocolError   uint16 = 1002
	CloseUnsupportedData uint16 = 1003
	CloseInvalidPayload  uint16 = 1007
	ClosePolicyViolation uint16 = 1008
	CloseMessageTooBig   uint16 = 1009
	CloseInternalError   uint16 = 1011
	CloseBanned          uint16 = 4003
	CloseUnauthorized    uint16 = 4001
)

// maxPayloadSize limits the size of a single incoming message.
const maxPayloadSize = 8 << 20 // 8 MiB

// handshakeGUID is the magic value defined by RFC 6455.
const handshakeGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// ErrNotWebSocket is returned when the request is not a WebSocket upgrade.
var ErrNotWebSocket = errors.New("请求不是有效的 WebSocket 升级")

// Conn is an established WebSocket connection.
type Conn struct {
	conn net.Conn
	br   *bufio.Reader

	writeMu sync.Mutex
	closed  bool
	closeMu sync.Mutex
}

// IsWebSocketUpgrade reports whether the request asks for a WebSocket upgrade.
func IsWebSocketUpgrade(r *http.Request) bool {
	return tokenListContains(r.Header.Get("Connection"), "upgrade") &&
		strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket")
}

func tokenListContains(header, token string) bool {
	for _, part := range strings.Split(header, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}

// Upgrade performs the WebSocket handshake and returns the established
// connection. The caller becomes responsible for closing it.
func Upgrade(w http.ResponseWriter, r *http.Request) (*Conn, error) {
	if !IsWebSocketUpgrade(r) {
		return nil, ErrNotWebSocket
	}
	if r.Header.Get("Sec-Websocket-Version") != "13" {
		return nil, errors.New("不支持的 WebSocket 版本")
	}
	key := r.Header.Get("Sec-Websocket-Key")
	if key == "" {
		return nil, errors.New("缺少 Sec-WebSocket-Key")
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("当前连接不支持 WebSocket 升级")
	}
	netConn, _, err := hijacker.Hijack()
	if err != nil {
		return nil, fmt.Errorf("劫持连接失败: %w", err)
	}

	sum := sha1.Sum([]byte(key + handshakeGUID))
	accept := base64.StdEncoding.EncodeToString(sum[:])

	response := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
	if _, err := netConn.Write([]byte(response)); err != nil {
		netConn.Close()
		return nil, err
	}

	return &Conn{
		conn: netConn,
		br:   bufio.NewReaderSize(netConn, 4096),
	}, nil
}

// ReadMessage returns the next complete application message.
//
// Fragmented messages are reassembled, control frames (ping, pong, close) are
// handled transparently.
func (c *Conn) ReadMessage() (byte, []byte, error) {
	var (
		assembled  []byte
		msgOpcode  byte
		fragmented bool
	)
	for {
		fin, opcode, payload, err := c.readFrame()
		if err != nil {
			return 0, nil, err
		}
		switch opcode {
		case OpPing:
			if err := c.WriteMessage(OpPong, payload); err != nil {
				return 0, nil, err
			}
			continue
		case OpPong:
			continue
		case OpClose:
			code := CloseNormal
			reason := ""
			if len(payload) >= 2 {
				code = binary.BigEndian.Uint16(payload[:2])
				reason = string(payload[2:])
			}
			_ = c.Close(code, reason)
			return OpClose, payload, io.EOF
		case OpText, OpBinary:
			if fragmented {
				return 0, nil, errors.New("收到新的消息但上一条尚未结束")
			}
			if fin {
				return opcode, payload, nil
			}
			fragmented = true
			msgOpcode = opcode
			assembled = append(assembled, payload...)
		case OpContinuation:
			if !fragmented {
				return 0, nil, errors.New("收到非预期的续帧")
			}
			assembled = append(assembled, payload...)
			if len(assembled) > maxPayloadSize {
				return 0, nil, errors.New("消息体过大")
			}
			if fin {
				return msgOpcode, assembled, nil
			}
		default:
			return 0, nil, fmt.Errorf("未知的操作码: %d", opcode)
		}
	}
}

// readFrame reads a single frame from the wire.
func (c *Conn) readFrame() (fin bool, opcode byte, payload []byte, err error) {
	var header [2]byte
	if _, err = io.ReadFull(c.br, header[:]); err != nil {
		return false, 0, nil, err
	}
	fin = header[0]&0x80 != 0
	opcode = header[0] & 0x0F
	masked := header[1]&0x80 != 0
	length := int64(header[1] & 0x7F)

	switch length {
	case 126:
		var ext [2]byte
		if _, err = io.ReadFull(c.br, ext[:]); err != nil {
			return false, 0, nil, err
		}
		length = int64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err = io.ReadFull(c.br, ext[:]); err != nil {
			return false, 0, nil, err
		}
		length = int64(binary.BigEndian.Uint64(ext[:]))
	}

	if length < 0 || length > maxPayloadSize {
		return false, 0, nil, errors.New("消息体过大")
	}
	if !masked {
		// RFC 6455 requires client frames to be masked.
		return false, 0, nil, errors.New("客户端帧未掩码")
	}

	var mask [4]byte
	if _, err = io.ReadFull(c.br, mask[:]); err != nil {
		return false, 0, nil, err
	}
	payload = make([]byte, length)
	if length > 0 {
		if _, err = io.ReadFull(c.br, payload); err != nil {
			return false, 0, nil, err
		}
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return fin, opcode, payload, nil
}

// WriteMessage sends an application message.
func (c *Conn) WriteMessage(opcode byte, data []byte) error {
	c.closeMu.Lock()
	closed := c.closed
	c.closeMu.Unlock()
	if closed {
		return errors.New("连接已关闭")
	}

	header := make([]byte, 0, 10)
	header = append(header, 0x80|opcode)
	length := len(data)
	switch {
	case length < 126:
		header = append(header, byte(length))
	case length <= 0xFFFF:
		header = append(header, 126, byte(length>>8), byte(length))
	default:
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(length))
		header = append(header, 127)
		header = append(header, ext[:]...)
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.conn.SetWriteDeadline(time.Now().Add(20 * time.Second)); err != nil {
		return err
	}
	if _, err := c.conn.Write(header); err != nil {
		return err
	}
	if len(data) > 0 {
		if _, err := c.conn.Write(data); err != nil {
			return err
		}
	}
	return nil
}

// WriteText sends a text message.
func (c *Conn) WriteText(data []byte) error { return c.WriteMessage(OpText, data) }

// Ping sends a ping frame.
func (c *Conn) Ping() error { return c.WriteMessage(OpPing, nil) }

// SetReadDeadline sets the deadline of the next read operation.
func (c *Conn) SetReadDeadline(t time.Time) error { return c.conn.SetReadDeadline(t) }

// SetWriteDeadline sets the deadline of the next write operation.
func (c *Conn) SetWriteDeadline(t time.Time) error { return c.conn.SetWriteDeadline(t) }

// Close performs the closing handshake and releases the connection.
func (c *Conn) Close(code uint16, reason string) error {
	c.closeMu.Lock()
	if c.closed {
		c.closeMu.Unlock()
		return nil
	}
	c.closed = true
	c.closeMu.Unlock()

	payload := make([]byte, 2+len(reason))
	binary.BigEndian.PutUint16(payload[:2], code)
	copy(payload[2:], reason)

	c.writeMu.Lock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	_, _ = c.conn.Write(append([]byte{0x80 | OpClose}, payload...))
	c.writeMu.Unlock()

	return c.conn.Close()
}

// RemoteAddr returns the address of the peer.
func (c *Conn) RemoteAddr() net.Addr { return c.conn.RemoteAddr() }
