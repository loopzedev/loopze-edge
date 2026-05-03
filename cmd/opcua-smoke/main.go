// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

// opcua-smoke is a minimal standalone diagnostic that exercises Connect →
// Read → Subscribe → CreateMonitoredItems against a server, using nothing
// but gopcua/opcua itself. It's the lowest-overhead way to tell whether a
// hang sits in our wrappers or in the library / server.
//
// Usage:
//
//	OPCUA_DEBUG=1 go run ./cmd/opcua-smoke opc.tcp://host:4840 i=2258
//
// The first argument is the endpoint URL, the second the NodeID to subscribe
// to (defaults to CurrentTime, i=2258).
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/debug"
	"github.com/gopcua/opcua/ua"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: opcua-smoke <endpointUrl> [nodeId=i=2258]")
	}
	endpoint := os.Args[1]
	nodeIDStr := "i=2258"
	if len(os.Args) >= 3 {
		nodeIDStr = os.Args[2]
	}
	if os.Getenv("OPCUA_DEBUG") != "" {
		debug.Enable = true
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := opcua.NewClient(endpoint, opcua.SecurityMode(ua.MessageSecurityModeNone))
	if err != nil {
		log.Fatalf("NewClient: %v", err)
	}
	if err := client.Connect(ctx); err != nil {
		log.Fatalf("Connect: %v", err)
	}
	defer func() {
		closeCtx, cc := context.WithTimeout(context.Background(), 2*time.Second)
		_ = client.Close(closeCtx)
		cc()
	}()
	fmt.Println("Connected")

	parsed, err := ua.ParseNodeID(nodeIDStr)
	if err != nil {
		log.Fatalf("ParseNodeID: %v", err)
	}

	// Step 1: Read the value once — sanity check that basic services work.
	readCtx, readCancel := context.WithTimeout(ctx, 5*time.Second)
	resp, err := client.Read(readCtx, &ua.ReadRequest{
		TimestampsToReturn: ua.TimestampsToReturnBoth,
		NodesToRead: []*ua.ReadValueID{
			{NodeID: parsed, AttributeID: ua.AttributeIDValue},
		},
	})
	readCancel()
	if err != nil {
		log.Fatalf("Read: %v", err)
	}
	fmt.Printf("Read result: status=%v value=%v\n", resp.Results[0].Status, resp.Results[0].Value.Value())

	// Step 2: open a Subscription.
	notifyCh := make(chan *opcua.PublishNotificationData, 16)
	sub, err := client.Subscribe(ctx, &opcua.SubscriptionParameters{
		Interval:          500 * time.Millisecond,
		LifetimeCount:     60,
		MaxKeepAliveCount: 10,
	}, notifyCh)
	if err != nil {
		log.Fatalf("Subscribe: %v", err)
	}
	fmt.Printf("Subscribed: id=%d revisedInterval=%v\n", sub.SubscriptionID, sub.RevisedPublishingInterval)

	// Step 3: register a single MonitoredItem. This is the spot LOOPZE hangs at.
	monCtx, monCancel := context.WithTimeout(ctx, 5*time.Second)
	t0 := time.Now()
	monRes, err := sub.Monitor(monCtx, ua.TimestampsToReturnBoth, &ua.MonitoredItemCreateRequest{
		ItemToMonitor: &ua.ReadValueID{
			NodeID:       parsed,
			AttributeID:  ua.AttributeIDValue,
			DataEncoding: &ua.QualifiedName{}, // Prosys requires non-nil; nil triggers ERRF
		},
		MonitoringMode: ua.MonitoringModeReporting,
		RequestedParameters: &ua.MonitoringParameters{
			ClientHandle:     1,
			SamplingInterval: 1000,
			QueueSize:        1,
			DiscardOldest:    true,
		},
	})
	monCancel()
	fmt.Printf("Monitor took %v\n", time.Since(t0))
	if err != nil {
		log.Fatalf("Monitor: %v", err)
	}
	if len(monRes.Results) == 0 {
		log.Fatal("Monitor: empty results")
	}
	fmt.Printf("Monitor result: status=%v itemId=%d revisedSampling=%v\n",
		monRes.Results[0].StatusCode,
		monRes.Results[0].MonitoredItemID,
		monRes.Results[0].RevisedSamplingInterval)

	// Step 4: wait for notifications.
	fmt.Println("Waiting for notifications (10s)…")
	deadline := time.After(10 * time.Second)
	notifs := 0
	for {
		select {
		case data, ok := <-notifyCh:
			if !ok {
				fmt.Println("notifyCh closed")
				return
			}
			if data.Error != nil {
				fmt.Printf("  error: %v\n", data.Error)
				continue
			}
			if dcn, ok := data.Value.(*ua.DataChangeNotification); ok {
				notifs++
				for _, m := range dcn.MonitoredItems {
					if m.Value == nil || m.Value.Value == nil {
						fmt.Printf("  notif #%d: handle=%d <empty value>\n", notifs, m.ClientHandle)
						continue
					}
					v := m.Value.Value
					raw := v.Value()
					fmt.Printf("  notif #%d: handle=%d variant_type=%v go_type=%T status=%v\n",
						notifs, m.ClientHandle, v.Type(), raw, m.Value.Status)
					// Detail-print for ExtensionObjects: the wire-level TypeID
					// and what gopcua decoded the body into. *ua.ExtensionObject
					// with Value == nil means gopcua dropped the body because
					// the TypeID isn't registered.
					if eo, ok := raw.(*ua.ExtensionObject); ok && eo != nil {
						typeID := "<nil>"
						if eo.TypeID != nil && eo.TypeID.NodeID != nil {
							typeID = eo.TypeID.NodeID.String()
						}
						fmt.Printf("    extObj typeID=%s encodingMask=%d valueGoType=%T\n",
							typeID, eo.EncodingMask, eo.Value)
					} else {
						fmt.Printf("    value=%v\n", raw)
					}
				}
			}
		case <-deadline:
			fmt.Printf("done — received %d notifications\n", notifs)
			return
		}
	}
}
