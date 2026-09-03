// 文件：describer-go/image/exif.go —— JPEG EXIF 提取：固定三件套 + sp-cod-exif-* 长尾
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package image

import (
	"bytes"
	"strings"
	"time"

	"github.com/rwcarlsen/goexif/exif"
	"github.com/rwcarlsen/goexif/tiff"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
)

// exifExtract JPEG EXIF：固定三件套（taken-at/camera/software），
// 其余标签进 sp-cod-exif-* 长尾（上限 30 键，单值 200 字符）。
func exifExtract(full []byte, a map[string]any, sp map[string]string) {
	x, err := exif.Decode(bytes.NewReader(full))
	if err != nil {
		return
	}

	// 拍摄时间：优先 DateTimeOriginal（"2006:01:02 15:04:05" 格式），退回 DateTime
	var taken time.Time
	if f, err := x.Get(exif.DateTimeOriginal); err == nil {
		if s, verr := f.StringVal(); verr == nil {
			taken, _ = time.Parse("2006:01:02 15:04:05", strings.TrimSpace(s))
		}
	}
	if taken.IsZero() {
		if t, err := x.DateTime(); err == nil {
			taken = t
		}
	}
	if !taken.IsZero() {
		a["cod-image-taken-at"] = taken.Unix()
	}

	var mk, model string
	if f, err := x.Get(exif.Make); err == nil {
		mk, _ = f.StringVal()
	}
	if f, err := x.Get(exif.Model); err == nil {
		model, _ = f.StringVal()
	}
	if cam := strings.TrimSpace(mk + " " + model); cam != "" {
		a["cod-image-camera"] = cam
	}
	if f, err := x.Get(exif.Software); err == nil {
		if s, _ := f.StringVal(); strings.TrimSpace(s) != "" {
			a["cod-image-software"] = strings.TrimSpace(s)
		}
	}

	// 长尾 → sp-cod-exif-*（上限 30 键）
	skip := map[string]bool{
		string(exif.Make): true, string(exif.Model): true, string(exif.Software): true,
		string(exif.DateTimeOriginal): true, string(exif.DateTime): true,
	}
	_ = x.Walk(&exifWalker{sp: sp, skip: skip})
}

type exifWalker struct {
	sp   map[string]string
	skip map[string]bool
	n    int
}

func (w *exifWalker) Walk(name exif.FieldName, tag *tiff.Tag) error {
	if w.n >= 30 {
		return nil
	}
	key := string(name)
	if w.skip[key] {
		return nil
	}
	val := strings.TrimSpace(tag.String())
	if val == "" || strings.Contains(val, "\x00") {
		return nil
	}
	w.sp["exif-"+describer.ToSlashLower(key)] = describer.TrimRunes(val, 200)
	w.n++
	return nil
}
