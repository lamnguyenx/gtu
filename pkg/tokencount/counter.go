package tokencount

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pkoukk/tiktoken-go"
	tiktoken_loader "github.com/pkoukk/tiktoken-go-loader"
	"github.com/sirupsen/logrus"
)

const defaultEncoding = "o200k_base"

var (
	encoder     *tiktoken.Tiktoken
	encoderOnce sync.Once
	encoderErr  error
)

func initEncoder() {
	encoderOnce.Do(func() {
		tiktoken.SetBpeLoader(tiktoken_loader.NewOfflineLoader())
		encoder, encoderErr = tiktoken.GetEncoding(defaultEncoding)
		if encoderErr != nil {
			logrus.Errorf("failed to initialize tiktoken encoder: %v", encoderErr)
		}
	})
}

// CountTokens reads a file and returns the estimated token count.
func CountTokens(path string, info os.FileInfo) int64 {
	initEncoder()

	if info.IsDir() {
		return 0
	}

	size := info.Size()
	if size == 0 {
		return 0
	}

	ext := strings.ToLower(filepath.Ext(path))

	if isImageExt(ext) {
		return countImageTokensFromFile(path, info)
	}

	return countTextTokens(path, info)
}

func isImageExt(ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
		return true
	}
	return false
}
