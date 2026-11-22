package modbus

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
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

// RunnerAuthLookup resolves the gRPC address and auth token for a site's WireRelay runner.
type RunnerAuthLookup func(ctx context.Context, siteID uuid.UUID) (addr string, token string, err error)

// DevicePool maintains 1 persistent WireRelay TunnelStream (or direct TCP/UDP socket in dev mode)
// per Modbus Device with per-device request serialization and idle teardown (D6, D26).
type DevicePool struct {
	authLookup    RunnerAuthLookup
	devDirectDial bool
	idleTeardown  time.Duration

	mu      sync.Mutex
	devices map[uuid.UUID]*deviceClient
}

func NewDevicePool(authLookup RunnerAuthLookup, devDirectDial bool, idleTeardown time.Duration) *DevicePool {
	if idleTeardown <= 0 {
		idleTeardown = 5 * time.Minute
	}
	return &DevicePool{
		authLookup:    authLookup,
		devDirectDial: devDirectDial,
		idleTeardown:  idleTeardown,
		devices:       make(map[uuid.UUID]*deviceClient),
	}
}

type deviceClient struct {
	mu       sync.Mutex // Serializes Modbus transactions per device (max_concurrent_requests=1 default)
	siteID   uuid.UUID
	deviceID uuid.UUID
	host     string
	port     int
	protocol string

	txSeq    atomic.Uint32
	lastUsed atomic.Int64

	conn       io.ReadWriteCloser
	grpcConn   *grpc.ClientConn
	cancelConn context.CancelFunc
}

func (c *deviceClient) closeLocked() {
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

// Exchange sends one Modbus PDU to dev and returns the response PDU.
func (p *DevicePool) Exchange(ctx context.Context, dev models.Device, reqPDU []byte) ([]byte, error) {
	dc := p.getOrCreate(dev)
	dc.mu.Lock()
	defer dc.mu.Unlock()

	// If device endpoint config changed, drop stale stream
	if dc.host != dev.Host || dc.port != dev.Port || dc.protocol != dev.Protocol {
		dc.closeLocked()
		dc.host = dev.Host
		dc.port = dev.Port
		dc.protocol = dev.Protocol
	}

	timeout := time.Duration(dev.RequestTimeoutMs) * time.Millisecond
	if timeout < 200*time.Millisecond {
		timeout = 2 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Try once on existing connection; if it fails mid-flight, reconnect and retry once cleanly.
	for attempt := 0; attempt < 2; attempt++ {
		if dc.conn == nil {
			if err := p.dialLocked(reqCtx, dc); err != nil {
				return nil, err
			}
		}

		txID := uint16(dc.txSeq.Add(1))
		frame := BuildMBAPFrame(txID, byte(dev.UnitID), reqPDU)
		respPDU, err := exchangeFrame(reqCtx, dc.conn, txID, frame)
		if err == nil {
			dc.lastUsed.Store(time.Now().UnixNano())
			return respPDU, nil
		}

		// Modbus protocol exception is NOT a transport failure - return immediately without reconnect.
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
	return nil, errors.New("modbus exchange failed")
}

// DrainSite tears down all open device streams for a site (conf rotation, deactivation, deletion).
func (p *DevicePool) DrainSite(siteID uuid.UUID) {
	p.mu.Lock()
	var targets []*deviceClient
	for id, dc := range p.devices {
		if dc.siteID == siteID {
			targets = append(targets, dc)
			delete(p.devices, id)
		}
	}
	p.mu.Unlock()

	for _, dc := range targets {
		dc.mu.Lock()
		dc.closeLocked()
		dc.mu.Unlock()
	}
}

// ReapIdle closes streams idle longer than idleTeardown to keep WireRelay stream counts low.
func (p *DevicePool) ReapIdle() {
	cutoff := time.Now().Add(-p.idleTeardown).UnixNano()
	p.mu.Lock()
	list := make([]*deviceClient, 0, len(p.devices))
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

// Close tears down all device connections on shutdown.
func (p *DevicePool) Close() {
	p.mu.Lock()
	list := make([]*deviceClient, 0, len(p.devices))
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

func (p *DevicePool) getOrCreate(dev models.Device) *deviceClient {
	p.mu.Lock()
	defer p.mu.Unlock()
	if dc, ok := p.devices[dev.ID]; ok {
		return dc
	}
	dc := &deviceClient{
		siteID:   dev.SiteID,
		deviceID: dev.ID,
		host:     dev.Host,
		port:     dev.Port,
		protocol: dev.Protocol,
	}
	dc.lastUsed.Store(time.Now().UnixNano())
	p.devices[dev.ID] = dc
	return dc
}

func (p *DevicePool) dialLocked(ctx context.Context, dc *deviceClient) error {
	network := "tcp"
	proto := pb.TransportProtocol_TRANSPORT_PROTOCOL_TCP
	if dc.protocol == "MODBUS_UDP" {
		network = "udp"
		proto = pb.TransportProtocol_TRANSPORT_PROTOCOL_UDP
	}

	if p.devDirectDial {
		addr := net.JoinHostPort(dc.host, strconv.Itoa(dc.port))
		var d net.Dialer
		c, err := d.DialContext(ctx, network, addr)
		if err != nil {
			return fmt.Errorf("direct dial %s %s: %w", network, addr, err)
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

	// Send TunnelHeader
	if err := stream.Send(&pb.TunnelFrame{
		Frame: &pb.TunnelFrame_Header{
			Header: &pb.TunnelHeader{
				TargetHost: dc.host,
				TargetPort: uint32(dc.port),
				Protocol:   proto,
				TimeoutMs:  5000,
			},
		},
	}); err != nil {
		cancel()
		_ = gconn.Close()
		return fmt.Errorf("send TunnelHeader: %w", err)
	}

	// Wait for TunnelAck confirmation bounded by ctx
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

func exchangeFrame(ctx context.Context, rw io.ReadWriter, expectedTxID uint16, frame []byte) ([]byte, error) {
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
		ch <- result{pdu: pdu}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		return r.pdu, r.err
	}
}

// tunnelStreamConn adapts WireRelay gRPC TunnelStream into an io.ReadWriteCloser.
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
