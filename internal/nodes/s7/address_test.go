// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package s7

import (
	"strings"
	"testing"
)

func TestParseS7Address_Positive(t *testing.T) {
	cases := []struct {
		name     string
		addr     string
		dt       string
		want     S7Item
		wantSize int
	}{
		// ── DB ──
		{
			name: "DB bit",
			addr: "DB10.DBX2.3", dt: "bool",
			want:     S7Item{Area: S7AreaDB, WordLen: S7WLBit, DBNumber: 10, Start: 2, Bit: 3, Amount: 1, DataType: "bool"},
			wantSize: 1,
		},
		{
			name: "DB byte (byte type)",
			addr: "DB10.DBB4", dt: "byte",
			want:     S7Item{Area: S7AreaDB, WordLen: S7WLByte, DBNumber: 10, Start: 4, Amount: 1, DataType: "byte"},
			wantSize: 1,
		},
		{
			name: "DB byte (char alias)",
			addr: "DB10.DBB4", dt: "char",
			want:     S7Item{Area: S7AreaDB, WordLen: S7WLByte, DBNumber: 10, Start: 4, Amount: 1, DataType: "char"},
			wantSize: 1,
		},
		{
			name: "DB word (word type)",
			addr: "DB10.DBW6", dt: "word",
			want:     S7Item{Area: S7AreaDB, WordLen: S7WLWord, DBNumber: 10, Start: 6, Amount: 1, DataType: "word"},
			wantSize: 2,
		},
		{
			name: "DB word (int type — same wire layout)",
			addr: "DB10.DBW6", dt: "int",
			want:     S7Item{Area: S7AreaDB, WordLen: S7WLWord, DBNumber: 10, Start: 6, Amount: 1, DataType: "int"},
			wantSize: 2,
		},
		{
			name: "DB dword (dword type)",
			addr: "DB10.DBD0", dt: "dword",
			want:     S7Item{Area: S7AreaDB, WordLen: S7WLDWord, DBNumber: 10, Start: 0, Amount: 1, DataType: "dword"},
			wantSize: 4,
		},
		{
			name: "DB dword (dint type)",
			addr: "DB10.DBD0", dt: "dint",
			want:     S7Item{Area: S7AreaDB, WordLen: S7WLDWord, DBNumber: 10, Start: 0, Amount: 1, DataType: "dint"},
			wantSize: 4,
		},
		{
			name: "DB dword (real type)",
			addr: "DB10.DBD0", dt: "real",
			want:     S7Item{Area: S7AreaDB, WordLen: S7WLDWord, DBNumber: 10, Start: 0, Amount: 1, DataType: "real"},
			wantSize: 4,
		},
		{
			name: "DB STRING(20)",
			addr: "DB1.STRING50.20", dt: "string",
			want:     S7Item{Area: S7AreaDB, WordLen: S7WLByte, DBNumber: 1, Start: 50, Amount: 22, DataType: "string", StringMaxLen: 20},
			wantSize: 22,
		},
		{
			name: "DB STRING(254) — max",
			addr: "DB1.STRING0.254", dt: "string",
			want:     S7Item{Area: S7AreaDB, WordLen: S7WLByte, DBNumber: 1, Start: 0, Amount: 256, DataType: "string", StringMaxLen: 254},
			wantSize: 256,
		},

		// ── 64-bit S7-1500 types via the DBL form ──
		{
			name: "DB long (lreal)",
			addr: "DB10.DBL16", dt: "lreal",
			want:     S7Item{Area: S7AreaDB, WordLen: S7WLByte, DBNumber: 10, Start: 16, Amount: 8, DataType: "lreal"},
			wantSize: 8,
		},
		{
			name: "DB long (lint)",
			addr: "DB10.DBL24", dt: "lint",
			want:     S7Item{Area: S7AreaDB, WordLen: S7WLByte, DBNumber: 10, Start: 24, Amount: 8, DataType: "lint"},
			wantSize: 8,
		},
		{
			name: "DB long (ulint)",
			addr: "DB10.DBL32", dt: "ulint",
			want:     S7Item{Area: S7AreaDB, WordLen: S7WLByte, DBNumber: 10, Start: 32, Amount: 8, DataType: "ulint"},
			wantSize: 8,
		},
		{
			name: "DB long (lword)",
			addr: "DB10.DBL40", dt: "lword",
			want:     S7Item{Area: S7AreaDB, WordLen: S7WLByte, DBNumber: 10, Start: 40, Amount: 8, DataType: "lword"},
			wantSize: 8,
		},
		{
			name: "DB long (dt — BCD date+time)",
			addr: "DB10.DBL48", dt: "dt",
			want:     S7Item{Area: S7AreaDB, WordLen: S7WLByte, DBNumber: 10, Start: 48, Amount: 8, DataType: "dt"},
			wantSize: 8,
		},
		{
			name: "DB DTL (12 bytes structured date+time)",
			addr: "DB3.DTL171", dt: "dtl",
			want:     S7Item{Area: S7AreaDB, WordLen: S7WLByte, DBNumber: 3, Start: 171, Amount: 12, DataType: "dtl"},
			wantSize: 12,
		},

		// ── New aliases on the existing byte/word/dword forms ──
		{
			name: "DB byte (sint)",
			addr: "DB10.DBB1", dt: "sint",
			want:     S7Item{Area: S7AreaDB, WordLen: S7WLByte, DBNumber: 10, Start: 1, Amount: 1, DataType: "sint"},
			wantSize: 1,
		},
		{
			name: "DB byte (usint)",
			addr: "DB10.DBB2", dt: "usint",
			want:     S7Item{Area: S7AreaDB, WordLen: S7WLByte, DBNumber: 10, Start: 2, Amount: 1, DataType: "usint"},
			wantSize: 1,
		},
		{
			name: "DB word (uint)",
			addr: "DB10.DBW8", dt: "uint",
			want:     S7Item{Area: S7AreaDB, WordLen: S7WLWord, DBNumber: 10, Start: 8, Amount: 1, DataType: "uint"},
			wantSize: 2,
		},
		{
			name: "DB word (date — 2 bytes, days since 1990)",
			addr: "DB10.DBW141", dt: "date",
			want:     S7Item{Area: S7AreaDB, WordLen: S7WLWord, DBNumber: 10, Start: 141, Amount: 1, DataType: "date"},
			wantSize: 2,
		},
		{
			name: "DB word (wchar)",
			addr: "DB10.DBW59", dt: "wchar",
			want:     S7Item{Area: S7AreaDB, WordLen: S7WLWord, DBNumber: 10, Start: 59, Amount: 1, DataType: "wchar"},
			wantSize: 2,
		},
		{
			name: "DB dword (udint)",
			addr: "DB10.DBD18", dt: "udint",
			want:     S7Item{Area: S7AreaDB, WordLen: S7WLDWord, DBNumber: 10, Start: 18, Amount: 1, DataType: "udint"},
			wantSize: 4,
		},
		{
			name: "DB dword (time — signed ms duration)",
			addr: "DB10.DBD129", dt: "time",
			want:     S7Item{Area: S7AreaDB, WordLen: S7WLDWord, DBNumber: 10, Start: 129, Amount: 1, DataType: "time"},
			wantSize: 4,
		},
		{
			name: "DB dword (tod — ms since midnight)",
			addr: "DB10.DBD143", dt: "tod",
			want:     S7Item{Area: S7AreaDB, WordLen: S7WLDWord, DBNumber: 10, Start: 143, Amount: 1, DataType: "tod"},
			wantSize: 4,
		},

		// ── WSTRING (UCS-2 wide string) ──
		{
			name: "DB WSTRING(20)",
			addr: "DB1.WSTRING50.20", dt: "wstring",
			want:     S7Item{Area: S7AreaDB, WordLen: S7WLByte, DBNumber: 1, Start: 50, Amount: 44, DataType: "wstring", StringMaxLen: 20},
			wantSize: 44,
		},
		{
			name: "DB WSTRING(16382) — max",
			addr: "DB1.WSTRING0.16382", dt: "wstring",
			want:     S7Item{Area: S7AreaDB, WordLen: S7WLByte, DBNumber: 1, Start: 0, Amount: 32768, DataType: "wstring", StringMaxLen: 16382},
			wantSize: 32768,
		},

		// ── Merker ──
		{
			name: "M bit",
			addr: "M0.3", dt: "bool",
			want:     S7Item{Area: S7AreaMK, WordLen: S7WLBit, Start: 0, Bit: 3, Amount: 1, DataType: "bool"},
			wantSize: 1,
		},
		{
			name: "MB byte",
			addr: "MB10", dt: "byte",
			want:     S7Item{Area: S7AreaMK, WordLen: S7WLByte, Start: 10, Amount: 1, DataType: "byte"},
			wantSize: 1,
		},
		{
			name: "MW word",
			addr: "MW12", dt: "word",
			want:     S7Item{Area: S7AreaMK, WordLen: S7WLWord, Start: 12, Amount: 1, DataType: "word"},
			wantSize: 2,
		},
		{
			name: "MD real",
			addr: "MD16", dt: "real",
			want:     S7Item{Area: S7AreaMK, WordLen: S7WLDWord, Start: 16, Amount: 1, DataType: "real"},
			wantSize: 4,
		},

		// ── Inputs (PE) ──
		{
			name: "I bit",
			addr: "I0.0", dt: "bool",
			want:     S7Item{Area: S7AreaPE, WordLen: S7WLBit, Start: 0, Bit: 0, Amount: 1, DataType: "bool"},
			wantSize: 1,
		},
		{
			name: "IB byte",
			addr: "IB1", dt: "byte",
			want:     S7Item{Area: S7AreaPE, WordLen: S7WLByte, Start: 1, Amount: 1, DataType: "byte"},
			wantSize: 1,
		},
		{
			name: "IW int",
			addr: "IW2", dt: "int",
			want:     S7Item{Area: S7AreaPE, WordLen: S7WLWord, Start: 2, Amount: 1, DataType: "int"},
			wantSize: 2,
		},
		{
			name: "ID dword",
			addr: "ID4", dt: "dword",
			want:     S7Item{Area: S7AreaPE, WordLen: S7WLDWord, Start: 4, Amount: 1, DataType: "dword"},
			wantSize: 4,
		},

		// ── Outputs (PA) ──
		{
			name: "Q bit",
			addr: "Q0.0", dt: "bool",
			want:     S7Item{Area: S7AreaPA, WordLen: S7WLBit, Start: 0, Bit: 0, Amount: 1, DataType: "bool"},
			wantSize: 1,
		},
		{
			name: "QB byte",
			addr: "QB1", dt: "byte",
			want:     S7Item{Area: S7AreaPA, WordLen: S7WLByte, Start: 1, Amount: 1, DataType: "byte"},
			wantSize: 1,
		},
		{
			name: "QW word",
			addr: "QW2", dt: "word",
			want:     S7Item{Area: S7AreaPA, WordLen: S7WLWord, Start: 2, Amount: 1, DataType: "word"},
			wantSize: 2,
		},
		{
			name: "QD real",
			addr: "QD4", dt: "real",
			want:     S7Item{Area: S7AreaPA, WordLen: S7WLDWord, Start: 4, Amount: 1, DataType: "real"},
			wantSize: 4,
		},

		// ── Counters / Timers ──
		{
			name: "Counter",
			addr: "C5", dt: "int",
			want:     S7Item{Area: S7AreaCT, WordLen: S7WLCounter, Start: 5, Amount: 1, DataType: "int"},
			wantSize: 2,
		},
		{
			name: "Timer",
			addr: "T3", dt: "int",
			want:     S7Item{Area: S7AreaTM, WordLen: S7WLTimer, Start: 3, Amount: 1, DataType: "int"},
			wantSize: 2,
		},

		// ── Robustness: case-insensitive, surrounding whitespace ──
		{
			name: "lowercase",
			addr: "db10.dbd0", dt: "real",
			want:     S7Item{Area: S7AreaDB, WordLen: S7WLDWord, DBNumber: 10, Start: 0, Amount: 1, DataType: "real"},
			wantSize: 4,
		},
		{
			name: "padded whitespace",
			addr: "  M0.3  ", dt: "bool",
			want:     S7Item{Area: S7AreaMK, WordLen: S7WLBit, Start: 0, Bit: 3, Amount: 1, DataType: "bool"},
			wantSize: 1,
		},
		{
			name: "uppercase dataType is normalised",
			addr: "DB10.DBD0", dt: "REAL",
			want:     S7Item{Area: S7AreaDB, WordLen: S7WLDWord, DBNumber: 10, Start: 0, Amount: 1, DataType: "real"},
			wantSize: 4,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseS7Address(c.addr, c.dt)
			if err != nil {
				t.Fatalf("ParseS7Address(%q, %q) returned error: %v", c.addr, c.dt, err)
			}
			if got != c.want {
				t.Errorf("S7Item mismatch\n  got:  %#v\n  want: %#v", got, c.want)
			}
			if size := got.ByteSize(); size != c.wantSize {
				t.Errorf("ByteSize() = %d, want %d", size, c.wantSize)
			}
		})
	}
}

func TestParseS7Address_Negative(t *testing.T) {
	cases := []struct {
		name        string
		addr        string
		dt          string
		errContains string
	}{
		{
			name: "empty address",
			addr: "", dt: "real",
			errContains: "must not be empty",
		},
		{
			name: "empty dataType",
			addr: "DB1.DBD0", dt: "",
			errContains: "dataType must not be empty",
		},
		{
			name: "symbolic DB address — OPC UA hint",
			addr: "DB1.MotorSpeed", dt: "real",
			errContains: "use OPC UA",
		},
		{
			name: "DBX with bit out of range (8)",
			addr: "DB1.DBX0.8", dt: "bool",
			errContains: "does not match",
		},
		{
			name: "M bit out of range (9)",
			addr: "M0.9", dt: "bool",
			errContains: "does not match",
		},
		{
			name: "DBD with bool dataType",
			addr: "DB1.DBD0", dt: "bool",
			errContains: "address form DBD requires",
		},
		{
			name: "M bit with int dataType",
			addr: "M0.3", dt: "int",
			errContains: "address form M.<bit> requires",
		},
		{
			name: "MD with byte dataType",
			addr: "MD10", dt: "byte",
			errContains: "address form MD requires",
		},
		{
			name: "DB STRING with maxLen 0",
			addr: "DB1.STRING0.0", dt: "string",
			errContains: "STRING maxLen 0 outside",
		},
		{
			name: "DB STRING with maxLen 255",
			addr: "DB1.STRING0.255", dt: "string",
			errContains: "STRING maxLen 255 outside",
		},
		{
			name: "DBL with real dataType (only lreal/lint/ulint allowed)",
			addr: "DB1.DBL0", dt: "real",
			errContains: "address form DBL requires",
		},
		{
			name: "DB WSTRING maxLen 0",
			addr: "DB1.WSTRING0.0", dt: "wstring",
			errContains: "WSTRING maxLen 0 outside",
		},
		{
			name: "DB WSTRING maxLen 16383",
			addr: "DB1.WSTRING0.16383", dt: "wstring",
			errContains: "WSTRING maxLen 16383 outside",
		},
		{
			name: "totally unknown form",
			addr: "ZB99", dt: "real",
			errContains: "does not match any known S7 form",
		},
		{
			name: "DB without DB number",
			addr: "DB.DBD0", dt: "real",
			errContains: "does not match any known S7 form",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseS7Address(c.addr, c.dt)
			if err == nil {
				t.Fatalf("ParseS7Address(%q, %q) succeeded; expected error containing %q", c.addr, c.dt, c.errContains)
			}
			if !strings.Contains(err.Error(), c.errContains) {
				t.Errorf("error %q does not contain expected substring %q", err.Error(), c.errContains)
			}
		})
	}
}

func TestS7Item_ByteSize_PerWordLen(t *testing.T) {
	cases := []struct {
		wordLen int
		amount  int
		want    int
	}{
		{S7WLBit, 1, 1},
		{S7WLByte, 1, 1},
		{S7WLByte, 22, 22}, // STRING(20) on the wire
		{S7WLChar, 1, 1},
		{S7WLWord, 1, 2},
		{S7WLInt, 1, 2},
		{S7WLDWord, 1, 4},
		{S7WLDInt, 1, 4},
		{S7WLReal, 1, 4},
		{S7WLCounter, 1, 2},
		{S7WLTimer, 1, 2},
	}
	for _, c := range cases {
		i := S7Item{WordLen: c.wordLen, Amount: c.amount}
		if got := i.ByteSize(); got != c.want {
			t.Errorf("ByteSize(WordLen=0x%02x, Amount=%d) = %d, want %d", c.wordLen, c.amount, got, c.want)
		}
	}
}
