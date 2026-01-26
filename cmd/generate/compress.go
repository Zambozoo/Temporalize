package main

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// Alphabets
const (
	base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	base36Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	base64Chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
)

const debugCompression = true

// --- Big Int Helpers ---

func decodeBaseN(s string, alphabet string) *big.Int {
	val := big.NewInt(0)
	base := big.NewInt(int64(len(alphabet)))

	for _, c := range s {
		idx := strings.IndexRune(alphabet, c)
		if idx == -1 {
			// For robustness, treat invalid chars as 0 or panic.
			// Given this is generation time, panic is okay to catch errors early.
			panic(fmt.Sprintf("invalid char %c in %s", c, s))
		}
		val.Mul(val, base)
		val.Add(val, big.NewInt(int64(idx)))
	}
	return val
}

func encodeBaseN(val *big.Int, alphabet string) string {
	if val.Sign() == 0 {
		return string(alphabet[0])
	}

	var res []byte
	base := big.NewInt(int64(len(alphabet)))
	zero := big.NewInt(0)
	v := new(big.Int).Set(val)
	mod := new(big.Int)

	for v.Cmp(zero) > 0 {
		v.DivMod(v, base, mod)
		res = append(res, alphabet[mod.Int64()])
	}

	for i, j := 0, len(res)-1; i < j; i, j = i+1, j-1 {
		res[i], res[j] = res[j], res[i]
	}
	return string(res)
}

func padString(s string, length int, padChar byte) string {
	if len(s) >= length {
		return s
	}
	return strings.Repeat(string(padChar), length-len(s)) + s
}

// compress generates the compressed byte slice for the QR code.
// Format:
// [AmazonAlbum+Explicit (7 bytes)]
// [AmazonTrack (7 bytes)]
// [AppleAlbum (Uvarint)]
// [AppleTrack (Varint Delta)]
// [Spotify (17 bytes)]
// [YouTube (9 bytes)]
func compress(explicit bool, amazonAlbum, amazonTrack, appleAlbum, appleTrack, spotify, youtube string) ([]byte, error) {
	var buf []byte

	// Amazon Album + Explicit (7 bytes)
	// Decode Base36
	var amzAlbVal *big.Int
	if amazonAlbum != "" {
		amzAlbVal = decodeBaseN(amazonAlbum, base36Chars)
	} else {
		amzAlbVal = big.NewInt(0)
	}

	amzAlbBytes := amzAlbVal.Bytes()
	if len(amzAlbBytes) > 7 {
		return nil, fmt.Errorf("amazon album too long")
	}

	// Pad to 7 bytes
	paddedAmzAlb := make([]byte, 7)
	copy(paddedAmzAlb[7-len(amzAlbBytes):], amzAlbBytes)

	// Set Explicit Bit (Bit 7 of byte 0)
	if explicit {
		paddedAmzAlb[0] |= 1 << 7
	}

	buf = append(buf, paddedAmzAlb...)

	// Amazon Track (7 bytes)
	if amazonTrack != "" {
		b := decodeBaseN(amazonTrack, base36Chars).Bytes()
		if len(b) > 7 {
			return nil, fmt.Errorf("amazon track too long")
		}
		padded := make([]byte, 7)
		copy(padded[7-len(b):], b)
		buf = append(buf, padded...)
	} else {
		buf = append(buf, make([]byte, 7)...)
	}

	// Apple Album (Uvarint)
	var appAlbVal uint64
	if appleAlbum != "" {
		var err error
		appAlbVal, err = strconv.ParseUint(appleAlbum, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid apple album id: %w", err)
		}
	}
	temp := make([]byte, 10)
	n := binary.PutUvarint(temp, appAlbVal)
	buf = append(buf, temp[:n]...)

	// Apple Track (Varint Delta)
	var appTrkVal uint64
	if appleTrack != "" {
		var err error
		appTrkVal, err = strconv.ParseUint(appleTrack, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid apple track id: %w", err)
		}
	}

	// Delta
	// If Album is 0 (missing), Delta is just Track.
	delta := int64(appTrkVal) - int64(appAlbVal)
	n = binary.PutVarint(temp, delta)
	buf = append(buf, temp[:n]...)

	// Spotify (17 bytes)
	if spotify != "" {
		b := decodeBaseN(spotify, base62Chars).Bytes()
		if len(b) > 17 {
			return nil, fmt.Errorf("spotify id too long")
		}
		padded := make([]byte, 17)
		copy(padded[17-len(b):], b)
		buf = append(buf, padded...)
	} else {
		buf = append(buf, make([]byte, 17)...)
	}

	// YouTube (9 bytes)
	if youtube != "" {
		b := decodeBaseN(youtube, base64Chars).Bytes()
		if len(b) > 9 {
			return nil, fmt.Errorf("youtube id too long")
		}
		padded := make([]byte, 9)
		copy(padded[9-len(b):], b)
		buf = append(buf, padded...)
	} else {
		buf = append(buf, make([]byte, 9)...)
	}

	if debugCompression {
		if err := verifyCompression(buf, explicit, amazonAlbum, amazonTrack, appleAlbum, appleTrack, spotify, youtube); err != nil {
			return nil, err
		}
	}

	return buf, nil
}

func decompress(data []byte) (bool, string, string, string, string, string, string, error) {
	if len(data) < 7 { // Min length for AmzAlb
		return false, "", "", "", "", "", "", fmt.Errorf("short data")
	}

	idx := 0

	// Amazon Album + Explicit (7 bytes)
	amzAlbBytes := make([]byte, 7)
	copy(amzAlbBytes, data[idx:idx+7])
	idx += 7

	// Extract Explicit
	explicit := (amzAlbBytes[0] & (1 << 7)) != 0
	// Clear Explicit bit for value decoding
	amzAlbBytes[0] &= 0x7F

	var amzAlb string
	val := new(big.Int).SetBytes(amzAlbBytes)
	if val.Sign() > 0 {
		amzAlb = padString(encodeBaseN(val, base36Chars), 10, base36Chars[0])
	}

	// Amazon Track (7 bytes)
	if idx+7 > len(data) {
		return false, "", "", "", "", "", "", fmt.Errorf("short data amz trk")
	}
	var amzTrk string
	val = new(big.Int).SetBytes(data[idx : idx+7])
	if val.Sign() > 0 {
		amzTrk = padString(encodeBaseN(val, base36Chars), 10, base36Chars[0])
	}
	idx += 7

	// Apple Album (Uvarint)
	appAlbVal, n := binary.Uvarint(data[idx:])
	if n <= 0 {
		return false, "", "", "", "", "", "", fmt.Errorf("bad varint app alb")
	}
	idx += n
	var appAlb string
	if appAlbVal > 0 {
		appAlb = strconv.FormatUint(appAlbVal, 10)
	}

	// Apple Track (Varint Delta)
	delta, n := binary.Varint(data[idx:])
	if n <= 0 {
		return false, "", "", "", "", "", "", fmt.Errorf("bad varint app trk")
	}
	idx += n
	var appTrk string
	appTrkVal := int64(appAlbVal) + delta
	if appTrkVal > 0 {
		appTrk = strconv.FormatInt(appTrkVal, 10)
	}

	// Spotify (17 bytes)
	if idx+17 > len(data) {
		return false, "", "", "", "", "", "", fmt.Errorf("short data spot")
	}
	var spot string
	val = new(big.Int).SetBytes(data[idx : idx+17])
	if val.Sign() > 0 {
		spot = padString(encodeBaseN(val, base62Chars), 22, base62Chars[0])
	}
	idx += 17

	// YouTube (9 bytes)
	if idx+9 > len(data) {
		return false, "", "", "", "", "", "", fmt.Errorf("short data yt")
	}
	var yt string
	val = new(big.Int).SetBytes(data[idx : idx+9])
	if val.Sign() > 0 {
		yt = padString(encodeBaseN(val, base64Chars), 11, base64Chars[0])
	}
	idx += 9

	return explicit, amzAlb, amzTrk, appAlb, appTrk, spot, yt, nil
}

func verifyCompression(buf []byte, explicit bool, amzAlb, amzTrk, appAlb, appTrk, spot, yt string) error {
	dExpl, dAmzAlb, dAmzTrk, dAppAlb, dAppTrk, dSpot, dYt, err := decompress(buf)
	if err != nil {
		return fmt.Errorf("sanity check failed: decompression error: %w", err)
	}

	if dExpl != explicit {
		return fmt.Errorf("sanity check failed: explicit mismatch")
	}

	check := func(name, original, decoded string, padLen int, padChar byte) error {
		expected := original
		if original != "" && padLen > 0 {
			expected = padString(original, padLen, padChar)
		}
		if decoded != expected {
			return fmt.Errorf("sanity check failed: %s mismatch: got %q, want %q", name, decoded, expected)
		}
		return nil
	}

	if err := check("AmazonAlbum", amzAlb, dAmzAlb, 10, base36Chars[0]); err != nil {
		return err
	}
	if err := check("AmazonTrack", amzTrk, dAmzTrk, 10, base36Chars[0]); err != nil {
		return err
	}
	if err := check("AppleAlbum", appAlb, dAppAlb, 0, 0); err != nil {
		return err
	}
	if err := check("AppleTrack", appTrk, dAppTrk, 0, 0); err != nil {
		return err
	}
	if err := check("Spotify", spot, dSpot, 22, base62Chars[0]); err != nil {
		return err
	}
	if err := check("YouTube", yt, dYt, 11, base64Chars[0]); err != nil {
		return err
	}

	return nil
}
