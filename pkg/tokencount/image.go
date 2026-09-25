package tokencount

import (
	"encoding/binary"
	"os"
)

const (
	visionBaseTokens = 85
	visionTileTokens = 170
	maxImageSide     = 1024
	tileSize         = 512
)

func countImageTokensFromFile(path string, _ os.FileInfo) int64 {
	w, h, ok := readImageDimensions(path)
	if !ok {
		return int64(visionBaseTokens)
	}
	return int64(calcVisionTiles(w, h))
}

func calcVisionTiles(width, height int) int {
	w, h := width, height

	if w > maxImageSide || h > maxImageSide {
		if w > h {
			h = h * maxImageSide / w
			w = maxImageSide
		} else {
			w = w * maxImageSide / h
			h = maxImageSide
		}
	}

	if w == 0 {
		w = 1
	}
	if h == 0 {
		h = 1
	}

	tilesH := (w + tileSize - 1) / tileSize
	tilesV := (h + tileSize - 1) / tileSize

	return visionBaseTokens + visionTileTokens*tilesH*tilesV
}

func readImageDimensions(path string) (width, height int, ok bool) {
	ext := filepathExtLower(path)
	switch ext {
	case ".png":
		return readPngDimensions(path)
	case ".jpg", ".jpeg":
		return readJpegDimensions(path)
	case ".gif":
		return readGifDimensions(path)
	case ".bmp":
		return readBmpDimensions(path)
	case ".webp":
		return readWebpDimensions(path)
	}
	return 0, 0, false
}

func filepathExtLower(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == os.PathSeparator {
			break
		}
		if path[i] == '.' {
			ext := path[i:]
			lowered := make([]byte, len(ext))
			for j := range ext {
				c := ext[j]
				if c >= 'A' && c <= 'Z' {
					c += 32
				}
				lowered[j] = c
			}
			return string(lowered)
		}
	}
	return ""
}

func readPngDimensions(path string) (int, int, bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()

	header := make([]byte, 24)
	n, err := f.Read(header)
	if err != nil || n < 24 {
		return 0, 0, false
	}

	if header[0] != 0x89 || header[1] != 'P' || header[2] != 'N' || header[3] != 'G' {
		return 0, 0, false
	}

	width := int(binary.BigEndian.Uint32(header[16:20]))
	height := int(binary.BigEndian.Uint32(header[20:24]))
	return width, height, true
}

func readJpegDimensions(path string) (int, int, bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()

	buf := make([]byte, 2)
	if _, err := f.Read(buf); err != nil {
		return 0, 0, false
	}
	if buf[0] != 0xFF || buf[1] != 0xD8 {
		return 0, 0, false
	}

	for {
		marker := make([]byte, 2)
		if _, err := f.Read(marker); err != nil {
			return 0, 0, false
		}
		if marker[0] != 0xFF {
			return 0, 0, false
		}

		// SOF markers: C0-C3, C5-C7, C9-CB, CD-CF
		if (marker[1] >= 0xC0 && marker[1] <= 0xC3) ||
			(marker[1] >= 0xC5 && marker[1] <= 0xC7) ||
			(marker[1] >= 0xC9 && marker[1] <= 0xCB) ||
			(marker[1] >= 0xCD && marker[1] <= 0xCF) {
			seg := make([]byte, 5)
			if _, err := f.Read(seg); err != nil {
				return 0, 0, false
			}
			height := int(seg[1])<<8 | int(seg[2])
			width := int(seg[3])<<8 | int(seg[4])
			return width, height, true
		}

		// Skip other markers
		if marker[1] == 0xD8 || marker[1] == 0xD9 {
			continue
		}
		lenBuf := make([]byte, 2)
		if _, err := f.Read(lenBuf); err != nil {
			return 0, 0, false
		}
		segLen := int(lenBuf[0])<<8 | int(lenBuf[1])
		if segLen < 2 {
			return 0, 0, false
		}
		if _, err := f.Seek(int64(segLen-2), 1); err != nil {
			return 0, 0, false
		}
	}
}

func readGifDimensions(path string) (int, int, bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()

	header := make([]byte, 10)
	n, err := f.Read(header)
	if err != nil || n < 10 {
		return 0, 0, false
	}

	if header[0] != 'G' || header[1] != 'I' || header[2] != 'F' {
		return 0, 0, false
	}

	width := int(header[6]) | int(header[7])<<8
	height := int(header[8]) | int(header[9])<<8
	return width, height, true
}

func readBmpDimensions(path string) (int, int, bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()

	header := make([]byte, 26)
	n, err := f.Read(header)
	if err != nil || n < 26 {
		return 0, 0, false
	}

	if header[0] != 'B' || header[1] != 'M' {
		return 0, 0, false
	}

	width := int(binary.LittleEndian.Uint32(header[18:22]))
	height := int(int32(binary.LittleEndian.Uint32(header[22:26])))
	if height < 0 {
		height = -height
	}
	return width, height, true
}

func readWebpDimensions(path string) (int, int, bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()

	header := make([]byte, 30)
	n, err := f.Read(header)
	if err != nil || n < 30 {
		return 0, 0, false
	}

	if header[12] != 'V' || header[13] != 'P' || header[14] != '8' {
		return 0, 0, false
	}

	form := string(header[12:16])
	switch form {
	case "VP8 ":
		width := int(binary.LittleEndian.Uint16(header[26:28])) & 0x3FFF
		height := int(binary.LittleEndian.Uint16(header[28:30])) & 0x3FFF
		return width, height, true
	case "VP8L":
		if n < 25 {
			return 0, 0, false
		}
		b := header[21]
		width := 1 + int(binary.LittleEndian.Uint16([]byte{header[22], b&0x3F<<2}))
		height := 1 + int(binary.LittleEndian.Uint16([]byte{header[24], header[23]&0x0F<<4}))
		return width, height, true
	case "VP8X":
		width := 1 + int(binary.LittleEndian.Uint32([]byte{header[24], header[25], header[26], 0}))
		height := 1 + int(binary.LittleEndian.Uint32([]byte{header[27], header[28], header[29], 0}))
		return width, height, true
	}
	return 0, 0, false
}
