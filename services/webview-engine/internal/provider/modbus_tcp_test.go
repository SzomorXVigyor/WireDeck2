package provider

import (
	"context"
	"encoding/binary"
	"io"
	"math"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/SzomorXVigyor/WireDeck2/services/webview-engine/internal/models"
)

func TestModbusTCPProvider_ValidateAndCoalesceBySlave(t *testing.T) {
	p := NewModbusTCPProvider(nil, true, time.Minute)
	norm, sum, err := p.ValidateAttributes(map[string]any{
		"slaveAddress":    7,
		"functionCode":    "READ_HOLDING",
		"registerAddress": 65,
		"dataType":        "FLOAT32",
		"wordOrder":       "LITTLE",
		"accessMode":      "READ_WRITE",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum.RegisterCount != 2 || norm["slaveAddress"] != 7 {
		t.Fatalf("unexpected normalized attrs: %+v %+v", norm, sum)
	}

	// Rejects write on READ_INPUT
	if _, _, err := p.ValidateAttributes(map[string]any{
		"slaveAddress":    1,
		"functionCode":    "READ_INPUT",
		"registerAddress": 10,
		"dataType":        "UINT16",
		"accessMode":      "READ_WRITE",
	}); err == nil {
		t.Fatal("expected error for READ_WRITE on READ_INPUT")
	}

	// Coalesces per (slaveAddress, functionCode)
	m1 := models.RegisterMapping{ID: uuid.New(), ProtocolAttributes: map[string]any{
		"slaveAddress": 1, "functionCode": "READ_HOLDING", "registerAddress": 10, "dataType": "UINT16", "accessMode": "READ",
	}}
	m2 := models.RegisterMapping{ID: uuid.New(), ProtocolAttributes: map[string]any{
		"slaveAddress": 1, "functionCode": "READ_HOLDING", "registerAddress": 12, "dataType": "UINT16", "accessMode": "READ",
	}}
	m3 := models.RegisterMapping{ID: uuid.New(), ProtocolAttributes: map[string]any{
		"slaveAddress": 2, "functionCode": "READ_HOLDING", "registerAddress": 10, "dataType": "UINT16", "accessMode": "READ",
	}}
	batches := coalesceModbusReads([]models.RegisterMapping{m1, m2, m3})
	if len(batches) != 2 {
		t.Fatalf("expected 2 batches (slave 1 coalesced + slave 2), got %d", len(batches))
	}
	if batches[0].slaveAddress != 1 || batches[0].quantity != 3 {
		t.Fatalf("unexpected slave 1 batch: %+v", batches[0])
	}
}

func TestModbusTCPProvider_MockPLCReadWriteAndUnitMismatch(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	var reg65 uint16 = 235
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				for {
					var hdr [7]byte
					if _, err := io.ReadFull(conn, hdr[:]); err != nil {
						return
					}
					txID := binary.BigEndian.Uint16(hdr[0:2])
					length := binary.BigEndian.Uint16(hdr[4:6])
					unitID := hdr[6]
					pdu := make([]byte, int(length)-1)
					if _, err := io.ReadFull(conn, pdu); err != nil {
						return
					}
					switch pdu[0] {
					case fcReadHoldingRegisters:
						if unitID == 9 {
							// Simulate noisy gateway returning wrong slave ID
							_, _ = conn.Write(buildMBAPFrame(txID, 10, []byte{fcReadHoldingRegisters, 0, 0xff, 0, 0x0d}))
							continue
						}
						resp := []byte{fcReadHoldingRegisters, 2, byte(reg65 >> 8), byte(reg65)}
						_, _ = conn.Write(buildMBAPFrame(txID, unitID, resp))
					case fcWriteSingleRegister:
						reg65 = binary.BigEndian.Uint16(pdu[3:5])
						_, _ = conn.Write(buildMBAPFrame(txID, unitID, pdu))
					}
				}
			}(c)
		}
	}()

	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)
	dev := models.Device{ID: uuid.New(), SiteID: uuid.New(), Name: "PLC", Host: host, Port: port, Protocol: ProtocolModbusTCP}
	p := NewModbusTCPProvider(nil, true, time.Minute)
	defer p.Close()

	m := models.RegisterMapping{
		ID: uuid.New(),
		ProtocolAttributes: map[string]any{
			"slaveAddress": 1, "functionCode": "READ_HOLDING", "registerAddress": 65,
			"dataType": "UINT16", "wordOrder": "BIG", "accessMode": "READ_WRITE",
		},
	}

	v, err := p.ReadOne(context.Background(), dev, m)
	if err != nil || math.Abs(v-235) > 1e-6 {
		t.Fatalf("ReadOne got %v err=%v", v, err)
	}

	if err := p.WriteOne(context.Background(), dev, m, 310); err != nil {
		t.Fatalf("WriteOne failed: %v", err)
	}
	v, err = p.ReadOne(context.Background(), dev, m)
	if err != nil || math.Abs(v-310) > 1e-6 {
		t.Fatalf("ReadOne after write got %v err=%v", v, err)
	}

	// Wrong unit ID frame must be rejected cleanly
	mBad := models.RegisterMapping{
		ID: uuid.New(),
		ProtocolAttributes: map[string]any{
			"slaveAddress": 9, "functionCode": "READ_HOLDING", "registerAddress": 65,
			"dataType": "UINT16", "wordOrder": "BIG", "accessMode": "READ",
		},
	}
	if _, err := p.ReadOne(context.Background(), dev, mBad); err == nil {
		t.Fatal("expected unit ID mismatch error")
	}
}
