package tokencount

import (
	"os"

	"github.com/sirupsen/logrus"
)

const maxReadSize = 10 * 1024 * 1024 // 10MB cap for token counting

func isBinaryContent(data []byte) bool {
	if len(data) > 8192 {
		data = data[:8192]
	}
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}

func countTextTokens(path string, info os.FileInfo) int64 {
	if encoder == nil {
		return 0
	}

	readSize := maxReadSize
	if int(info.Size()) < readSize {
		readSize = int(info.Size())
	}

	file, err := os.Open(path)
	if err != nil {
		logrus.Debugf("failed to open %s for token counting: %v", path, err)
		return 0
	}
	defer file.Close()

	buf := make([]byte, readSize)
	n, err := file.Read(buf)
	if err != nil || n == 0 {
		return 0
	}
	buf = buf[:n]

	if isBinaryContent(buf) {
		return 0
	}

	tokens := encoder.EncodeOrdinary(string(buf))
	return int64(len(tokens))
}
