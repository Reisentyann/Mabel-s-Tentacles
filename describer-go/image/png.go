// 文件：describer-go/image/png.go —— PNG tEXt/zTXt/iTXt 提取 → sp-cod-png-*（AI 生图 prompt 与参数）
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package image

import (
	"bytes"
	"compress/zlib"
	"io"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
)

// pngTextChunks 解析 PNG tEXt / zTXt / iTXt 文本块 → sp-cod-png-*。
// AI 生图工具（ComfyUI/NovelAI）把生成参数与 prompt 嵌在这里。
// 上限 30 键，单值 2000 字符。
func pngTextChunks(full []byte, sp map[string]string) {
	const pngSig = "\x89PNG\r\n\x1a\n"
	if len(full) < 8 || string(full[:8]) != pngSig {
		return
	}
	data := full[8:]
	n := 0
	for len(data) >= 12 && n < 30 {
		length := int(uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3]))
		ctype := string(data[4:8])
		if length < 0 || 8+length+4 > len(data) {
			return
		}
		body := data[8 : 8+length]
		switch ctype {
		case "tEXt":
			if kw, val, ok := splitNull(body); ok {
				putSP(sp, &n, "png-"+sanitizeKey(kw), latin1ToUTF8(val))
			}
		case "zTXt":
			// keyword\0 压缩方法(0=zlib) + zlib 数据；文本为 latin-1
			if i := bytes.IndexByte(body, 0); i > 0 && i+2 <= len(body) && body[i+1] == 0 {
				if out, err := inflate(body[i+2:]); err == nil {
					putSP(sp, &n, "png-"+sanitizeKey(string(body[:i])), latin1ToUTF8(out))
				}
			}
		case "iTXt":
			// keyword\0 压缩标志 压缩方法 langTag\0 翻译关键字\0 文本(utf-8)
			if kw, rest, ok := splitNull(body); ok && len(rest) >= 2 && rest[0] == 0 {
				rest = rest[2:]
				if _, rest2, ok := splitNull(rest); ok {
					if _, text, ok := splitNull(rest2); ok {
						putSP(sp, &n, "png-"+sanitizeKey(kw), string(text))
					}
				}
			}
		}
		if ctype == "IEND" {
			return
		}
		data = data[8+length+4:]
	}
}

func putSP(sp map[string]string, n *int, key, val string) {
	if key == "" || val == "" || *n >= 30 {
		return
	}
	if _, exists := sp[key]; exists {
		return
	}
	sp[key] = describer.TrimRunes(val, 2000)
	*n++
}

func splitNull(b []byte) (string, []byte, bool) {
	i := bytes.IndexByte(b, 0)
	if i <= 0 || i+1 >= len(b) {
		return "", nil, false
	}
	return string(b[:i]), b[i+1:], true
}

func inflate(b []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(io.LimitReader(r, 1<<20)) // 1MB 上限防炸弹
}

func latin1ToUTF8(b []byte) string {
	out := make([]rune, len(b))
	for i, x := range b {
		out[i] = rune(x)
	}
	return string(out)
}

// sanitizeKey 关键字规范化：小写、非字母数字折叠为 -、去首尾 -。
func sanitizeKey(s string) string {
	var b []byte
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b = append(b, byte(r))
		case r >= 'A' && r <= 'Z':
			b = append(b, byte(r-'A'+'a'))
		default:
			if len(b) > 0 && b[len(b)-1] != '-' {
				b = append(b, '-')
			}
		}
	}
	for len(b) > 0 && b[len(b)-1] == '-' {
		b = b[:len(b)-1]
	}
	return string(b)
}
