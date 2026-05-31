package tokenizer

// buildByteToChar returns the GPT-2 byte→rune map: printable byte ranges
// (33–126, 161–172, 174–255) map to themselves; the remaining bytes map to
// runes 256, 257, … in ascending byte order.
func buildByteToChar() [256]rune {
	var result [256]rune
	bs := make([]int, 0, 256)
	for b := 33; b <= 126; b++ {
		bs = append(bs, b)
	}
	for b := 161; b <= 172; b++ {
		bs = append(bs, b)
	}
	for b := 174; b <= 255; b++ {
		bs = append(bs, b)
	}
	cs := make([]int, len(bs))
	copy(cs, bs)
	n := 0
	for b := 0; b < 256; b++ {
		found := false
		for _, x := range bs {
			if x == b {
				found = true
				break
			}
		}
		if !found {
			bs = append(bs, b)
			cs = append(cs, 256+n)
			n++
		}
	}
	for i := range bs {
		result[bs[i]] = rune(cs[i])
	}
	return result
}

// byteLevelEncode maps each UTF-8 byte of s through b2c and concatenates.
func byteLevelEncode(s string, b2c [256]rune) string {
	var buf []rune
	for i := 0; i < len(s); i++ {
		buf = append(buf, b2c[s[i]])
	}
	return string(buf)
}
