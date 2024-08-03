package mtproto

import (
	"crypto/rand"
	"net"
)

type MTProto struct {
	conn net.Conn
}

func (m *MTProto) Connect(addr string) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	m.conn = conn
	return nil
}

func (m *MTProto) GenerateNonce() ([]byte, error) {
	nonce := make([]byte, 16)
	_, err := rand.Read(nonce)
	if err != nil {
		return nil, err
	}
	return nonce, nil
}

func (m *MTProto) SendMessage(msg []byte) error {
	_, err := m.conn.Write(msg)
	return err
}

func (m *MTProto) ReadMessage() ([]byte, error) {
	buffer := make([]byte, 4096)
	n, err := m.conn.Read(buffer)
	if err != nil {
		return nil, err
	}
	return buffer[:n], nil
}
