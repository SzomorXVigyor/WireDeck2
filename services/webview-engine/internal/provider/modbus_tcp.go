package provider

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"

	"github.com/SzomorXVigyor/WireDeck2/pkg/wirerelay"
	pb "github.com/SzomorXVigyor/WireDeck2/pkg/wirerelay/v1"
	"github.com/SzomorXVigyor/WireDeck2/services/webview-engine/internal/models"
)

const (
	ProtocolModbusTCP = "MODBUS_TCP"

	fcReadCoils              byte = 0x01
	fcReadDiscreteInputs     byte = 0x02
	fcReadHoldingRegisters   byte = 0x03
	fcReadInputRegisters     byte = 0x04
	fcWriteSingleCoil        byte = 0x05
	fcWriteSingleRegister    byte = 0x06
	fcWriteMultipleRegisters byte = 0x10

	maxCoalesceRegisters = 125
	maxCoalesceBits      = 2000
	maxCoalesceGapRegs   = 8
	maxCoalesceGapBits   = 32

	defaultRequestTimeout = 2500 * time.Millisecond
)

// RunnerAuthLookup resolves the gRPC address and auth token for a site's WireRelay runner.
type RunnerAuthLookup func(ctx context.Context, siteID uuid.UUID) (addr string, token string, err error)

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

// ModbusTCPProvider implements Provider for MODBUS_TCP devices over WireRelay gRPC tunnels.
type ModbusTCPProvider struct {
	authLookup    RunnerAuthLookup
	devDirectDial bool
	idleTeardown  time.Duration

	mu      sync.Mutex
	devices map[uuid.UUID]*tcpDeviceClient
}

func NewModbusTCPProvider(authLookup RunnerAuthLookup, devDirectDial bool, idleTeardown time.Duration) *ModbusTCPProvider {
	if idleTeardown <= 0 {
		idleTeardown = 5 * time.Minute
	}
	return &ModbusTCPProvider{
		authLookup:    authLookup,
		devDirectDial: devDirectDial,
		idleTeardown:  idleTeardown,
		devices:       make(map[uuid.UUID]*tcpDeviceClient),
	}
}

func (p *ModbusTCPProvider) Protocol() string { return ProtocolModbusTCP }

// ValidateAttributes validates and normalizes MODBUS_TCP protocolAttributes.
func (p *ModbusTCPProvider) ValidateAttributes(attrs map[string]any) (map[string]any, RegisterSummary, error) {
	parsed, err := ParseModbusTCPAttributes(attrs)
	if err != nil {
		return nil, RegisterSummary{}, err
	}
	norm := map[string]any{
		"slaveAddress":    parsed.SlaveAddress,
		"functionCode":    parsed.FunctionCode,
		"registerAddress": parsed.RegisterAddress,
		"dataType":        parsed.DataType,
		"wordOrder":       parsed.WordOrder,
		"accessMode":      parsed.AccessMode,
	}
	return norm, RegisterSummary{
		FunctionCode:    parsed.FunctionCode,
		RegisterAddress: parsed.RegisterAddress,
		RegisterCount:   RegisterCountForType(parsed.DataType),
		DataType:        parsed.DataType,
		WordOrder:       parsed.WordOrder,
		AccessMode:      parsed.AccessMode,
	}, nil
}

// ParseModbusTCPAttributes extracts and validates MODBUS_TCP attributes from dynamic JSONB.
func ParseModbusTCPAttributes(attrs map[string]any) (models.ModbusTCPAttributes, error) {
	if attrs == nil {
		return models.ModbusTCPAttributes{}, errors.New("protocolAttributes is required for MODBUS_TCP")
	}

	slave, ok := asInt(attrs["slaveAddress"])
	if !ok {
		slave = 1
	}
	if slave < 0 || slave > 255 {
		return models.ModbusTCPAttributes{}, fmt.Errorf("slaveAddress %d out of range [0, 255]", slave)
	}

	fc, _ := attrs["functionCode"].(string)
	if _, err := readFCByte(fc); err != nil {
		return models.ModbusTCPAttributes{}, err
	}

	addr, ok := asInt(attrs["registerAddress"])
	if !ok || addr < 0 || addr > 65535 {
		return models.ModbusTCPAttributes{}, fmt.Errorf("registerAddress out of range [0, 65535]")
	}

	dataType, _ := attrs["dataType"].(string)
	switch dataType {
	case "BOOL", "UINT16", "INT16", "UINT32", "INT32", "FLOAT32":
	default:
		return models.ModbusTCPAttributes{}, fmt.Errorf("unsupported dataType %q", dataType)
	}
	if (fc == "READ_COIL" || fc == "READ_DISCRETE") && dataType != "BOOL" {
		dataType = "BOOL"
	}

	wordOrder, _ := attrs["wordOrder"].(string)
	if wordOrder == "" {
		wordOrder = "BIG"
	}
	if wordOrder != "BIG" && wordOrder != "LITTLE" {
		return models.ModbusTCPAttributes{}, fmt.Errorf("unsupported wordOrder %q (must be BIG or LITTLE)", wordOrder)
	}

	accessMode, _ := attrs["accessMode"].(string)
	if accessMode == "" {
		accessMode = "READ"
	}
	if accessMode != "READ" && accessMode != "READ_WRITE" {
		return models.ModbusTCPAttributes{}, fmt.Errorf("unsupported accessMode %q (must be READ or READ_WRITE)", accessMode)
	}
	if (fc == "READ_DISCRETE" || fc == "READ_INPUT") && accessMode == "READ_WRITE" {
		return models.ModbusTCPAttributes{}, fmt.Errorf("%s registers are read-only in Modbus", fc)
	}

	return models.ModbusTCPAttributes{
		SlaveAddress:    slave,
		FunctionCode:    fc,
		RegisterAddress: addr,
		DataType:        dataType,
		WordOrder:       wordOrder,
		AccessMode:      accessMode,
	}, nil
}

func RegisterCountForType(dataType string) int {
	switch dataType {
	case "UINT32", "INT32", "FLOAT32":
		return 2
	default:
		return 1
	}
}

func (p *ModbusTCPProvider) ReadOne(ctx context.Context, dev models.Device, mapping models.RegisterMapping) (float64, error) {
	attrs, err := ParseModbusTCPAttributes(mapping.ProtocolAttributes)
	if err != nil {
		return 0, err
	}
	fc, _ := readFCByte(attrs.FunctionCode)
	qty := uint16(RegisterCountForType(attrs.DataType))
	if isBitFC(fc) {
		qty = 1
	}
	respPDU, err := p.exchange(ctx, dev, byte(attrs.SlaveAddress), buildReadPDU(fc, uint16(attrs.RegisterAddress), qty))
	if err != nil {
		return 0, err
	}
	payload, err := parseReadResponsePDU(fc, respPDU)
	if err != nil {
		return 0, err
	}
	return decodeValue(attrs, payload, 0)
}

func (p *ModbusTCPProvider) ReadBatch(ctx context.Context, dev models.Device, mappings []models.RegisterMapping) ([]ReadResult, bool) {
	batches := coalesceModbusReads(mappings)
	results := make([]ReadResult, 0, len(mappings))
	var hadTransportErr bool

	for _, b := range batches {
		reqPDU := buildReadPDU(b.fc, b.startAddr, b.quantity)
		respPDU, err := p.exchange(ctx, dev, b.slaveAddress, reqPDU)
		if err != nil {
			var modbusErr *ExceptionError
			if !errors.As(err, &modbusErr) {
				hadTransportErr = true
			}
			for _, item := range b.items {
				results = append(results, ReadResult{MappingID: item.mapping.ID, Err: err})
			}
			continue
		}
		payload, err := parseReadResponsePDU(b.fc, respPDU)
		if err != nil {
			for _, item := range b.items {
				results = append(results, ReadResult{MappingID: item.mapping.ID, Err: err})
			}
			continue
		}
		for _, item := range b.items {
			offset := item.attrs.RegisterAddress - int(b.startAddr)
			val, decErr := decodeValue(item.attrs, payload, offset)
			if decErr != nil {
				results = append(results, ReadResult{MappingID: item.mapping.ID, Err: decErr})
				continue
			}
			vCopy := val
			results = append(results, ReadResult{MappingID: item.mapping.ID, Value: &vCopy})
		}
	}
	return results, hadTransportErr
}

func (p *ModbusTCPProvider) WriteOne(ctx context.Context, dev models.Device, mapping models.RegisterMapping, value float64) error {
	attrs, err := ParseModbusTCPAttributes(mapping.ProtocolAttributes)
	if err != nil {
		return err
	}
	if attrs.AccessMode != "READ_WRITE" {
		return errors.New("register mapping is read-only")
	}
	fc, reqPDU, err := buildWritePDU(attrs, value)
	if err != nil {
		return err
	}
	respPDU, err := p.exchange(ctx, dev, byte(attrs.SlaveAddress), reqPDU)
	if err != nil {
		return err
	}
	return parseWriteResponsePDU(fc, respPDU)
}

func (p *ModbusTCPProvider) DrainSite(siteID uuid.UUID) {
	p.mu.Lock()
	var list []*tcpDeviceClient
	for id, dc := range p.devices {
		if dc.siteID == siteID {
			list = append(list, dc)
			delete(p.devices, id)
		}
	}
	p.mu.Unlock()
	for _, dc := range list {
		dc.mu.Lock()
		dc.closeLocked()
		dc.mu.Unlock()
	}
}

func (p *ModbusTCPProvider) ReapIdle() {
	cutoff := time.Now().Add(-p.idleTeardown).UnixNano()
	p.mu.Lock()
	list := make([]*tcpDeviceClient, 0, len(p.devices))
	for _, dc := range p.devices {
		list = append(list, dc)
	}
	p.mu.Unlock()
	for _, dc := range list {
		if dc.lastUsed.Load() < cutoff {
			dc.mu.Lock()
			if dc.conn != nil && dc.lastUsed.Load() < cutoff {
				dc.closeLocked()
			}
			dc.mu.Unlock()
		}
	}
}

func (p *ModbusTCPProvider) Close() {
	p.mu.Lock()
	list := make([]*tcpDeviceClient, 0, len(p.devices))
	for id, dc := range p.devices {
		list = append(list, dc)
		delete(p.devices, id)
	}
	p.mu.Unlock()
	for _, dc := range list {
		dc.mu.Lock()
		dc.closeLocked()
		dc.mu.Unlock()
	}
}

// --- Per-device TCP / WireRelay Transport ---

type tcpDeviceClient struct {
	mu       sync.Mutex
	siteID   uuid.UUID
	deviceID uuid.UUID
	host     string
	port     int

	txSeq    atomic.Uint32
	lastUsed atomic.Int64

	conn       io.ReadWriteCloser
	grpcConn   *grpc.ClientConn
	cancelConn context.CancelFunc
}

func (c *tcpDeviceClient) closeLocked() {
	if c.cancelConn != nil {
		c.cancelConn()
		c.cancelConn = nil
	}
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
	if c.grpcConn != nil {
		_ = c.grpcConn.Close()
		c.grpcConn = nil
	}
}

func (p *ModbusTCPProvider) getOrCreate(dev models.Device) *tcpDeviceClient {
	p.mu.Lock()
	defer p.mu.Unlock()
	if dc, ok := p.devices[dev.ID]; ok {
		return dc
	}
	dc := &tcpDeviceClient{
		siteID:   dev.SiteID,
		deviceID: dev.ID,
		host:     dev.Host,
		port:     dev.Port,
	}
	dc.lastUsed.Store(time.Now().UnixNano())
	p.devices[dev.ID] = dc
	return dc
}

func (p *ModbusTCPProvider) exchange(ctx context.Context, dev models.Device, unitID byte, reqPDU []byte) ([]byte, error) {
	dc := p.getOrCreate(dev)
	dc.mu.Lock()
	defer dc.mu.Unlock()

	if dc.host != dev.Host || dc.port != dev.Port {
		dc.closeLocked()
		dc.host = dev.Host
		dc.port = dev.Port
	}

	reqCtx, cancel := context.WithTimeout(ctx, defaultRequestTimeout)
	defer cancel()

	for attempt := 0; attempt < 2; attempt++ {
		if dc.conn == nil {
			if err := p.dialLocked(reqCtx, dc); err != nil {
				return nil, err
			}
		}

		txID := uint16(dc.txSeq.Add(1))
		frame := buildMBAPFrame(txID, unitID, reqPDU)
		respPDU, err := exchangeMBAP(reqCtx, dc.conn, txID, unitID, frame)
		if err == nil {
			dc.lastUsed.Store(time.Now().UnixNano())
			return respPDU, nil
		}

		var modbusErr *ExceptionError
		if errors.As(err, &modbusErr) {
			dc.lastUsed.Store(time.Now().UnixNano())
			return nil, err
		}

		dc.closeLocked()
		if attempt == 1 || reqCtx.Err() != nil {
			return nil, err
		}
	}
	return nil, errors.New("modbus TCP exchange failed")
}

func (p *ModbusTCPProvider) dialLocked(ctx context.Context, dc *tcpDeviceClient) error {
	if p.devDirectDial {
		addr := net.JoinHostPort(dc.host, strconv.Itoa(dc.port))
		var d net.Dialer
		c, err := d.DialContext(ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("direct dial tcp %s: %w", addr, err)
		}
		dc.conn = c
		return nil
	}

	if p.authLookup == nil {
		return errors.New("runner auth lookup is not configured")
	}
	runnerAddr, authToken, err := p.authLookup(ctx, dc.siteID)
	if err != nil {
		return fmt.Errorf("resolve runner auth: %w", err)
	}

	gconn, client, err := wirerelay.Dial(runnerAddr)
	if err != nil {
		return fmt.Errorf("dial runner %s: %w", runnerAddr, err)
	}

	streamCtx, cancel := context.WithCancel(context.Background())
	streamCtx = wirerelay.WithAuthToken(streamCtx, authToken)
	stream, err := client.TunnelStream(streamCtx)
	if err != nil {
		cancel()
		_ = gconn.Close()
		return fmt.Errorf("open TunnelStream: %w", err)
	}

	if err := stream.Send(&pb.TunnelFrame{
		Frame: &pb.TunnelFrame_Header{
			Header: &pb.TunnelHeader{
				TargetHost: dc.host,
				TargetPort: uint32(dc.port),
				Protocol:   pb.TransportProtocol_TRANSPORT_PROTOCOL_TCP,
				TimeoutMs:  5000,
			},
		},
	}); err != nil {
		cancel()
		_ = gconn.Close()
		return fmt.Errorf("send TunnelHeader: %w", err)
	}

	type openRes struct {
		frame *pb.TunnelFrame
		err   error
	}
	resCh := make(chan openRes, 1)
	go func() {
		f, err := stream.Recv()
		resCh <- openRes{frame: f, err: err}
	}()

	var first *pb.TunnelFrame
	select {
	case <-ctx.Done():
		cancel()
		_ = gconn.Close()
		return fmt.Errorf("tunnel open timeout: %w", ctx.Err())
	case r := <-resCh:
		if r.err != nil {
			cancel()
			_ = gconn.Close()
			return fmt.Errorf("recv TunnelAck: %w", r.err)
		}
		first = r.frame
	}

	ack := first.GetAck()
	if ack == nil || !ack.GetSuccess() {
		cancel()
		_ = gconn.Close()
		msg := "tunnel rejected"
		if ack != nil && ack.GetErrorMessage() != "" {
			msg = ack.GetErrorMessage()
		}
		return errors.New(msg)
	}

	dc.grpcConn = gconn
	dc.cancelConn = cancel
	dc.conn = &tunnelStreamConn{stream: stream, cancel: cancel}
	return nil
}

func exchangeMBAP(ctx context.Context, rw io.ReadWriter, expectedTxID uint16, expectedUnit byte, frame []byte) ([]byte, error) {
	type result struct {
		pdu []byte
		err error
	}
	ch := make(chan result, 1)

	go func() {
		if _, err := rw.Write(frame); err != nil {
			ch <- result{err: fmt.Errorf("write MBAP frame: %w", err)}
			return
		}
		var hdr [7]byte
		if _, err := io.ReadFull(rw, hdr[:]); err != nil {
			ch <- result{err: fmt.Errorf("read MBAP header: %w", err)}
			return
		}
		txID := binary.BigEndian.Uint16(hdr[0:2])
		protoID := binary.BigEndian.Uint16(hdr[2:4])
		length := binary.BigEndian.Uint16(hdr[4:6])
		respUnit := hdr[6]
		if protoID != 0 {
			ch <- result{err: fmt.Errorf("invalid MBAP protocol ID %d", protoID)}
			return
		}
		if length < 2 || length > 260 {
			ch <- result{err: fmt.Errorf("invalid MBAP length %d", length)}
			return
		}
		pdu := make([]byte, int(length)-1)
		if _, err := io.ReadFull(rw, pdu); err != nil {
			ch <- result{err: fmt.Errorf("read Modbus PDU: %w", err)}
			return
		}
		if txID != expectedTxID {
			ch <- result{err: fmt.Errorf("MBAP transaction ID mismatch: got %d, want %d", txID, expectedTxID)}
			return
		}
		// Reject stray frames from another slave on a shared RS485 gateway (allow 0/255 wildcard)
		if expectedUnit != 0 && expectedUnit != 255 && respUnit != expectedUnit {
			ch <- result{err: fmt.Errorf("modbus unit ID mismatch: got %d, expected %d (pdu=%s)", respUnit, expectedUnit, hex.EncodeToString(pdu))}
			return
		}
		ch <- result{pdu: pdu}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		return r.pdu, r.err
	}
}

type tunnelStreamConn struct {
	stream pb.WireRelayService_TunnelStreamClient
	cancel context.CancelFunc
	buf    []byte
}

func (t *tunnelStreamConn) Read(p []byte) (int, error) {
	if len(t.buf) > 0 {
		n := copy(p, t.buf)
		t.buf = t.buf[n:]
		return n, nil
	}
	for {
		frame, err := t.stream.Recv()
		if err != nil {
			return 0, err
		}
		if frame.GetClose() != nil {
			return 0, io.EOF
		}
		if data := frame.GetPayload(); len(data) > 0 {
			n := copy(p, data)
			if n < len(data) {
				t.buf = append(t.buf[:0], data[n:]...)
			}
			return n, nil
		}
	}
}

func (t *tunnelStreamConn) Write(p []byte) (int, error) {
	cp := make([]byte, len(p))
	copy(cp, p)
	if err := t.stream.Send(&pb.TunnelFrame{
		Frame: &pb.TunnelFrame_Payload{Payload: cp},
	}); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (t *tunnelStreamConn) Close() error {
	_ = t.stream.Send(&pb.TunnelFrame{Frame: &pb.TunnelFrame_Close{Close: &pb.TunnelClose{Reason: "webview_closed"}}})
	_ = t.stream.CloseSend()
	if t.cancel != nil {
		t.cancel()
	}
	return nil
}

// --- Modbus PDU Codec & Coalescer ---

func readFCByte(name string) (byte, error) {
	switch name {
	case "READ_COIL":
		return fcReadCoils, nil
	case "READ_DISCRETE":
		return fcReadDiscreteInputs, nil
	case "READ_HOLDING":
		return fcReadHoldingRegisters, nil
	case "READ_INPUT":
		return fcReadInputRegisters, nil
	default:
		return 0, fmt.Errorf("unsupported functionCode %q", name)
	}
}

func isBitFC(fc byte) bool {
	return fc == fcReadCoils || fc == fcReadDiscreteInputs
}

func buildMBAPFrame(txID uint16, unitID byte, pdu []byte) []byte {
	frame := make([]byte, 7+len(pdu))
	binary.BigEndian.PutUint16(frame[0:2], txID)
	binary.BigEndian.PutUint16(frame[2:4], 0)
	binary.BigEndian.PutUint16(frame[4:6], uint16(1+len(pdu)))
	frame[6] = unitID
	copy(frame[7:], pdu)
	return frame
}

func buildReadPDU(fc byte, startAddr, quantity uint16) []byte {
	pdu := make([]byte, 5)
	pdu[0] = fc
	binary.BigEndian.PutUint16(pdu[1:3], startAddr)
	binary.BigEndian.PutUint16(pdu[3:5], quantity)
	return pdu
}

func parseReadResponsePDU(expectedFC byte, pdu []byte) ([]byte, error) {
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
	if byteCount == 0 || len(pdu) != 2+byteCount {
		return nil, fmt.Errorf("malformed modbus read response (byteCount=%d, pduLen=%d, hex=%s)", byteCount, len(pdu), hex.EncodeToString(pdu))
	}
	return pdu[2 : 2+byteCount], nil
}

func decodeValue(attrs models.ModbusTCPAttributes, payload []byte, regOffset int) (float64, error) {
	fc, err := readFCByte(attrs.FunctionCode)
	if err != nil {
		return 0, err
	}
	if isBitFC(fc) {
		byteIdx := regOffset / 8
		bitIdx := uint(regOffset % 8)
		if byteIdx < 0 || byteIdx >= len(payload) {
			return 0, fmt.Errorf("bit offset %d out of range for %d payload bytes", regOffset, len(payload))
		}
		if (payload[byteIdx] & (1 << bitIdx)) != 0 {
			return 1, nil
		}
		return 0, nil
	}

	count := RegisterCountForType(attrs.DataType)
	startByte := regOffset * 2
	endByte := startByte + count*2
	if startByte < 0 || endByte > len(payload) {
		return 0, fmt.Errorf("register offset %d (count %d) out of range for %d bytes", regOffset, count, len(payload))
	}

	rawRegs := make([]uint16, count)
	for i := 0; i < count; i++ {
		rawRegs[i] = binary.BigEndian.Uint16(payload[startByte+i*2 : startByte+i*2+2])
	}

	switch attrs.DataType {
	case "BOOL":
		if rawRegs[0] != 0 {
			return 1, nil
		}
		return 0, nil
	case "UINT16":
		return float64(rawRegs[0]), nil
	case "INT16":
		return float64(int16(rawRegs[0])), nil
	case "UINT32":
		return float64(combine32(rawRegs[0], rawRegs[1], attrs.WordOrder)), nil
	case "INT32":
		return float64(int32(combine32(rawRegs[0], rawRegs[1], attrs.WordOrder))), nil
	case "FLOAT32":
		f := math.Float32frombits(combine32(rawRegs[0], rawRegs[1], attrs.WordOrder))
		if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
			return 0, errors.New("decoded IEEE-754 float32 is NaN or Inf")
		}
		return float64(f), nil
	default:
		return 0, fmt.Errorf("unsupported dataType %q", attrs.DataType)
	}
}

func buildWritePDU(attrs models.ModbusTCPAttributes, value float64) (byte, []byte, error) {
	addr := uint16(attrs.RegisterAddress)

	if attrs.FunctionCode == "READ_COIL" {
		pdu := make([]byte, 5)
		pdu[0] = fcWriteSingleCoil
		binary.BigEndian.PutUint16(pdu[1:3], addr)
		if value != 0 {
			binary.BigEndian.PutUint16(pdu[3:5], 0xFF00)
		}
		return fcWriteSingleCoil, pdu, nil
	}

	regs, err := encodeRegisters(attrs, value)
	if err != nil {
		return 0, nil, err
	}
	if len(regs) == 1 {
		pdu := make([]byte, 5)
		pdu[0] = fcWriteSingleRegister
		binary.BigEndian.PutUint16(pdu[1:3], addr)
		binary.BigEndian.PutUint16(pdu[3:5], regs[0])
		return fcWriteSingleRegister, pdu, nil
	}

	byteCount := len(regs) * 2
	pdu := make([]byte, 6+byteCount)
	pdu[0] = fcWriteMultipleRegisters
	binary.BigEndian.PutUint16(pdu[1:3], addr)
	binary.BigEndian.PutUint16(pdu[3:5], uint16(len(regs)))
	pdu[5] = byte(byteCount)
	for i, r := range regs {
		binary.BigEndian.PutUint16(pdu[6+i*2:6+i*2+2], r)
	}
	return fcWriteMultipleRegisters, pdu, nil
}

func parseWriteResponsePDU(expectedFC byte, pdu []byte) error {
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

func encodeRegisters(attrs models.ModbusTCPAttributes, val float64) ([]uint16, error) {
	switch attrs.DataType {
	case "BOOL":
		if val != 0 {
			return []uint16{1}, nil
		}
		return []uint16{0}, nil
	case "UINT16":
		r := math.Round(val)
		if r < 0 || r > 65535 {
			return nil, fmt.Errorf("value %.4f out of UINT16 range [0, 65535]", val)
		}
		return []uint16{uint16(r)}, nil
	case "INT16":
		r := math.Round(val)
		if r < -32768 || r > 32767 {
			return nil, fmt.Errorf("value %.4f out of INT16 range [-32768, 32767]", val)
		}
		return []uint16{uint16(int16(r))}, nil
	case "UINT32":
		r := math.Round(val)
		if r < 0 || r > math.MaxUint32 {
			return nil, fmt.Errorf("value %.4f out of UINT32 range [0, 4294967295]", val)
		}
		return split32(uint32(r), attrs.WordOrder), nil
	case "INT32":
		r := math.Round(val)
		if r < math.MinInt32 || r > math.MaxInt32 {
			return nil, fmt.Errorf("value %.4f out of INT32 range [-2147483648, 2147483647]", val)
		}
		return split32(uint32(int32(r)), attrs.WordOrder), nil
	case "FLOAT32":
		return split32(math.Float32bits(float32(val)), attrs.WordOrder), nil
	default:
		return nil, fmt.Errorf("unsupported dataType %q", attrs.DataType)
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

type batchItem struct {
	mapping models.RegisterMapping
	attrs   models.ModbusTCPAttributes
}

type modbusBatch struct {
	slaveAddress byte
	fc           byte
	startAddr    uint16
	quantity     uint16
	items        []batchItem
}

type groupKey struct {
	slave byte
	fc    byte
}

func coalesceModbusReads(mappings []models.RegisterMapping) []modbusBatch {
	groups := make(map[groupKey][]batchItem)
	for _, m := range mappings {
		attrs, err := ParseModbusTCPAttributes(m.ProtocolAttributes)
		if err != nil {
			continue
		}
		fc, err := readFCByte(attrs.FunctionCode)
		if err != nil {
			continue
		}
		k := groupKey{slave: byte(attrs.SlaveAddress), fc: fc}
		groups[k] = append(groups[k], batchItem{mapping: m, attrs: attrs})
	}

	keys := make([]groupKey, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].slave != keys[j].slave {
			return keys[i].slave < keys[j].slave
		}
		return keys[i].fc < keys[j].fc
	})

	var batches []modbusBatch
	for _, k := range keys {
		list := groups[k]
		sort.Slice(list, func(i, j int) bool {
			return list[i].attrs.RegisterAddress < list[j].attrs.RegisterAddress
		})

		maxSpan := maxCoalesceRegisters
		maxGap := maxCoalesceGapRegs
		if isBitFC(k.fc) {
			maxSpan = maxCoalesceBits
			maxGap = maxCoalesceGapBits
		}

		var cur *modbusBatch
		for _, item := range list {
			cnt := RegisterCountForType(item.attrs.DataType)
			if isBitFC(k.fc) {
				cnt = 1
			}
			mStart := item.attrs.RegisterAddress
			mEnd := mStart + cnt

			if cur == nil {
				cur = &modbusBatch{
					slaveAddress: k.slave,
					fc:           k.fc,
					startAddr:    uint16(mStart),
					quantity:     uint16(cnt),
					items:        []batchItem{item},
				}
				continue
			}

			curEnd := int(cur.startAddr) + int(cur.quantity)
			gap := mStart - curEnd
			newEnd := curEnd
			if mEnd > newEnd {
				newEnd = mEnd
			}
			newSpan := newEnd - int(cur.startAddr)

			if gap <= maxGap && newSpan <= maxSpan {
				cur.quantity = uint16(newSpan)
				cur.items = append(cur.items, item)
			} else {
				batches = append(batches, *cur)
				cur = &modbusBatch{
					slaveAddress: k.slave,
					fc:           k.fc,
					startAddr:    uint16(mStart),
					quantity:     uint16(cnt),
					items:        []batchItem{item},
				}
			}
		}
		if cur != nil {
			batches = append(batches, *cur)
		}
	}
	return batches
}

func asInt(v any) (int, bool) {
	switch t := v.(type) {
	case int:
		return t, true
	case int64:
		return int(t), true
	case float64:
		return int(t), true
	case string:
		n, err := strconv.Atoi(t)
		return n, err == nil
	default:
		return 0, false
	}
}
