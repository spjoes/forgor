package sync

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
)

type Encoder struct {
	buf *bytes.Buffer
}

func NewEncoder() *Encoder {
	return &Encoder{buf: new(bytes.Buffer)}
}

func (e *Encoder) Bytes() []byte {
	return e.buf.Bytes()
}

func (e *Encoder) Reset() {
	e.buf.Reset()
}

func (e *Encoder) WriteU8(v uint8) {
	e.buf.WriteByte(v)
}

func (e *Encoder) WriteU32(v uint32) {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	e.buf.Write(b[:])
}

func (e *Encoder) WriteU64(v uint64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	e.buf.Write(b[:])
}

func (e *Encoder) WriteFixedBytes(b []byte, expectedLen int) error {
	if len(b) != expectedLen {
		return fmt.Errorf("expected %d bytes, got %d", expectedLen, len(b))
	}
	e.buf.Write(b)
	return nil
}

func (e *Encoder) WriteBytes(b []byte) {
	e.WriteU32(uint32(len(b)))
	e.buf.Write(b)
}

func (e *Encoder) WriteString(s string) {
	e.WriteU32(uint32(len(s)))
	e.buf.WriteString(s)
}

func (e *Encoder) WriteBool(v bool) {
	if v {
		e.buf.WriteByte(1)
	} else {
		e.buf.WriteByte(0)
	}
}

func (e *Encoder) WriteUUID(uuid []byte) error {
	return e.WriteFixedBytes(uuid, 16)
}

func (e *Encoder) WriteDeviceID(deviceID []byte) error {
	return e.WriteFixedBytes(deviceID, 32)
}

func (e *Encoder) WriteHash(hash []byte) error {
	return e.WriteFixedBytes(hash, 32)
}

func (e *Encoder) WriteSignature(sig []byte) error {
	return e.WriteFixedBytes(sig, 64)
}

func (e *Encoder) WriteNonce(nonce []byte) error {
	return e.WriteFixedBytes(nonce, 24)
}

func (e *Encoder) WritePublicKey(key []byte) error {
	return e.WriteFixedBytes(key, 32)
}

func (e *Encoder) WriteStringArray(arr []string) {
	e.WriteU32(uint32(len(arr)))
	for _, s := range arr {
		e.WriteString(s)
	}
}

type DeviceIDCounterEntry struct {
	DeviceID []byte
	Counter  uint64
}

func (e *Encoder) WriteDeviceIDCounterMap(entries []DeviceIDCounterEntry) error {
	sort.Slice(entries, func(i, j int) bool {
		return bytes.Compare(entries[i].DeviceID, entries[j].DeviceID) < 0
	})

	e.WriteU32(uint32(len(entries)))
	for _, entry := range entries {
		if err := e.WriteDeviceID(entry.DeviceID); err != nil {
			return err
		}
		e.WriteU64(entry.Counter)
	}
	return nil
}

type DeviceIDHashEntry struct {
	DeviceID []byte
	Hash     []byte
}

func (e *Encoder) WriteDeviceIDHashMap(entries []DeviceIDHashEntry) error {
	sort.Slice(entries, func(i, j int) bool {
		return bytes.Compare(entries[i].DeviceID, entries[j].DeviceID) < 0
	})

	e.WriteU32(uint32(len(entries)))
	for _, entry := range entries {
		if err := e.WriteDeviceID(entry.DeviceID); err != nil {
			return err
		}
		if err := e.WriteHash(entry.Hash); err != nil {
			return err
		}
	}
	return nil
}
