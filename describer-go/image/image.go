// 文件：describer-go/image/image.go —— cod-image 插件：尺寸/长宽比/调色板/色系/亮度对比（k-means 确定性）
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

// Package image cod-image 插件：图像确定性事实（尺寸/调色板/色系/亮度）。
// 字段字典见 docs/元数据字段说明.md 第 4.2 节。
// 全部算法确定性：k-means 固定迭代 + 确定性初始化，同一输入永远同一输出。
package image

import (
	"bytes"
	"fmt"
	"image"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"sort"
	"strings"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
)

func init() { describer.Register(descriptor{}) }

type descriptor struct{}

func (descriptor) Family() string { return "image" }

// FamilyVersion=1：首发版本（尺寸/调色板/色系/亮度/EXIF/PNG tEXt）。
func (descriptor) FamilyVersion() int { return 1 }
func (descriptor) SPNamespaces() []string {
	return []string{"sp-cod-exif-", "sp-cod-png-"}
}
func (descriptor) Supports(_ string, _ []byte, b describer.Basic) bool {
	return strings.HasPrefix(b.Mime, "image/")
}

type pixel struct{ r, g, b, a uint8 }

func (descriptor) Analyze(_ describer.Input, full []byte) (map[string]any, map[string]string) {
	img, format, err := image.Decode(bytes.NewReader(full))
	if err != nil {
		return nil, nil
	}
	a := map[string]any{}
	sp := map[string]string{}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	a["cod-image-width"] = w
	a["cod-image-height"] = h
	a["cod-image-aspect"] = aspect(w, h)
	switch {
	case w == h:
		a["cod-image-orientation"] = "square"
	case w > h:
		a["cod-image-orientation"] = "landscape"
	default:
		a["cod-image-orientation"] = "portrait"
	}
	a["cod-image-megapixels"] = describer.Round1(float64(w*h) / 1e6)

	// 降采样到 ≤128×128（步长采样，确定性）
	px := sample(img, 128)
	if len(px) == 0 {
		return a, sp
	}

	// 亮度 / 对比度（luma 均值与标准差，0-100）
	var sum, sumsq float64
	for _, p := range px {
		l := luma(p)
		sum += l
		sumsq += l * l
	}
	n := float64(len(px))
	mean := sum / n
	a["cod-image-brightness"] = describer.Round1(mean * 100 / 255)
	a["cod-image-contrast"] = describer.Round1(math.Sqrt(sumsq/n-mean*mean) * 100 / 255)

	// 透明通道
	for _, p := range px {
		if p.a < 255 {
			a["cod-image-has-alpha"] = true
			break
		}
	}

	// GIF 帧数
	if format == "gif" {
		if frames, err := decodeGIFAll(full); err == nil && len(frames) > 1 {
			a["cod-image-animated"] = true
			a["cod-image-frames"] = len(frames)
		}
	}

	// k-means 主色（k=5，固定 10 轮迭代 + 等距确定性初始化）
	clusters := kmeans(px, 5)
	sort.Slice(clusters, func(i, j int) bool {
		if clusters[i].n != clusters[j].n {
			return clusters[i].n > clusters[j].n
		}
		return clusters[i].hex < clusters[j].hex
	})
	palette := make([]map[string]any, 0, len(clusters))
	for _, c := range clusters {
		palette = append(palette, map[string]any{
			"hex":   c.hex,
			"ratio": describer.Round2(float64(c.n) / n),
		})
	}
	a["cod-image-palette"] = palette
	a["cod-image-dominant"] = clusters[0].hex

	// 平均饱和度（灰度判定）+ 主色色系
	var satSum float64
	for _, p := range px {
		_, s, _ := rgbToHSL(p)
		satSum += s
	}
	avgSat := satSum / n
	fam := familyOf(clusters[0].rgb(), avgSat)
	a["cod-image-family"] = fam
	if fam == "grayscale" {
		a["cod-image-grayscale"] = true
	}

	// EXIF（JPEG）/ PNG 文本块
	if format == "jpeg" {
		exifExtract(full, a, sp)
	}
	if format == "png" {
		pngTextChunks(full, sp)
	}
	return a, sp
}

func gcd(x, y int) int {
	for y != 0 {
		x, y = y, x%y
	}
	return x
}

// decodeGIFAll 解码 GIF 全部帧（帧数统计用）。
func decodeGIFAll(full []byte) ([]*image.Paletted, error) {
	g, err := gif.DecodeAll(bytes.NewReader(full))
	if err != nil {
		return nil, err
	}
	return g.Image, nil
}

func aspect(w, h int) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	g := gcd(w, h)
	return fmt.Sprintf("%d:%d", w/g, h/g)
}

func sample(img image.Image, maxDim int) []pixel {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil
	}
	sx := (w + maxDim - 1) / maxDim
	sy := (h + maxDim - 1) / maxDim
	if sx < 1 {
		sx = 1
	}
	if sy < 1 {
		sy = 1
	}
	var px []pixel
	for y := b.Min.Y; y < b.Max.Y; y += sy {
		for x := b.Min.X; x < b.Max.X; x += sx {
			r16, g16, b16, a16 := img.At(x, y).RGBA()
			px = append(px, pixel{uint8(r16 >> 8), uint8(g16 >> 8), uint8(b16 >> 8), uint8(a16 >> 8)})
		}
	}
	return px
}

func luma(p pixel) float64 {
	return 0.299*float64(p.r) + 0.587*float64(p.g) + 0.114*float64(p.b)
}

func hex(p pixel) string {
	return fmt.Sprintf("#%02x%02x%02x", p.r, p.g, p.b)
}

type cluster struct {
	cent pixel
	n    int
	hex  string
}

func (c *cluster) rgb() pixel { return c.cent }

// kmeans 确定性 k-means：等距取像素为初始质心，固定 10 轮。
func kmeans(px []pixel, k int) []cluster {
	if k > len(px) {
		k = len(px)
	}
	if k <= 0 {
		return nil
	}
	clusters := make([]cluster, k)
	for i := 0; i < k; i++ {
		idx := len(px) * (i*2 + 1) / (2 * k)
		if idx >= len(px) {
			idx = len(px) - 1
		}
		clusters[i] = cluster{cent: px[idx], hex: hex(px[idx])}
	}
	for iter := 0; iter < 10; iter++ {
		sums := make([][4]float64, k)
		counts := make([]int, k)
		for _, p := range px {
			best, bestDist := 0, math.MaxFloat64
			for i := range clusters {
				dr := float64(p.r) - float64(clusters[i].cent.r)
				dg := float64(p.g) - float64(clusters[i].cent.g)
				db := float64(p.b) - float64(clusters[i].cent.b)
				d := dr*dr + dg*dg + db*db
				if d < bestDist {
					best, bestDist = i, d
				}
			}
			sums[best][0] += float64(p.r)
			sums[best][1] += float64(p.g)
			sums[best][2] += float64(p.b)
			sums[best][3]++
			counts[best]++
		}
		for i := range clusters {
			if counts[i] == 0 {
				continue // 空簇保留旧质心
			}
			clusters[i].cent = pixel{
				uint8(sums[i][0] / float64(counts[i])),
				uint8(sums[i][1] / float64(counts[i])),
				uint8(sums[i][2] / float64(counts[i])),
				255,
			}
			clusters[i].hex = hex(clusters[i].cent)
		}
	}
	for i := range clusters {
		clusters[i].n = 0
	}
	for _, p := range px {
		best, bestDist := 0, math.MaxFloat64
		for i := range clusters {
			dr := float64(p.r) - float64(clusters[i].cent.r)
			dg := float64(p.g) - float64(clusters[i].cent.g)
			db := float64(p.b) - float64(clusters[i].cent.b)
			d := dr*dr + dg*dg + db*db
			if d < bestDist {
				best, bestDist = i, d
			}
		}
		clusters[best].n++
	}
	// 过滤空簇
	var out []cluster
	for _, c := range clusters {
		if c.n > 0 {
			out = append(out, c)
		}
	}
	return out
}

// rgbToHSL 返回 H(0-360) S(0-1) L(0-1)。
func rgbToHSL(p pixel) (float64, float64, float64) {
	r, g, b := float64(p.r)/255, float64(p.g)/255, float64(p.b)/255
	maxC := math.Max(r, math.Max(g, b))
	minC := math.Min(r, math.Min(g, b))
	l := (maxC + minC) / 2
	if maxC == minC {
		return 0, 0, l
	}
	d := maxC - minC
	s := d / (1 - math.Abs(2*l-1))
	var h float64
	switch maxC {
	case r:
		h = 60 * math.Mod((g-b)/d, 6)
	case g:
		h = 60 * ((b-r)/d + 2)
	default:
		h = 60 * ((r-g)/d + 4)
	}
	if h < 0 {
		h += 360
	}
	return h, s, l
}

// familyOf 主色色系：灰度（全图平均饱和度）> 中性（低饱和/极端明度）> 色相桶。
// warm/cool 保留给 llm 轨与人工标注，代码只产出 8 色相桶 + grayscale/neutral。
func familyOf(p pixel, avgSat float64) string {
	if avgSat < 0.08 {
		return "grayscale"
	}
	_, s, l := rgbToHSL(p)
	if s < 0.2 || l < 0.08 || l > 0.92 {
		return "neutral"
	}
	h, _, _ := rgbToHSL(p)
	switch {
	case h < 15 || h >= 345:
		return "red"
	case h < 45:
		return "orange"
	case h < 70:
		return "yellow"
	case h < 165:
		return "green"
	case h < 200:
		return "cyan"
	case h < 255:
		return "blue"
	default:
		return "purple"
	}
}
