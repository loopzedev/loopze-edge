// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

// Package nats provides an embedded NATS server with JetStream for the LOOPZE
// runtime. It is used for debug message persistence (streams), shared state
// (KV stores), and future fleet communication — but NOT for internal
// node-to-node message routing (which uses Go channels).
package nats

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Broker wraps an embedded NATS server with JetStream enabled and provides
// a client connection for runtime use (creating KV stores, streams, etc.).
type Broker struct {
	server *server.Server
	conn   *nats.Conn
	js     jetstream.JetStream
	opts   *server.Options
}

// Config holds configuration for the embedded NATS broker.
type Config struct {
	// DataDir is the base directory for JetStream storage.
	DataDir string

	// Port is the NATS client port. Use -1 for auto-assigned port (default).
	Port int
}

// New creates and starts an embedded NATS server with JetStream enabled.
// The server stores JetStream data under dataDir/jetstream.
func New(cfg Config) (*Broker, error) {
	storeDir := filepath.Join(cfg.DataDir, "jetstream")

	opts := &server.Options{
		ServerName: "loopze-embedded",
		Port:       cfg.Port,
		DontListen: cfg.Port == -1,
		NoSigs:     true,
		JetStream:  true,
		StoreDir:   storeDir,
	}

	ns, err := server.NewServer(opts)
	if err != nil {
		return nil, fmt.Errorf("nats: failed to create server: %w", err)
	}

	// Route NATS logs through slog.
	ns.SetLoggerV2(newSlogAdapter(), false, false, false)

	// Start the server.
	ns.Start()

	// Wait for the server to be ready.
	if !ns.ReadyForConnections(10 * time.Second) {
		ns.Shutdown()
		return nil, fmt.Errorf("nats: server not ready after 10s")
	}

	// For in-process connections when not listening on a TCP port.
	var nc *nats.Conn
	if cfg.Port == -1 {
		nc, err = nats.Connect("", nats.InProcessServer(ns))
	} else {
		nc, err = nats.Connect(ns.ClientURL())
	}
	if err != nil {
		ns.Shutdown()
		return nil, fmt.Errorf("nats: failed to connect internal client: %w", err)
	}

	slog.Debug("embedded NATS started",
		"in_process", cfg.Port == -1,
		"store_dir", storeDir,
	)

	// Create JetStream context.
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		ns.Shutdown()
		return nil, fmt.Errorf("nats: failed to create jetstream context: %w", err)
	}

	slog.Debug("NATS JetStream ready")

	return &Broker{
		server: ns,
		conn:   nc,
		js:     js,
		opts:   opts,
	}, nil
}

// Shutdown gracefully stops the internal client connection and the NATS server.
func (b *Broker) Shutdown() {
	if b.conn != nil {
		b.conn.Close()
	}
	if b.server != nil {
		b.server.Shutdown()
		b.server.WaitForShutdown()
		slog.Debug("embedded NATS stopped")
	}
}

// Conn returns the internal NATS client connection.
func (b *Broker) Conn() *nats.Conn {
	return b.conn
}

// JetStream returns the JetStream context for creating streams and KV stores.
func (b *Broker) JetStream() jetstream.JetStream {
	return b.js
}

// ClientURL returns the connection URL for external NATS clients (e.g. nats-cli).
// Returns empty string when running in-process mode (port -1).
func (b *Broker) ClientURL() string {
	if b.opts.DontListen {
		return ""
	}
	return b.server.ClientURL()
}

// InProcessServer returns the underlying NATS server for in-process client connections.
func (b *Broker) InProcessServer() *server.Server {
	return b.server
}

// SetupDebugStream creates (or updates) the JetStream stream for debug messages.
// The stream uses memory storage with a ring-buffer of 1000 messages.
// Subject pattern: debug.<flowID>.<nodeID>
func (b *Broker) SetupDebugStream(ctx context.Context) (jetstream.Stream, error) {
	stream, err := b.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      "DEBUG",
		Subjects:  []string{"debug.>"},
		Retention: jetstream.LimitsPolicy,
		MaxMsgs:   1000,
		Storage:   jetstream.MemoryStorage,
		Discard:   jetstream.DiscardOld,
	})
	if err != nil {
		return nil, fmt.Errorf("nats: failed to create debug stream: %w", err)
	}

	slog.Debug("NATS debug stream ready", "name", "DEBUG", "max_msgs", 1000)
	return stream, nil
}

// GetDebugMessages reads the last `limit` messages from the DEBUG JetStream
// stream. An optional subject filter (e.g. "debug.flow1.>" or
// "debug.flow1.node42") restricts which messages are returned.
// Messages are returned in chronological order (oldest → newest).
func (b *Broker) GetDebugMessages(ctx context.Context, limit int, subject string) ([][]byte, error) {
	stream, err := b.js.Stream(ctx, "DEBUG")
	if err != nil {
		return nil, fmt.Errorf("nats: stream lookup: %w", err)
	}

	info, err := stream.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("nats: stream info: %w", err)
	}

	if info.State.Msgs == 0 {
		return nil, nil
	}

	// Calculate the sequence to start from so we deliver at most `limit` msgs.
	lastSeq := info.State.LastSeq
	firstSeq := info.State.FirstSeq
	startSeq := uint64(1)
	if lastSeq >= uint64(limit) && lastSeq-uint64(limit)+1 >= firstSeq {
		startSeq = lastSeq - uint64(limit) + 1
	} else {
		startSeq = firstSeq
	}

	filterSubjects := []string{"debug.>"}
	if subject != "" {
		filterSubjects = []string{subject}
	}

	consumer, err := stream.OrderedConsumer(ctx, jetstream.OrderedConsumerConfig{
		DeliverPolicy:  jetstream.DeliverByStartSequencePolicy,
		OptStartSeq:    startSeq,
		FilterSubjects: filterSubjects,
	})
	if err != nil {
		return nil, fmt.Errorf("nats: create consumer: %w", err)
	}

	batch, err := consumer.FetchNoWait(limit)
	if err != nil {
		return nil, fmt.Errorf("nats: fetch: %w", err)
	}

	var results [][]byte
	for msg := range batch.Messages() {
		data := make([]byte, len(msg.Data()))
		copy(data, msg.Data())
		results = append(results, data)
	}

	return results, nil
}

// ContextStore defines the two storage modes for context KV buckets.
// Users can choose between persistent (file-backed, survives restarts)
// and memory (fast, volatile, lost on restart).
type ContextStore string

const (
	// ContextStoreMemory uses in-memory storage — fast but volatile.
	ContextStoreMemory ContextStore = "memory"
	// ContextStorePersistent uses file-backed storage — slower but survives restarts.
	ContextStorePersistent ContextStore = "persistent"
)

func (cs ContextStore) jetStreamStorage() jetstream.StorageType {
	if cs == ContextStorePersistent {
		return jetstream.FileStorage
	}
	return jetstream.MemoryStorage
}

// SetupContextKV creates the global context KV buckets used by all flows
// for shared state (global.get/global.set in Function Nodes).
// Two buckets are created: one memory-backed (fast) and one file-backed (persistent).
func (b *Broker) SetupContextKV(ctx context.Context) (memory jetstream.KeyValue, persistent jetstream.KeyValue, err error) {
	memory, err = b.js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:  "context-global-memory",
		Storage: jetstream.MemoryStorage,
		History: 1,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("nats: failed to create global memory context KV: %w", err)
	}

	persistent, err = b.js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:  "context-global-persistent",
		Storage: jetstream.FileStorage,
		History: 1,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("nats: failed to create global persistent context KV: %w", err)
	}

	slog.Debug("NATS global context KV ready", "buckets", "memory + persistent")
	return memory, persistent, nil
}

// SetupSessionKV creates (or updates) the JetStream KV bucket used by the
// auth package to persist user sessions. File-storage so sessions survive
// a restart; per-entry TTL = ttl, refreshed on every Put (sliding window).
func (b *Broker) SetupSessionKV(ctx context.Context, ttl time.Duration) (jetstream.KeyValue, error) {
	kv, err := b.js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:  "auth-sessions",
		Storage: jetstream.FileStorage,
		History: 1,
		TTL:     ttl,
	})
	if err != nil {
		return nil, fmt.Errorf("nats: failed to create session KV: %w", err)
	}
	slog.Debug("NATS session KV ready", "bucket", "auth-sessions", "ttl", ttl)
	return kv, nil
}

// SetupFlowContextKV creates (or updates) KV buckets for a specific flow's
// context state (flow.get/flow.set in Function Nodes).
// Two buckets are created per flow: memory and persistent.
func (b *Broker) SetupFlowContextKV(ctx context.Context, flowID string) (memory jetstream.KeyValue, persistent jetstream.KeyValue, err error) {
	memory, err = b.js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:  "context-flow-" + flowID + "-memory",
		Storage: jetstream.MemoryStorage,
		History: 1,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("nats: failed to create flow memory context KV for %q: %w", flowID, err)
	}

	persistent, err = b.js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:  "context-flow-" + flowID + "-persistent",
		Storage: jetstream.FileStorage,
		History: 1,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("nats: failed to create flow persistent context KV for %q: %w", flowID, err)
	}

	slog.Debug("NATS flow context KV ready", "flow_id", flowID, "buckets", "memory + persistent")
	return memory, persistent, nil
}

// slogAdapter bridges NATS server logging to Go's slog.
type slogAdapter struct{}

func newSlogAdapter() *slogAdapter { return &slogAdapter{} }

// isJetStreamBanner reports whether a Noticef message is part of the JetStream
// ASCII banner that nats-server prints on startup. Such lines contain only
// box-drawing characters or the docs URL and add no operational value.
func isJetStreamBanner(msg string) bool {
	trimmed := strings.TrimSpace(msg)
	if strings.Contains(trimmed, "https://docs.nats.io/jetstream") {
		return true
	}
	for _, r := range trimmed {
		switch r {
		case '_', '|', '/', '\\', ' ':
		default:
			return false
		}
	}
	return true
}

func (s *slogAdapter) Noticef(format string, v ...any) {
	msg := fmt.Sprintf(format, v...)
	if isJetStreamBanner(msg) {
		return
	}
	// NATS notices are operational chatter, not LOOPZE-level info — route them
	// to debug so they only surface when the user runs with -log-level=debug.
	slog.Debug(msg, "component", "nats")
}

func (s *slogAdapter) Warnf(format string, v ...any) {
	slog.Warn(fmt.Sprintf(format, v...), "component", "nats")
}

func (s *slogAdapter) Fatalf(format string, v ...any) {
	slog.Error(fmt.Sprintf(format, v...), "component", "nats")
}

func (s *slogAdapter) Errorf(format string, v ...any) {
	slog.Error(fmt.Sprintf(format, v...), "component", "nats")
}

func (s *slogAdapter) Debugf(format string, v ...any) {
	slog.Debug(fmt.Sprintf(format, v...), "component", "nats")
}

func (s *slogAdapter) Tracef(format string, v ...any) {
	slog.Debug(fmt.Sprintf(format, v...), "component", "nats")
}
