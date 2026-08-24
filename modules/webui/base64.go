package webui

import "encoding/base64"

func encodeToString(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}
