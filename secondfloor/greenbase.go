package secondfloor

import "bytes"

type GreenbaseComparer struct{}

type greenbaseMode int

const (
	modeTokens greenbaseMode = iota
	modeBytewise
	modeLength
)

func (GreenbaseComparer) Name() string {
	return "greenbase.KeyComparator"
}

func (GreenbaseComparer) Compare(a, b []byte) int {
	plen, mode := greenbasePrefix(a)
	if len(b) <= plen-1 {
		if bytes.Compare(a[:len(b)], b) < 0 {
			return -1
		}
		return 1
	}
	if c := bytes.Compare(a[:plen], b[:plen]); c != 0 {
		return c
	}
	switch mode {
	case modeTokens:
		return compareTokens(a[plen:], b[plen:])
	case modeBytewise:
		return bytes.Compare(a, b)
	}
	return compareInts(len(a), len(b))
}

func (GreenbaseComparer) Separator(dst, a, b []byte) []byte {
	return nil
}

func (GreenbaseComparer) Successor(dst, b []byte) []byte {
	return nil
}

func greenbasePrefix(a []byte) (int, greenbaseMode) {
	if len(a) == 0 {
		return 0, modeLength
	}
	var delims int
	switch a[0] {
	case '!':
		delims = 2
	case '#':
		delims = 3
	default:
		return 1, modeBytewise
	}
	for i := 1; i < len(a); i++ {
		switch a[i] {
		case 0:
			return i + 1, modeBytewise
		case '!', '#', '$':
			delims--
			if delims == 0 {
				return i + 1, modeTokens
			}
		}
	}
	return len(a), modeLength
}

func readVarint(b []byte, p int) (int, int, bool) {
	v := 0
	for shift := 0; shift <= 21; shift += 7 {
		if p >= len(b) {
			return 0, 0, false
		}
		c := b[p]
		p++
		v |= int(c&0x7f) << shift
		if c < 0x80 {
			return p, v, true
		}
	}
	return 0, 0, false
}

func readToken(b []byte, p int) (int, int, bool) {
	start, length, ok := readVarint(b, p)
	if !ok || len(b) < start+length+1 {
		return 0, 0, false
	}
	return start, length, true
}

func compareTokens(a, b []byte) int {
	pa, pb := 0, 0
	for {
		da, la, oka := readToken(a, pa)
		if !oka {
			break
		}
		db, lb, okb := readToken(b, pb)
		if !okb {
			return 1
		}
		if la != lb {
			n := min(la, lb)
			if c := bytes.Compare(a[da:da+n], b[db:db+n]); c != 0 {
				return c
			}
			return compareInts(la, lb)
		}
		if c := bytes.Compare(a[da:da+la+1], b[db:db+la+1]); c != 0 {
			return c
		}
		pa = da + la + 1
		pb = db + lb + 1
	}
	if _, _, ok := readToken(b, pb); ok {
		return -1
	}
	return 0
}

func compareInts(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
