package secondfloor

import "math/big"

const base62Alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func Base62ID(gid []byte) string {
	n := new(big.Int).SetBytes(gid)
	base := big.NewInt(62)
	mod := new(big.Int)
	id := make([]byte, 22)
	for i := len(id) - 1; i >= 0; i-- {
		n.DivMod(n, base, mod)
		id[i] = base62Alphabet[mod.Int64()]
	}
	return string(id)
}

func TrackURI(gid []byte) string {
	return "spotify:track:" + Base62ID(gid)
}
