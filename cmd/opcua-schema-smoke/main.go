// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

// opcua-schema-smoke probes whether a server exposes the schema metadata
// that LOOPZE's ExtensionObject decoder relies on:
//
//  1. DataType attribute of the variable (gives us the DataType NodeID)
//  2. DataTypeDefinition attribute of the DataType (gives us the
//     StructureDefinition / EnumDefinition with field layout and encoding ID)
//
// If step 2 fails, LOOPZE can't decode the ExtensionObject body even with
// the marker-type registered, because we have no schema to drive the binary
// codec.
//
// Usage:
//
//	go run ./cmd/opcua-schema-smoke opc.tcp://host:4840 'ns=2;s=Demo.Structures.SensorReading'
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/debug"
	"github.com/gopcua/opcua/ua"
)

func main() {
	if len(os.Args) < 3 {
		log.Fatal("usage: opcua-schema-smoke <endpointUrl> <variableNodeID>")
	}
	endpoint := os.Args[1]
	varNodeID := os.Args[2]

	if os.Getenv("OPCUA_DEBUG") != "" {
		debug.Enable = true
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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

	parsed, err := ua.ParseNodeID(varNodeID)
	if err != nil {
		log.Fatalf("ParseNodeID: %v", err)
	}

	// Step 1: read the DataType attribute of the variable.
	resp, err := client.Read(ctx, &ua.ReadRequest{
		NodesToRead: []*ua.ReadValueID{
			{NodeID: parsed, AttributeID: ua.AttributeIDDataType, DataEncoding: &ua.QualifiedName{}},
			{NodeID: parsed, AttributeID: ua.AttributeIDBrowseName, DataEncoding: &ua.QualifiedName{}},
			{NodeID: parsed, AttributeID: ua.AttributeIDDescription, DataEncoding: &ua.QualifiedName{}},
		},
	})
	if err != nil {
		log.Fatalf("Read: %v", err)
	}
	if len(resp.Results) < 3 {
		log.Fatalf("expected 3 results, got %d", len(resp.Results))
	}
	dtRes := resp.Results[0]
	if dtRes.Status != ua.StatusOK {
		log.Fatalf("DataType read status: %v", dtRes.Status)
	}
	dataType, ok := dtRes.Value.Value().(*ua.NodeID)
	if !ok || dataType == nil {
		log.Fatalf("DataType is not a NodeID: %T", dtRes.Value.Value())
	}
	fmt.Printf("Variable BrowseName: %v\n", resp.Results[1].Value.Value())
	fmt.Printf("Variable DataType:   %s\n", dataType.String())

	// Step 2: read the DataTypeDefinition attribute of the DataType node
	// itself. This is what LOOPZE's resolver leans on.
	defResp, err := client.Read(ctx, &ua.ReadRequest{
		NodesToRead: []*ua.ReadValueID{
			{NodeID: dataType, AttributeID: ua.AttributeIDDataTypeDefinition, DataEncoding: &ua.QualifiedName{}},
			{NodeID: dataType, AttributeID: ua.AttributeIDBrowseName, DataEncoding: &ua.QualifiedName{}},
			{NodeID: dataType, AttributeID: ua.AttributeIDIsAbstract, DataEncoding: &ua.QualifiedName{}},
		},
	})
	if err != nil {
		log.Fatalf("Read DataTypeDefinition: %v", err)
	}
	if len(defResp.Results) < 3 {
		log.Fatalf("expected 3 def-results, got %d", len(defResp.Results))
	}
	def := defResp.Results[0]
	bn := defResp.Results[1]
	abs := defResp.Results[2]

	fmt.Printf("DataType BrowseName: %v\n", bn.Value.Value())
	fmt.Printf("DataType IsAbstract: %v\n", abs.Value.Value())
	fmt.Printf("DataTypeDefinition status: %v\n", def.Status)
	if def.Status != ua.StatusOK {
		fmt.Println()
		fmt.Println("⚠ Server does not expose DataTypeDefinition for this DataType.")
		fmt.Println("  LOOPZE's schema-driven decoder cannot work without it.")
		fmt.Println("  Options: ask the server to expose DataTypeDefinition (1.04+ requirement)")
		fmt.Println("  or implement the browse-fallback path (Phase-5 issue, deferred).")
		os.Exit(2)
	}
	if def.Value == nil || def.Value.Value() == nil {
		fmt.Println("⚠ DataTypeDefinition status=Good but value is empty.")
		os.Exit(2)
	}
	extObj, ok := def.Value.Value().(*ua.ExtensionObject)
	if !ok || extObj == nil {
		fmt.Printf("⚠ DataTypeDefinition is not wrapped in an ExtensionObject: %T\n", def.Value.Value())
		os.Exit(2)
	}
	fmt.Printf("DataTypeDefinition wrapper: TypeID=%v EncodingMask=%d ValueGoType=%T\n",
		extObj.TypeID, extObj.EncodingMask, extObj.Value)

	// Step 3: dump whichever concrete kind we got.
	switch v := extObj.Value.(type) {
	case *ua.StructureDefinition:
		dumpStruct(v)
	case *ua.EnumDefinition:
		dumpEnum(v)
	default:
		fmt.Printf("⚠ Unexpected definition kind: %T\n", v)
	}
}

func dumpStruct(sd *ua.StructureDefinition) {
	fmt.Println()
	fmt.Println("✓ Got StructureDefinition")
	fmt.Printf("  StructureType:    %v\n", sd.StructureType)
	if sd.DefaultEncodingID != nil {
		fmt.Printf("  DefaultEncoding:  %s\n", sd.DefaultEncodingID.String())
	} else {
		fmt.Println("  ⚠ DefaultEncodingID is nil — LOOPZE cannot register the marker type without it.")
	}
	if sd.BaseDataType != nil {
		fmt.Printf("  BaseDataType:     %s\n", sd.BaseDataType.String())
	}
	fmt.Printf("  Field count:      %d\n", len(sd.Fields))
	for i, f := range sd.Fields {
		dt := "<nil>"
		if f.DataType != nil {
			dt = f.DataType.String()
		}
		fmt.Printf("    [%d] %s  DataType=%s ValueRank=%d Optional=%v\n",
			i, f.Name, dt, f.ValueRank, f.IsOptional)
	}
	// Pretty-print the whole thing as JSON for completeness.
	if pretty, err := json.MarshalIndent(sd, "  ", "  "); err == nil {
		fmt.Println()
		fmt.Println("Raw definition:")
		fmt.Println("  " + string(pretty))
	}
}

func dumpEnum(ed *ua.EnumDefinition) {
	fmt.Println()
	fmt.Println("✓ Got EnumDefinition")
	for _, f := range ed.Fields {
		fmt.Printf("    %d -> %s\n", f.Value, f.Name)
	}
}
