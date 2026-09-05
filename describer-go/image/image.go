// 文件：describer-go/image/image.go —— cod-image 插件：尺寸/长宽比/调色板/色系/亮度对比/明暗/冷暖/肤色/梯度/对称（k-means 确定性）
// 修改：2026-09-05（日期由 fresh-header.ps1 刷新）

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

// FamilyVersion=2：v1 首发；v2 修 contrast 纯色图 NaN——方差浮点误差可微负，
// 须钳非负再开方（旧实现产出 NaN，经旧 Round1 截断成垃圾整数）。
// v3 增补 P2 画像字段 13 个（字段字典 4.2 节）：color-count/flat-ratio/dark/
// light/saturation/warm-ratio/cool-ratio/edge-density/sharpness/symmetry/
// alpha-ratio/skin-tone-ratio/entropy。
func (descriptor) FamilyVersion() int { return 3 }
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
	px, gw, gh := sampleGrid(img, 128)
	if len(px) == 0 {
		return a, sp
	}
	n := float64(len(px))

	// 亮度 / 对比度（luma 均值与标准差，0-100）。方差钳非负：纯色图的
	// 浮点累计误差可致 sumsq/n-mean² 微负，直接开方会得 NaN。
	var sum, sumsq float64
	lumas := make([]float64, len(px))
	for i, p := range px {
		l := luma(p)
		lumas[i] = l
		sum += l
		sumsq += l * l
	}
	mean := sum / n
	brightness := mean * 100 / 255
	a["cod-image-brightness"] = describer.Round1(brightness)
	a["cod-image-contrast"] = describer.Round1(math.Sqrt(math.Max(0, sumsq/n-mean*mean)) * 100 / 255)

	// P2：明暗整体判定（复用 mean luma）
	a["cod-image-dark"] = brightness < 25
	a["cod-image-light"] = brightness > 80

	// P2 画像统计：一遍像素遍历（冷暖/肤色/alpha/luma 直方图/量化色桶）
	var warmN, coolN, skinN, alphaN int
	hist := [32]int{}
	quant := map[uint32]int{} // 4bit/通道量化色桶 → 计数（color-count / flat-ratio）
	for _, p := range px {
		h, s, _ := rgbToHSL(p)
		// 冷暖判定带饱和度门槛：灰/近灰（H 无意义）不算任何色系
		if s >= 0.1 {
			if h <= 60 || h >= 330 {
				warmN++
			}
			if h >= 180 && h < 300 {
				coolN++
			}
		}
		if isSkinTone(p) {
			skinN++
		}
		if p.a < 250 {
			alphaN++
		}
		hist[int(luma(p))*32/256]++
		quant[uint32(p.r>>4)<<8|uint32(p.g>>4)<<4|uint32(p.b>>4)]++
	}
	a["cod-image-warm-ratio"] = describer.Round2(float64(warmN) / n)
	a["cod-image-cool-ratio"] = describer.Round2(float64(coolN) / n)
	a["cod-image-skin-tone-ratio"] = describer.Round2(float64(skinN) / n)
	a["cod-image-alpha-ratio"] = describer.Round2(float64(alphaN) / n)
	a["cod-image-color-count"] = len(quant)
	var flat float64
	for _, c := range quant {
		r := float64(c) / n
		if r >= 0.05 {
			flat += r
		}
	}
	a["cod-image-flat-ratio"] = describer.Round2(flat)
	// luma 直方图（32 桶）香农熵：纯色图 0，噪声图趋近 log2(32)
	var ent float64
	for _, c := range hist {
		if c == 0 {
			continue
		}
		p := float64(c) / n
		ent -= p * math.Log2(p)
	}
	a["cod-image-entropy"] = describer.Round2(ent)

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
	a["cod-image-saturation"] = describer.Round1(avgSat * 100)
	fam := familyOf(clusters[0].rgb(), avgSat)
	a["cod-image-family"] = fam
	if fam == "grayscale" {
		a["cod-image-grayscale"] = true
	}

	// P2：梯度统计（Sobel 幅值曼哈顿 + 拉普拉斯响应方差，采样网格内部点）
	if gw >= 3 && gh >= 3 {
		inner, edges := 0, 0
		var lapSum, lapSq float64
		for y := 1; y < gh-1; y++ {
			for x := 1; x < gw-1; x++ {
				c := lumas[y*gw+x]
				l := lumas[y*gw+x-1]
				r := lumas[y*gw+x+1]
				t := lumas[(y-1)*gw+x]
				bt := lumas[(y+1)*gw+x]
				gx := -lumas[(y-1)*gw+x-1] + lumas[(y-1)*gw+x+1] -
					2*l + 2*r - lumas[(y+1)*gw+x-1] + lumas[(y+1)*gw+x+1]
				gy := -lumas[(y-1)*gw+x-1] - 2*t - lumas[(y-1)*gw+x+1] +
					lumas[(y+1)*gw+x-1] + 2*bt + lumas[(y+1)*gw+x+1]
				inner++
				if math.Abs(gx)+math.Abs(gy) > 40 {
					edges++
				}
				lap := 4*c - l - r - t - bt
				lapSum += lap
				lapSq += lap * lap
			}
		}
		if inner > 0 {
			a["cod-image-edge-density"] = describer.Round2(float64(edges) / float64(inner))
			lm := lapSum / float64(inner)
			// 方差钳非负（同 contrast：浮点误差可微负）
			a["cod-image-sharpness"] = describer.Round1(math.Max(0, lapSq/float64(inner)-lm*lm))
		}
	}

	// P2：水平镜像对称度（左右半 luma 差 <16 的配对占比；海报/封面构图高）
	if gw >= 2 {
		pairs, sym := 0, 0
		for y := 0; y < gh; y++ {
			for x := 0; x < gw/2; x++ {
				pairs++
				if math.Abs(lumas[y*gw+x]-lumas[y*gw+(gw-1-x)]) < 16 {
					sym++
				}
			}
		}
		if pairs > 0 {
			a["cod-image-symmetry"] = describer.Round2(float64(sym) / float64(pairs))
		}
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
	px, _, _ := sampleGrid(img, maxDim)
	return px
}

// sampleGrid 步长采样到 ≤maxDim×maxDim 网格（确定性），返回一维像素切片
// 与网格宽高（P2 梯度/对称类字段按 px[y*gw+x] 二维访问）。
func sampleGrid(img image.Image, maxDim int) ([]pixel, int, int) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, 0, 0
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
	gw, gh := 0, 0
	for y := b.Min.Y; y < b.Max.Y; y += sy {
		gh++
		gw = 0
		for x := b.Min.X; x < b.Max.X; x += sx {
			r16, g16, b16, a16 := img.At(x, y).RGBA()
			px = append(px, pixel{uint8(r16 >> 8), uint8(g16 >> 8), uint8(b16 >> 8), uint8(a16 >> 8)})
			gw++
		}
	}
	return px, gw, gh
}

func luma(p pixel) float64 {
	return 0.299*float64(p.r) + 0.587*float64(p.g) + 0.114*float64(p.b)
}

// isSkinTone 肤色范围判定（RGB 规则）：R>60 且 R>G>B 且 R-B>15 且 |R-G|<80。
// int 转换防 uint8 相减下溢。
func isSkinTone(p pixel) bool {
	rg := int(p.r) - int(p.g)
	return p.r > 60 && p.r > p.g && p.g > p.b &&
		int(p.r)-int(p.b) > 15 &&
		rg < 80 && rg > -80
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
