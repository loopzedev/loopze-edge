// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// S7 area codes — wire-level constants from the S7 protocol. They are stable
// across CPUs and firmware versions, so we declare our own copy instead of
// depending on the (unexported) ones in github.com/robinson/gos7. The values
// match gos7's internal table 1:1.
const (
	S7AreaPE = 0x81 // Process Inputs (I)
	S7AreaPA = 0x82 // Process Outputs (Q)
	S7AreaMK = 0x83 // Merker / Flags (M)
	S7AreaDB = 0x84 // Data Block (DB)
	S7AreaCT = 0x1C // Counters
	S7AreaTM = 0x1D // Timers
)

// S7 word-length codes — also from the S7 protocol. The codec uses the smaller
// set Bit / Byte / Word / DWord exclusively because the wire layout for signed
// vs. unsigned integers is identical (both 16-bit values share WordLen=Word);
// the signedness lives in the dataType string instead and is resolved by the
// codec on decode.
const (
	S7WLBit     = 0x01
	S7WLByte    = 0x02
	S7WLChar    = 0x03
	S7WLWord    = 0x04
	S7WLInt     = 0x05
	S7WLDWord   = 0x06
	S7WLDInt    = 0x07
	S7WLReal    = 0x08
	S7WLCounter = 0x1C
	S7WLTimer   = 0x1D
)

// S7Item describes a single addressed S7 variable in a wire-portable form.
// It mirrors github.com/robinson/gos7.S7DataItem but stays in our package so
// the rest of the codebase doesn't have to import gos7 transitively.
//
// The conversion to the gos7 type lives in s7_plc.go (PR-3). Bit access on
// the wire encodes the bit position into Start as `byte*8+bit`; this struct
// keeps Start as the byte offset and exposes Bit separately so the parser
// output stays human-readable. The conversion does the multiplication.
type S7Item struct {
	Area     int    // S7Area* constant
	WordLen  int    // S7WL* constant
	DBNumber int    // 0 unless Area == S7AreaDB
	Start    int    // byte offset within the area
	Bit      int    // 0..7, only meaningful when WordLen == S7WLBit
	Amount   int    // unit count: 1 for scalars, maxLen+2 for STRING, N for raw
	DataType string // user-facing type ("bool", "real", "string", …) — kept so
	// the codec doesn't need the original config to decode the result
	StringMaxLen int // declared maxLen for STRING (== Amount-2); 0 for non-strings
}

// Address-form regexes. Order matters during dispatch only insofar as the
// symbolic-DB check runs first to give a precise error message for cases like
// `DB1.MotorSpeed`. The frontend `S7AddressInput.vue` validator must mirror
// these exactly — drift will be caught by the cross-fixture test (TS side
// wired in PR-9).
var (
	reDBBit    = regexp.MustCompile(`^DB(\d+)\.DBX(\d+)\.([0-7])$`)
	reDBByte   = regexp.MustCompile(`^DB(\d+)\.DBB(\d+)$`)
	reDBWord   = regexp.MustCompile(`^DB(\d+)\.DBW(\d+)$`)
	reDBDWord  = regexp.MustCompile(`^DB(\d+)\.DBD(\d+)$`)
	reDBString = regexp.MustCompile(`^DB(\d+)\.STRING(\d+)\.(\d+)$`)
	// Symbolic-DB catcher: ≥4 word chars after the dot, end of string. Tight
	// enough to NOT match malformed wire forms like `DB1.DBX0.8` (rejected as
	// "does not match" with a hint to fix the bit number) but loose enough to
	// catch typical TIA-Portal identifiers (`DB1.MotorSpeed`, `DB99.tank_temp`).
	// Single-character or short trailing tokens fall through to the generic
	// error rather than the OPC UA hint — they're more likely typos than
	// genuine symbolic addresses.
	reDBSym = regexp.MustCompile(`^DB\d+\.[A-Za-z_][A-Za-z0-9_]{3,}$`)

	reMBit  = regexp.MustCompile(`^M(\d+)\.([0-7])$`)
	reMByte = regexp.MustCompile(`^MB(\d+)$`)
	reMWord = regexp.MustCompile(`^MW(\d+)$`)
	reMDWrd = regexp.MustCompile(`^MD(\d+)$`)

	reIBit  = regexp.MustCompile(`^I(\d+)\.([0-7])$`)
	reIByte = regexp.MustCompile(`^IB(\d+)$`)
	reIWord = regexp.MustCompile(`^IW(\d+)$`)
	reIDWrd = regexp.MustCompile(`^ID(\d+)$`)

	reQBit  = regexp.MustCompile(`^Q(\d+)\.([0-7])$`)
	reQByte = regexp.MustCompile(`^QB(\d+)$`)
	reQWord = regexp.MustCompile(`^QW(\d+)$`)
	reQDWrd = regexp.MustCompile(`^QD(\d+)$`)

	reCounter = regexp.MustCompile(`^C(\d+)$`)
	reTimer   = regexp.MustCompile(`^T(\d+)$`)
)

// ParseS7Address turns a Siemens-style address string ("DB10.DBD0", "M0.3",
// "IB4", "DB1.STRING50.20", …) plus the user-declared dataType into a wire-
// portable S7Item.
//
// The parser is strict about the form/dataType contract: the address syntax
// determines a permitted set of dataTypes (e.g. `M0.0` is bit-addressable so
// dataType must be "bool"; `MD10` is dword-addressable so dataType must be
// one of dword/dint/real). Mismatches return an error rather than silently
// re-typing the access — Siemens projects depend on the exact wire size, and
// silently widening a read would mask a configuration mistake.
//
// Optimized DBs (TIA Portal default for S7-1200/1500) use symbolic addressing
// (`DB1.MotorSpeed`) and are not addressable via the S7 protocol at all. The
// parser detects this form and rejects with a clear hint towards OPC UA.
func ParseS7Address(addr, dataType string) (S7Item, error) {
	if addr = strings.TrimSpace(addr); addr == "" {
		return S7Item{}, fmt.Errorf("address must not be empty")
	}
	dt := strings.ToLower(strings.TrimSpace(dataType))
	if dt == "" {
		return S7Item{}, fmt.Errorf("dataType must not be empty")
	}
	upper := strings.ToUpper(addr)

	switch {
	// ── DB ──────────────────────────────────────────────────────────────
	case reDBBit.MatchString(upper):
		m := reDBBit.FindStringSubmatch(upper)
		if err := requireType(dt, "DBX", "bool"); err != nil {
			return S7Item{}, err
		}
		return mkBitItem(S7AreaDB, atoi(m[1]), atoi(m[2]), atoi(m[3]), dt), nil

	case reDBByte.MatchString(upper):
		m := reDBByte.FindStringSubmatch(upper)
		if err := requireType(dt, "DBB", "byte", "char"); err != nil {
			return S7Item{}, err
		}
		return mkScalarItem(S7AreaDB, atoi(m[1]), atoi(m[2]), wordLenFor(dt), dt), nil

	case reDBWord.MatchString(upper):
		m := reDBWord.FindStringSubmatch(upper)
		if err := requireType(dt, "DBW", "word", "int"); err != nil {
			return S7Item{}, err
		}
		return mkScalarItem(S7AreaDB, atoi(m[1]), atoi(m[2]), wordLenFor(dt), dt), nil

	case reDBDWord.MatchString(upper):
		m := reDBDWord.FindStringSubmatch(upper)
		if err := requireType(dt, "DBD", "dword", "dint", "real"); err != nil {
			return S7Item{}, err
		}
		return mkScalarItem(S7AreaDB, atoi(m[1]), atoi(m[2]), wordLenFor(dt), dt), nil

	case reDBString.MatchString(upper):
		m := reDBString.FindStringSubmatch(upper)
		if err := requireType(dt, "STRING", "string"); err != nil {
			return S7Item{}, err
		}
		maxLen := atoi(m[3])
		if maxLen < 1 || maxLen > 254 {
			return S7Item{}, fmt.Errorf("STRING maxLen %d outside [1, 254]", maxLen)
		}
		return S7Item{
			Area:         S7AreaDB,
			WordLen:      S7WLByte,
			DBNumber:     atoi(m[1]),
			Start:        atoi(m[2]),
			Amount:       maxLen + 2, // [maxLen][actLen][chars × maxLen]
			DataType:     "string",
			StringMaxLen: maxLen,
		}, nil

	// ── M (Merker / Flags) ──────────────────────────────────────────────
	case reMBit.MatchString(upper):
		m := reMBit.FindStringSubmatch(upper)
		if err := requireType(dt, "M.<bit>", "bool"); err != nil {
			return S7Item{}, err
		}
		return mkBitItem(S7AreaMK, 0, atoi(m[1]), atoi(m[2]), dt), nil

	case reMByte.MatchString(upper):
		m := reMByte.FindStringSubmatch(upper)
		if err := requireType(dt, "MB", "byte", "char"); err != nil {
			return S7Item{}, err
		}
		return mkScalarItem(S7AreaMK, 0, atoi(m[1]), wordLenFor(dt), dt), nil

	case reMWord.MatchString(upper):
		m := reMWord.FindStringSubmatch(upper)
		if err := requireType(dt, "MW", "word", "int"); err != nil {
			return S7Item{}, err
		}
		return mkScalarItem(S7AreaMK, 0, atoi(m[1]), wordLenFor(dt), dt), nil

	case reMDWrd.MatchString(upper):
		m := reMDWrd.FindStringSubmatch(upper)
		if err := requireType(dt, "MD", "dword", "dint", "real"); err != nil {
			return S7Item{}, err
		}
		return mkScalarItem(S7AreaMK, 0, atoi(m[1]), wordLenFor(dt), dt), nil

	// ── I (Inputs / PE) ─────────────────────────────────────────────────
	case reIBit.MatchString(upper):
		m := reIBit.FindStringSubmatch(upper)
		if err := requireType(dt, "I.<bit>", "bool"); err != nil {
			return S7Item{}, err
		}
		return mkBitItem(S7AreaPE, 0, atoi(m[1]), atoi(m[2]), dt), nil

	case reIByte.MatchString(upper):
		m := reIByte.FindStringSubmatch(upper)
		if err := requireType(dt, "IB", "byte", "char"); err != nil {
			return S7Item{}, err
		}
		return mkScalarItem(S7AreaPE, 0, atoi(m[1]), wordLenFor(dt), dt), nil

	case reIWord.MatchString(upper):
		m := reIWord.FindStringSubmatch(upper)
		if err := requireType(dt, "IW", "word", "int"); err != nil {
			return S7Item{}, err
		}
		return mkScalarItem(S7AreaPE, 0, atoi(m[1]), wordLenFor(dt), dt), nil

	case reIDWrd.MatchString(upper):
		m := reIDWrd.FindStringSubmatch(upper)
		if err := requireType(dt, "ID", "dword", "dint", "real"); err != nil {
			return S7Item{}, err
		}
		return mkScalarItem(S7AreaPE, 0, atoi(m[1]), wordLenFor(dt), dt), nil

	// ── Q (Outputs / PA) ────────────────────────────────────────────────
	case reQBit.MatchString(upper):
		m := reQBit.FindStringSubmatch(upper)
		if err := requireType(dt, "Q.<bit>", "bool"); err != nil {
			return S7Item{}, err
		}
		return mkBitItem(S7AreaPA, 0, atoi(m[1]), atoi(m[2]), dt), nil

	case reQByte.MatchString(upper):
		m := reQByte.FindStringSubmatch(upper)
		if err := requireType(dt, "QB", "byte", "char"); err != nil {
			return S7Item{}, err
		}
		return mkScalarItem(S7AreaPA, 0, atoi(m[1]), wordLenFor(dt), dt), nil

	case reQWord.MatchString(upper):
		m := reQWord.FindStringSubmatch(upper)
		if err := requireType(dt, "QW", "word", "int"); err != nil {
			return S7Item{}, err
		}
		return mkScalarItem(S7AreaPA, 0, atoi(m[1]), wordLenFor(dt), dt), nil

	case reQDWrd.MatchString(upper):
		m := reQDWrd.FindStringSubmatch(upper)
		if err := requireType(dt, "QD", "dword", "dint", "real"); err != nil {
			return S7Item{}, err
		}
		return mkScalarItem(S7AreaPA, 0, atoi(m[1]), wordLenFor(dt), dt), nil

	// ── Counters / Timers ───────────────────────────────────────────────
	case reCounter.MatchString(upper):
		m := reCounter.FindStringSubmatch(upper)
		if err := requireType(dt, "C", "int", "counter"); err != nil {
			return S7Item{}, err
		}
		return S7Item{
			Area:     S7AreaCT,
			WordLen:  S7WLCounter,
			Start:    atoi(m[1]),
			Amount:   1,
			DataType: dt,
		}, nil

	case reTimer.MatchString(upper):
		m := reTimer.FindStringSubmatch(upper)
		if err := requireType(dt, "T", "int", "timer"); err != nil {
			return S7Item{}, err
		}
		return S7Item{
			Area:     S7AreaTM,
			WordLen:  S7WLTimer,
			Start:    atoi(m[1]),
			Amount:   1,
			DataType: dt,
		}, nil
	}

	// No form matched — give the most helpful error we can. If this looks like
	// a symbolic DB reference, point at OPC UA; otherwise generic.
	//
	// We need to use the original (mixed-case) input here because regexp uses
	// the upper-case form internally but a user-facing hint should mention the
	// address as the user wrote it. The regex is anchored on uppercase though,
	// so we test against `upper` and report `addr`.
	if reDBSym.MatchString(upper) {
		return S7Item{}, fmt.Errorf(
			"address %q looks symbolic; the S7 protocol requires non-optimized DBs (uncheck \"Optimized block access\" in TIA Portal) — use OPC UA for symbolic access",
			addr,
		)
	}
	return S7Item{}, fmt.Errorf("address %q does not match any known S7 form (DB.<DBX|DBB|DBW|DBD|STRING>, M/MB/MW/MD, I/IB/IW/ID, Q/QB/QW/QD, C, T)", addr)
}

// ByteSize returns how many wire bytes one element of this item consumes.
// Used by the PLC manager when bundling items into a multi-read PDU.
//
// For BIT items the "byte size" is 1 since the read still ships a full byte
// from the PLC even though only one bit is meaningful (the codec masks it).
// For STRING/raw the size is `Amount` because Amount carries the byte count.
func (i S7Item) ByteSize() int {
	switch i.WordLen {
	case S7WLBit, S7WLByte, S7WLChar:
		return i.Amount
	case S7WLWord, S7WLInt, S7WLCounter, S7WLTimer:
		return 2 * i.Amount
	case S7WLDWord, S7WLDInt, S7WLReal:
		return 4 * i.Amount
	default:
		return 0
	}
}

// requireType checks that the user-supplied dataType is in the set permitted
// by the address form. The `form` argument is the form name (e.g. "DBD") and
// is included in the error message.
func requireType(got, form string, allowed ...string) error {
	for _, a := range allowed {
		if got == a {
			return nil
		}
	}
	if len(allowed) == 1 {
		return fmt.Errorf("address form %s requires dataType %q, got %q", form, allowed[0], got)
	}
	return fmt.Errorf("address form %s requires one of dataType %v, got %q", form, allowed, got)
}

// wordLenFor maps the user dataType to a wire WordLen for non-bit, non-string
// items. Word/Int share WordLen (both 16-bit on the wire); DWord/DInt/Real
// share WordLen (all 32-bit). The decode-side disambiguation lives in the
// codec.
func wordLenFor(dt string) int {
	switch dt {
	case "bool":
		return S7WLBit
	case "byte", "char":
		return S7WLByte
	case "word", "int":
		return S7WLWord
	case "dword", "dint", "real":
		return S7WLDWord
	default:
		return S7WLByte
	}
}

func mkBitItem(area, db, byteAddr, bit int, dt string) S7Item {
	return S7Item{
		Area:     area,
		WordLen:  S7WLBit,
		DBNumber: db,
		Start:    byteAddr,
		Bit:      bit,
		Amount:   1,
		DataType: dt,
	}
}

func mkScalarItem(area, db, byteAddr, wordLen int, dt string) S7Item {
	return S7Item{
		Area:     area,
		WordLen:  wordLen,
		DBNumber: db,
		Start:    byteAddr,
		Amount:   1,
		DataType: dt,
	}
}

// atoi is strconv.Atoi without an error path. Safe because it's only called
// after a regex has already matched `\d+`.
func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
