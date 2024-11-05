package sasl

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/apache/thrift/lib/go/thrift"
)

type TSaslTransport struct {
	rbuf *bytes.Buffer
	wbuf *bytes.Buffer

	trans thrift.TTransport
	sasl  Client
}

// Status is SASL negotiation status
type Status byte

// SASL negotiation statuses
const (
	StatusStart    Status = 1
	StatusOK       Status = 2
	StatusBad      Status = 3
	StatusError    Status = 4
	StatusComplete Status = 5
)

func NewTSaslTransport(t thrift.TTransport, opts *Options) (*TSaslTransport, error) {
	sasl := NewClient(opts)

	return &TSaslTransport{
		trans: t,
		sasl:  sasl,

		rbuf: bytes.NewBuffer(nil),
		wbuf: bytes.NewBuffer(nil),
	}, nil
}

func (t *TSaslTransport) IsOpen() bool {
	return t.trans.IsOpen()
}

func (t *TSaslTransport) Open() error {

	if !t.trans.IsOpen() {
		if err := t.trans.Open(); err != nil {
			return err
		}
	}

	mech, initial, _, err := t.sasl.Start([]string{MechPlain})
	if err != nil {
		return err
	}

	if err := t.negotiationSend(StatusStart, []byte(mech)); err != nil {
		return fmt.Errorf("sasl: negotiation failed. %v", err)
	}
	if err := t.negotiationSend(StatusOK, initial); err != nil {
		return fmt.Errorf("sasl: negotiation failed. %v", err)
	}

	for {
		status, challenge, err := t.recieve()
		if err != nil {
			return fmt.Errorf("sasl: negotiation failed. %v", err)
		}

		if status != StatusOK && status != StatusComplete {
			return fmt.Errorf("sasl: negotiation failed. bad status: %d", status)
		}

		if status == StatusComplete {
			break
		}

		payload, _, err := t.sasl.Step(challenge)
		if err != nil {
			return fmt.Errorf("sasl: negotiation failed. %v", err)
		}
		if err := t.negotiationSend(StatusOK, payload); err != nil {
			return fmt.Errorf("sasl: negotiation failed. %v", err)
		}

	}
	return nil

}

func (t *TSaslTransport) Read(buf []byte) (int, error) {
	n, err := t.rbuf.Read(buf)
	if err != nil && err != io.EOF {
		return 0, err
	}
	if err == io.EOF {
		return t.readFrame(buf)
	}
	return n, nil
}

func (t *TSaslTransport) readFrame(buf []byte) (int, error) {
	header := make([]byte, 4)
	_, err := t.trans.Read(header)
	if err != nil {
		return 0, err
	}

	l := binary.BigEndian.Uint32(header)

	body := make([]byte, l)
	_, err = io.ReadFull(t.trans, body)
	if err != nil {
		return 0, err
	}
	t.rbuf = bytes.NewBuffer(body)
	return t.rbuf.Read(buf)
}

func (t *TSaslTransport) Write(buf []byte) (int, error) {
	return t.wbuf.Write(buf)
}

func (t *TSaslTransport) Flush(ctx context.Context) error {

	in, err := ioutil.ReadAll(t.wbuf)
	if err != nil {
		return err
	}

	v := len(in)
	var payload []byte
	payload = append(payload, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	payload = append(payload, in...)

	t.trans.Write(payload)

	t.wbuf.Reset()
	return t.trans.Flush(ctx)
}

func (t *TSaslTransport) RemainingBytes() (num_bytes uint64) {
	return t.trans.RemainingBytes()
}

func (t *TSaslTransport) Close() error {
	t.sasl.Free()
	return t.trans.Close()
}

func (t *TSaslTransport) negotiationSend(status Status, body []byte) error {
	var payload []byte
	payload = append(payload, byte(status))
	v := len(body)
	payload = append(payload, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	payload = append(payload, body...)
	_, err := t.trans.Write(payload)
	if err != nil {
		return err
	}

	if err := t.trans.Flush(context.Background()); err != nil {
		return err
	}

	return nil
}

func (t *TSaslTransport) recieve() (Status, []byte, error) {
	header := make([]byte, 5)
	_, err := t.trans.Read(header)
	if err != nil {
		return 0, nil, err
	}
	return Status(header[0]), header[1:], nil
}
