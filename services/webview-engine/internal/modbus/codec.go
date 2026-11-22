package modbus

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/SzomorXVigyor/WireDeck2/services/webview-engine/internal/models"
)

const (
	FCReadCoils              byte = 0x01
	FCReadDiscreteInputs     byte = 0x02
	FCReadHoldingRegisters   byte = 0x03
	FCReadInputRegisters     byte = 0x04
	FCWriteSingleCoil        byte = 0x05
	FCWriteSingleRegister    byte = 0x06
	FCWriteMultipleCoils     byte = 0x0F
	FCWriteMultipleRegisters byte = 0x10

	MaxCoalesceRegisters = 125
	MaxCoalesceBits      = 2000
	MaxCoalesceGapRegs   = 8
	MaxCoalesceGapBits   = 32
)

// ExceptionError represents a standard Modbus protocol exception response.
type ExceptionError struct {
	FunctionCode  byte
	ExceptionCode byte
}

func (e *ExceptionError) Error() string {
	return fmt.Sprintf("modbus exception fc=0x%02x code=0x%02x (%s)", e.FunctionCode, e.ExceptionCode, exceptionName(e.ExceptionCode))
}

func exceptionName(code byte) string {
	switch code {
	case 0x01:
		return "illegal function"
	case 0x02:
		return "illegal data address"
	case 0x03:
		return "illegal data value"
	case 0x04:
		return "server device failure"
	case 0x05:
		return "acknowledge"
	case 0x06:
		return "server device busy"
	case 0x0A:
		return "gateway path unavailable"
	case 0x0B:
		return "gateway target failed to respond"
	default:
		return "unknown exception"
	}
}

// ReadFunctionByte converts a schema function_code string into its Modbus FC byte.
func ReadFunctionByte(name string) (byte, error) {
	switch name {
	case "READ_COIL":
		return FCReadCoils, nil
	case "READ_DISCRETE":
		return FCReadDiscreteInputs, nil
	case "READ_HOLDING":
		return FCReadHoldingRegisters, nil
	case "READ_INPUT":
		return FCReadInputRegisters, nil
	default:
		return 0, fmt.Errorf("unsupported read function code %q", name)
	}
}

// IsBitFunction reports whether fc operates on coils / discrete inputs.
func IsBitFunction(fc byte) bool {
	return fc == FCReadCoils || fc == FCReadDiscreteInputs
}

// RegisterCountForType returns the required 16-bit register (or 1-bit coil) count.
func RegisterCountForType(dataType string) int {
	switch dataType {
	case "UINT32", "INT32", "FLOAT32":
		return 2
	default:
		return 1
	}
}

// BuildMBAPFrame wraps a Modbus PDU with a 7-byte MBAP header for TCP/UDP transport.
func BuildMBAPFrame(txID uint16, unitID byte, pdu []byte) []byte {
	frame := make([]byte, 7+len(pdu))
	binary.BigEndian.PutUint16(frame[0:2], txID)
	binary.BigEndian.PutUint16(frame[2:4], 0) // Protocol ID = 0 (Modbus)
	binary.BigEndian.PutUint16(frame[4:6], uint16(1+len(pdu)))
	frame[6] = unitID
	copy(frame[7:], pdu)
	return frame
}

// BuildReadPDU builds a read request PDU for FC 0x01, 0x02, 0x03, or 0x04.
func BuildReadPDU(fc byte, startAddr, quantity uint16) []byte {
	pdu := make([]byte, 5)
	pdu[0] = fc
	binary.BigEndian.PutUint16(pdu[1:3], startAddr)
	binary.BigEndian.PutUint16(pdu[3:5], quantity)
	return pdu
}

// ParseReadResponsePDU validates a read response PDU and returns its payload bytes.
func ParseReadResponsePDU(expectedFC byte, pdu []byte) ([]byte, error) {
	if len(pdu) < 2 {
		return nil, errors.New("modbus response PDU too short")
	}
	fc := pdu[0]
	if fc == expectedFC|0x80 {
		return nil, &ExceptionError{FunctionCode: expectedFC, ExceptionCode: pdu[1]}
	}
	if fc != expectedFC {
		return nil, fmt.Errorf("unexpected modbus function code 0x%02x (expected 0x%02x)", fc, expectedFC)
	}
	byteCount := int(pdu[1])
	if len(pdu) < 2+byteCount {
		return nil, fmt.Errorf("truncated modbus read payload: got %d bytes, expected %d", len(pdu)-2, byteCount)
	}
	return pdu[2 : 2+byteCount], nil
}

// DecodeBit extracts bit index bitOffset (0-based from batch start) from coil/discrete payload bytes.
func DecodeBit(payload []byte, bitOffset int) (bool, error) {
	byteIdx := bitOffset / 8
	bitIdx := uint(bitOffset % 8)
	if byteIdx < 0 || byteIdx >= len(payload) {
		return false, fmt.Errorf("bit offset %d out of range for %d payload bytes", bitOffset, len(payload))
	}
	return (payload[byteIdx] & (1 << bitIdx)) != 0, nil
}

// DecodeValue decodes a typed, scaled float64 value from a coalesced read payload.
// regOffset is the mapping's offset from the batch's StartAddress (in bits for bit FCs, registers for register FCs).
func DecodeValue(m models.RegisterMapping, payload []byte, regOffset int) (float64, error) {
	fc, err := ReadFunctionByte(m.FunctionCode)
	if err != nil {
		return 0, err
	}
	if IsBitFunction(fc) {
		b, err := DecodeBit(payload, regOffset)
		if err != nil {
			return 0, err
		}
		if b {
			return 1, nil
		}
		return 0, nil
	}

	count := RegisterCountForType(m.DataType)
	startByte := regOffset * 2
	endByte := startByte + count*2
	if startByte < 0 || endByte > len(payload) {
		return 0, fmt.Errorf("register offset %d (count %d) out of range for %d bytes", regOffset, count, len(payload))
	}

	rawRegs := make([]uint16, count)
	for i := 0; i < count; i++ {
		hi := payload[startByte+i*2]
		lo := payload[startByte+i*2+1]
		if m.ByteOrder == "LITTLE" {
			rawRegs[i] = uint16(lo)<<8 | uint16(hi)
		} else {
			rawRegs[i] = uint16(hi)<<8 | uint16(lo)
		}
	}

	var raw float64
	switch m.DataType {
	case "BOOL":
		if rawRegs[0] != 0 {
			return 1, nil
		}
		return 0, nil
	case "UINT16":
		raw = float64(rawRegs[0])
	case "INT16":
		raw = float64(int16(rawRegs[0]))
	case "UINT32":
		raw = float64(combine32(rawRegs[0], rawRegs[1], m.WordOrder))
	case "INT32":
		raw = float64(int32(combine32(rawRegs[0], rawRegs[1], m.WordOrder)))
	case "FLOAT32":
		f := math.Float32frombits(combine32(rawRegs[0], rawRegs[1], m.WordOrder))
		if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
			return 0, errors.New("decoded IEEE-754 float32 is NaN or Inf")
		}
		raw = float64(f)
	default:
		return 0, fmt.Errorf("unsupported data type %q", m.DataType)
	}

	scale := m.Scale
	if scale == 0 {
		scale = 1
	}
	return raw*scale + m.OffsetVal, nil
}

// BuildWritePDU encodes a logical float64 value into the proper Modbus write PDU
// (FC 0x05 WRITE_COIL, FC 0x06 WRITE_REGISTER, or FC 0x10 WRITE_MULTIPLE_REGISTERS).
func BuildWritePDU(m models.RegisterMapping, value float64) (byte, []byte, error) {
	wfc := m.WriteFunctionCode
	if wfc == "" {
		// Infer sensible default write function code when omitted
		switch {
		case m.FunctionCode == "READ_COIL":
			wfc = "WRITE_COIL"
		case RegisterCountForType(m.DataType) == 2:
			wfc = "WRITE_MULTIPLE_REGISTERS"
		default:
			wfc = "WRITE_REGISTER"
		}
	}

	addr := uint16(m.RegisterAddress)

	switch wfc {
	case "WRITE_COIL":
		pdu := make([]byte, 5)
		pdu[0] = FCWriteSingleCoil
		binary.BigEndian.PutUint16(pdu[1:3], addr)
		if value != 0 {
			binary.BigEndian.PutUint16(pdu[3:5], 0xFF00)
		} else {
			binary.BigEndian.PutUint16(pdu[3:5], 0x0000)
		}
		return FCWriteSingleCoil, pdu, nil

	case "WRITE_REGISTER":
		regs, err := encodeRegisters(m, value)
		if err != nil {
			return 0, nil, err
		}
		if len(regs) != 1 {
			return 0, nil, fmt.Errorf("data type %s requires WRITE_MULTIPLE_REGISTERS (FC 0x10), not WRITE_REGISTER", m.DataType)
		}
		pdu := make([]byte, 5)
		pdu[0] = FCWriteSingleRegister
		binary.BigEndian.PutUint16(pdu[1:3], addr)
		writeWireWord(pdu[3:5], regs[0], m.ByteOrder)
		return FCWriteSingleRegister, pdu, nil

	case "WRITE_MULTIPLE_REGISTERS":
		regs, err := encodeRegisters(m, value)
		if err != nil {
			return 0, nil, err
		}
		byteCount := len(regs) * 2
		pdu := make([]byte, 6+byteCount)
		pdu[0] = FCWriteMultipleRegisters
		binary.BigEndian.PutUint16(pdu[1:3], addr)
		binary.BigEndian.PutUint16(pdu[3:5], uint16(len(regs)))
		pdu[5] = byte(byteCount)
		for i, r := range regs {
			writeWireWord(pdu[6+i*2:6+i*2+2], r, m.ByteOrder)
		}
		return FCWriteMultipleRegisters, pdu, nil

	default:
		return 0, nil, fmt.Errorf("unsupported write function code %q", wfc)
	}
}

// ParseWriteResponsePDU validates a write response PDU for FC 0x05, 0x06, or 0x10.
func ParseWriteResponsePDU(expectedFC byte, pdu []byte) error {
	if len(pdu) < 2 {
		return errors.New("modbus write response PDU too short")
	}
	if pdu[0] == expectedFC|0x80 {
		return &ExceptionError{FunctionCode: expectedFC, ExceptionCode: pdu[1]}
	}
	if pdu[0] != expectedFC {
		return fmt.Errorf("unexpected write response FC 0x%02x (expected 0x%02x)", pdu[0], expectedFC)
	}
	if len(pdu) < 5 {
		return errors.New("modbus write response truncated")
	}
	return nil
}

func encodeRegisters(m models.RegisterMapping, scaledVal float64) ([]uint16, error) {
	if m.DataType == "BOOL" {
		if scaledVal != 0 {
			return []uint16{1}, nil
		}
		return []uint16{0}, nil
	}
	scale := m.Scale
	if scale == 0 {
		scale = 1
	}
	raw := (scaledVal - m.OffsetVal) / scale

	switch m.DataType {
	case "UINT16":
		rounded := math.Round(raw)
		if rounded < 0 || rounded > 65535 {
			return nil, fmt.Errorf("value %.4f (raw %.2f) out of UINT16 range [0, 65535]", scaledVal, raw)
		}
		return []uint16{uint16(rounded)}, nil
	case "INT16":
		rounded := math.Round(raw)
		if rounded < -32768 || rounded > 32767 {
			return nil, fmt.Errorf("value %.4f (raw %.2f) out of INT16 range [-32768, 32767]", scaledVal, raw)
		}
		return []uint16{uint16(int16(rounded))}, nil
	case "UINT32":
		rounded := math.Round(raw)
		if rounded < 0 || rounded > math.MaxUint32 {
			return nil, fmt.Errorf("value %.4f out of UINT32 range", scaledVal)
		}
		return split32(uint32(rounded), m.WordOrder), nil
	case "INT32":
		rounded := math.Round(raw)
		if rounded < math.MinInt32 || rounded > math.MaxInt32 {
			return nil, fmt.Errorf("value %.4f out of INT32 range", scaledVal)
		}
		return split32(uint32(int32(rounded)), m.WordOrder), nil
	case "FLOAT32":
		return split32(math.Float32bits(float32(raw)), m.WordOrder), nil
	default:
		return nil, fmt.Errorf("unsupported data type %q", m.DataType)
	}
}

func combine32(w0, w1 uint16, wordOrder string) uint32 {
	if wordOrder == "LITTLE" {
		return uint32(w1)<<16 | uint32(w0)
	}
	return uint32(w0)<<16 | uint32(w1)
}

func split32(v uint32, wordOrder string) []uint16 {
	hi := uint16(v >> 16)
	lo := uint16(v & 0xFFFF)
	if wordOrder == "LITTLE" {
		return []uint16{lo, hi}
	}
	return []uint16{hi, lo}
}

func writeWireWord(dst []byte, v uint16, byteOrder string) {
	if byteOrder == "LITTLE" {
		dst[0] = byte(v & 0xFF)
		dst[1] = byte(v >> 8)
	} else {
		dst[0] = byte(v >> 8)
		dst[1] = byte(v & 0xFF)
	}
}

// ReadBatch represents a coalesced contiguous Modbus read covering one or more mappings.
type ReadBatch struct {
	FunctionCode byte
	StartAddress uint16
	Quantity     uint16
	Mappings     []models.RegisterMapping
}

// CoalesceReads merges mappings on the same device into contiguous Modbus read batches
// respecting Modbus protocol limits (<=125 registers / <=2000 bits) and small gap bridging (D6).
func CoalesceReads(mappings []models.RegisterMapping) []ReadBatch {
	byFC := make(map[byte][]models.RegisterMapping)
	for _, m := range mappings {
		fc, err := ReadFunctionByte(m.FunctionCode)
		if err != nil {
			continue
		}
		byFC[fc] = append(byFC[fc], m)
	}

	fcs := make([]byte, 0, len(byFC))
	for fc := range byFC {
		fcs = append(fcs, fc)
	}
	sort.Slice(fcs, func(i, j int) bool { return fcs[i] < fcs[j] })

	var batches []ReadBatch
	for _, fc := range fcs {
		list := byFC[fc]
		sort.Slice(list, func(i, j int) bool {
			return list[i].RegisterAddress < list[j].RegisterAddress
		})

		maxSpan := MaxCoalesceRegisters
		maxGap := MaxCoalesceGapRegs
		if IsBitFunction(fc) {
			maxSpan = MaxCoalesceBits
			maxGap = MaxCoalesceGapBits
		}

		var cur *ReadBatch
		for _, m := range list {
			cnt := RegisterCountForType(m.DataType)
			if IsBitFunction(fc) {
				cnt = 1
			}
			mStart := m.RegisterAddress
			mEnd := mStart + cnt

			if cur == nil {
				cur = &ReadBatch{
					FunctionCode: fc,
					StartAddress: uint16(mStart),
					Quantity:     uint16(cnt),
					Mappings:     []models.RegisterMapping{m},
				}
				continue
			}

			curEnd := int(cur.StartAddress) + int(cur.Quantity)
			gap := mStart - curEnd
			newEnd := curEnd
			if mEnd > newEnd {
				newEnd = mEnd
			}
			newSpan := newEnd - int(cur.StartAddress)

			if gap <= maxGap && newSpan <= maxSpan {
				cur.Quantity = uint16(newSpan)
				cur.Mappings = append(cur.Mappings, m)
			} else {
				batches = append(batches, *cur)
				cur = &ReadBatch{
					FunctionCode: fc,
					StartAddress: uint16(mStart),
					Quantity:     uint16(cnt),
					Mappings:     []models.RegisterMapping{m},
				}
			}
		}
		if cur != nil {
			batches = append(batches, *cur)
		}
	}
	return batches
}
