package utils

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"io"
)

func CompressGzip(data string) string {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)

	if _, err := gz.Write([]byte(data)); err != nil {
		return data
	}

	if err := gz.Close(); err != nil {
		return data
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func DecompressGzip(data string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", err
	}

	gz, err := gzip.NewReader(bytes.NewReader(decoded))
	if err != nil {
		return "", err
	}
	defer gz.Close()

	result, err := io.ReadAll(gz)
	if err != nil {
		return "", err
	}

	return string(result), nil
}
