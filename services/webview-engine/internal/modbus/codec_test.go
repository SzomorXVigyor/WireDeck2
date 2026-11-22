package modbus

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

func TestCodec_DecodeAllTypesAndOrders(t *testing.T) {
	// Holding registers payload:
	// [0] = 0x00FA (250) -> UINT16 with scale 0.1 + offset 5 => 30.0
	// [1] = 0xFFF6 (-10) -> INT16 => -10
	// [2..3] = IEEE-754 float32(23.5) = 0x41BC0000 (BIG word order)
	// [4..5] = IEEE-754 float32(23.5) in LITTLE word order (0x0000, 0x41BC)
	f32Bits := math.Float32bits(23.5)
	payload := make([]byte, 12)
	binary.BigEndian.PutUint16(payload[0:2], 250)
	neg10 := int16(-10)
	binary.BigEndian.PutUint16(payload[2:4], uint16(neg10))
	binary.BigEndian.PutUint16(payload[4:6], uint16(f32Bits>>16))
	binary.BigEndian.PutUint16(payload[6:8], uint16(f32Bits&0xFFFF))
	binary.BigEndian.PutUint16(payload[8:10], uint16(f32Bits&0xFFFF))
	binary.BigEndian.PutUint16(payload[10:12], uint16(f32Bits>>16))

	v, err := DecodeValue(models.RegisterMapping{
		FunctionCode: "READ_HOLDING", DataType: "UINT16", Scale: 0.1, OffsetVal: 5,
	}, payload, 0)
	if err != nil || math.Abs(v-30.0) > 1e-6 {
		t.Fatalf("UINT16 scaled got %v, err=%v", v, err)
	}

	v, err = DecodeValue(models.RegisterMapping{
		FunctionCode: "READ_HOLDING", DataType: "INT16", Scale: 1,
	}, payload, 1)
	if err != nil || v != -10 {
		t.Fatalf("INT16 got %v, err=%v", v, err)
	}

	v, err = DecodeValue(models.RegisterMapping{
		FunctionCode: "READ_HOLDING", DataType: "FLOAT32", WordOrder: "BIG", ByteOrder: "BIG", Scale: 1,
	}, payload, 2)
	if err != nil || math.Abs(v-23.5) > 1e-5 {
		t.Fatalf("FLOAT32 BIG got %v, err=%v", v, err)
	}

	v, err = DecodeValue(models.RegisterMapping{
		FunctionCode: "READ_HOLDING", DataType: "FLOAT32", WordOrder: "LITTLE", ByteOrder: "BIG", Scale: 1,
	}, payload, 4)
	if err != nil || math.Abs(v-23.5) > 1e-5 {
		t.Fatalf("FLOAT32 LITTLE got %v, err=%v", v, err)
	}
}

func TestCodec_WriteRoundTripFloat32AndCoil(t *testing.T) {
	mFloat := models.RegisterMapping{
		FunctionCode:      "READ_HOLDING",
		WriteFunctionCode: "WRITE_MULTIPLE_REGISTERS",
		RegisterAddress:   100,
		DataType:          "FLOAT32",
		WordOrder:         "LITTLE",
		ByteOrder:         "BIG",
		Scale:             1,
	}
	fc, pdu, err := BuildWritePDU(mFloat, 42.25)
	if err != nil {
		t.Fatal(err)
	}
	if fc != FCWriteMultipleRegisters || pdu[0] != FCWriteMultipleRegisters || pdu[5] != 4 {
		t.Fatalf("unexpected FC16 PDU: %x", pdu)
	}
	got, err := DecodeValue(mFloat, pdu[6:10], 0)
	if err != nil || math.Abs(got-42.25) > 1e-4 {
		t.Fatalf("round-trip FLOAT32 failed: got %v, err=%v", got, err)
	}

	mCoil := models.RegisterMapping{
		FunctionCode:      "READ_COIL",
		WriteFunctionCode: "WRITE_COIL",
		RegisterAddress:   7,
		DataType:          "BOOL",
	}
	fc, pdu, err = BuildWritePDU(mCoil, 1)
	if err != nil || fc != FCWriteSingleCoil || binary.BigEndian.Uint16(pdu[3:5]) != 0xFF00 {
		t.Fatalf("unexpected coil write PDU: fc=%x pdu=%x err=%v", fc, pdu, err)
	}
}

func TestCoalesceReads_ContiguousAndGapSplit(t *testing.T) {
	mappings := []models.RegisterMapping{
		{Name: "a", FunctionCode: "READ_HOLDING", RegisterAddress: 10, DataType: "UINT16"},
		{Name: "b", FunctionCode: "READ_HOLDING", RegisterAddress: 14, DataType: "FLOAT32"}, // gap=3 <= 8 -> merged (10..16, qty=6)
		{Name: "c", FunctionCode: "READ_HOLDING", RegisterAddress: 50, DataType: "UINT16"},  // gap=34 > 8 -> separate batch
		{Name: "d", FunctionCode: "READ_COIL", RegisterAddress: 0, DataType: "BOOL"},
		{Name: "e", FunctionCode: "READ_COIL", RegisterAddress: 5, DataType: "BOOL"},
	}
	batches := CoalesceReads(mappings)
	if len(batches) != 3 {
		t.Fatalf("expected 3 batches (1 coil + 2 holding), got %d: %+v", len(batches), batches)
	}
	if batches[0].FunctionCode != FCReadCoils || batches[0].StartAddress != 0 || batches[0].Quantity != 6 {
		t.Fatalf("unexpected coil batch: %+v", batches[0])
	}
	if batches[1].FunctionCode != FCReadHoldingRegisters || batches[1].StartAddress != 10 || batches[1].Quantity != 6 {
		t.Fatalf("unexpected holding batch 1: %+v", batches[1])
	}
	if batches[2].FunctionCode != FCReadHoldingRegisters || batches[2].StartAddress != 50 || batches[2].Quantity != 1 {
		t.Fatalf("unexpected holding batch 2: %+v", batches[2])
	}
}

func TestDevicePool_DirectTCPExchangeWithMockPLC(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	// Mock Modbus TCP PLC serving holding register 0 = 1234 and accepting FC06 writes
	var reg0 uint16 = 1234
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
					case FCReadHoldingRegisters:
						resp := []byte{FCReadHoldingRegisters, 2, byte(reg0 >> 8), byte(reg0)}
						_, _ = conn.Write(BuildMBAPFrame(txID, unitID, resp))
					case FCWriteSingleRegister:
						reg0 = binary.BigEndian.Uint16(pdu[3:5])
						_, _ = conn.Write(BuildMBAPFrame(txID, unitID, pdu))
					}
				}
			}(c)
		}
	}()

	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)
	dev := models.Device{
		ID:               uuid.New(),
		SiteID:           uuid.New(),
		Name:             "MockPLC",
		Host:             host,
		Port:             port,
		Protocol:         "MODBUS_TCP",
		UnitID:           1,
		RequestTimeoutMs: 1000,
		IsActive:         true,
	}

	pool := NewDevicePool(nil, true, time.Minute)
	defer pool.Close()

	// 1. Read holding register 0
	respPDU, err := pool.Exchange(context.Background(), dev, BuildReadPDU(FCReadHoldingRegisters, 0, 1))
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	payload, err := ParseReadResponsePDU(FCReadHoldingRegisters, respPDU)
	if err != nil {
		t.Fatalf("parse read failed: %v", err)
	}
	m := models.RegisterMapping{FunctionCode: "READ_HOLDING", DataType: "UINT16", Scale: 1}
	val, err := DecodeValue(m, payload, 0)
	if err != nil || val != 1234 {
		t.Fatalf("expected 1234, got %v (err=%v)", val, err)
	}

	// 2. Write 5678 and read back
	fc, wpdu, err := BuildWritePDU(m, 5678)
	if err != nil {
		t.Fatal(err)
	}
	wresp, err := pool.Exchange(context.Background(), dev, wpdu)
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if err := ParseWriteResponsePDU(fc, wresp); err != nil {
		t.Fatalf("parse write resp failed: %v", err)
	}

	respPDU, err = pool.Exchange(context.Background(), dev, BuildReadPDU(FCReadHoldingRegisters, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	payload, _ = ParseReadResponsePDU(FCReadHoldingRegisters, respPDU)
	val, _ = DecodeValue(m, payload, 0)
	if val != 5678 {
		t.Fatalf("expected 5678 after write, got %v", val)
	}
}
