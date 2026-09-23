// 文件：chaos-go/jmcomic/jmcomic.go —— 混沌机组件·JMComic：调用 Python API/CLI 查询详情与下载本子
// 修改：2026-09-23（日期由 fresh-header.ps1 刷新）

// Package jmcomic 是混沌机的 JMComic 娱乐/下载组件：功能名 `jm_comic`。
//
// 封装了对 JMComic-Crawler-Python 的调用，支持查询漫画详情（jmv）
// 以及下载漫画/章节到指定路径（download_album / download_photo）。
package jmcomic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Reisentyann/Mabel-s-Tentacles/chaos-go"
)

// resultMarker 结果行前缀：jmcomic 自身会向 stdout 打中文日志，
// 真正的结果 JSON 用本前缀标记，Go 侧按行扫描提取（容忍日志噪音）。
// 子进程强制 PYTHONUTF8=1——管道下 Python 默认 GBK，本子标题的 ♡ 等字符
// 无法编码会导致脚本崩溃、stdout 乱码（2026-09-21 本地实测修复）。
const resultMarker = "##JMRESULT##"

// pythonEnv 子进程环境：强制 UTF-8 输入输出（管道默认跟随系统 locale）。
func pythonEnv() []string {
	return append(os.Environ(), "PYTHONUTF8=1", "PYTHONIOENCODING=utf-8")
}

// extractResult 从子进程 stdout 按行扫描标记行并解析结果 JSON。
func extractResult(stdout []byte) (map[string]any, bool) {
	for _, line := range strings.Split(string(stdout), "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, resultMarker); ok {
			var result map[string]any
			if err := json.Unmarshal([]byte(after), &result); err == nil {
				return result, true
			}
		}
	}
	return nil, false
}

func init() {
	chaos.Register(jmComicFeature{})
}

type jmComicFeature struct{}

func (jmComicFeature) Name() string {
	return "jm_comic"
}

func (jmComicFeature) Description() string {
	return "JMComic tools in chaos machine: query comic metadata or trigger download of album/photo via JMComic-Crawler-Python. " +
		"Actions: 'view' (query info, default) or 'download'. " +
		"Params: jm_id (album/photo id, e.g. 123456), action ('view'|'download'), target_type ('album'|'photo', default 'album'), " +
		"dir (save dir for download), option_file (optional option.yml path)."
}

func (jmComicFeature) Params() []chaos.Param {
	return []chaos.Param{
		{
			Name:        "jm_id",
			Type:        chaos.ParamString,
			Description: "The JM comic album/photo ID (or text containing ID, e.g. '350234').",
			Required:    true,
		},
		{
			Name:        "action",
			Type:        chaos.ParamString,
			Description: "Action to execute: 'view' (retrieve metadata) or 'download' (download comic). Default is 'view'.",
			Default:     "view",
		},
		{
			Name:        "target_type",
			Type:        chaos.ParamString,
			Description: "Type of target: 'album' or 'photo'. Default is 'album'.",
			Default:     "album",
		},
		{
			Name:        "format",
			Type:        chaos.ParamString,
			Description: "Archive format when downloading: 'zip' (default) or 'raw' (loose images).",
			Default:     "zip",
		},
		{
			Name:        "save_name",
			Type:        chaos.ParamString,
			Description: "Optional target logic path/filename in Mabel storage (e.g. 'comics/350234.zip'). If omitted, defaults to 'comics/[JM<id>] <title>.zip'.",
		},
		{
			Name:        "dir",
			Type:        chaos.ParamString,
			Description: "Optional temporary directory for downloading files.",
		},
		{
			Name:        "option_file",
			Type:        chaos.ParamString,
			Description: "Optional YAML option file path for JMComic configuration.",
		},
	}
}

func (jmComicFeature) Run(c *chaos.Chaos, p chaos.Params) (map[string]any, error) {
	rawID := chaos.StrParam(p, "jm_id", "")
	if strings.TrimSpace(rawID) == "" {
		return nil, errors.New("param jm_id is required")
	}

	action := strings.ToLower(strings.TrimSpace(chaos.StrParam(p, "action", "view")))
	targetType := strings.ToLower(strings.TrimSpace(chaos.StrParam(p, "target_type", "album")))
	format := strings.ToLower(strings.TrimSpace(chaos.StrParam(p, "format", "zip")))
	dir := chaos.StrParam(p, "dir", "")
	optionFile := chaos.StrParam(p, "option_file", "")

	switch action {
	case "view", "info":
		return runView(rawID, optionFile)
	case "download":
		res, err := runDownload(rawID, targetType, format, dir, optionFile)
		if err != nil {
			return res, err
		}
		if sn := chaos.StrParam(p, "save_name", ""); sn != "" {
			res["save_name"] = sn
		}
		return res, nil
	default:
		return nil, fmt.Errorf("unknown action '%s', expected 'view' or 'download'", action)
	}
}

func runView(rawID, optionFile string) (map[string]any, error) {
	pyScript := `
import json, sys, os
base_dir = os.path.abspath(os.path.join(os.path.dirname(__file__) if '__file__' in locals() else '.', '..'))
local_src = os.path.join(base_dir, 'JMComic-Crawler-Python', 'src')
if os.path.exists(local_src):
    sys.path.insert(0, local_src)

import jmcomic

raw_id = sys.argv[1]
option_path = sys.argv[2] if len(sys.argv) > 2 and sys.argv[2] else None

try:
    option = jmcomic.create_option_by_file(option_path) if option_path else jmcomic.JmModuleConfig.option_class().default()
    client = option.build_jm_client()
    album_id = jmcomic.JmcomicText.parse_to_jm_id(raw_id)
    album = client.get_album_detail(album_id)
    
    data = {
        "success": True,
        "id": album.album_id,
        "title": album.title,
        "author": getattr(album, "author", []),
        "tags": getattr(album, "tags", []),
        "actors": getattr(album, "actors", []),
        "works": getattr(album, "works", []),
        "page_count": getattr(album, "page_count", 0),
        "pub_date": getattr(album, "pub_date", ""),
        "update_date": getattr(album, "update_date", ""),
        "episode_count": len(album.episode_list) if hasattr(album, "episode_list") else 0,
        "episodes": [
            ({"id": str(ep[0]), "title": str(ep[-1])} if isinstance(ep, (tuple, list))
             else {"id": str(getattr(ep, "photo_id", "")), "title": str(getattr(ep, "title", ""))})
            for ep in getattr(album, "episode_list", [])
        ]
    }
    print("` + resultMarker + `" + json.dumps(data, ensure_ascii=False))
except Exception as e:
    err_data = {
        "success": False,
        "error": str(e)
    }
    print("` + resultMarker + `" + json.dumps(err_data, ensure_ascii=False))
    sys.exit(1)
`
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "python", "-c", pyScript, rawID, optionFile)
	cmd.Env = pythonEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if result, ok := extractResult(stdout.Bytes()); ok {
		if err != nil {
			return result, fmt.Errorf("jmcomic python script failed: %v", result["error"])
		}
		return result, nil
	}

	if err != nil {
		return nil, fmt.Errorf("python execution error: %v, stderr: %s", err, stderr.String())
	}
	return map[string]any{"success": true, "raw_output": stdout.String()}, nil
}

func runDownload(rawID, targetType, format, dir, optionFile string) (map[string]any, error) {
	pyScript := `
import json, sys, os
base_dir = os.path.abspath(os.path.join(os.path.dirname(__file__) if '__file__' in locals() else '.', '..'))
local_src = os.path.join(base_dir, 'JMComic-Crawler-Python', 'src')
if os.path.exists(local_src):
    sys.path.insert(0, local_src)

import jmcomic

raw_id = sys.argv[1]
target_type = sys.argv[2]
format_type = sys.argv[3]
target_dir = sys.argv[4] if len(sys.argv) > 4 and sys.argv[4] else None
option_path = sys.argv[5] if len(sys.argv) > 5 and sys.argv[5] else None

def text_list(value):
    if value is None:
        return []
    if isinstance(value, str):
        return [value] if value.strip() else []
    try:
        values = list(value)
    except TypeError:
        values = [value]
    return [str(item) for item in values if str(item).strip()]

def int_value(value):
    try:
        return int(value or 0)
    except (TypeError, ValueError):
        return 0

def detail_payload(detail, target_kind, fallback_id):
    raw_episodes = getattr(detail, "episode_list", []) or []
    episodes = []
    for episode in raw_episodes:
        if isinstance(episode, (tuple, list)):
            episode_id = str(episode[0]) if episode else ""
            episode_title = str(episode[-1]) if episode else ""
        else:
            episode_id = str(getattr(episode, "photo_id", ""))
            episode_title = str(getattr(episode, "title", ""))
        episodes.append({"id": episode_id, "title": episode_title})
    id_attr = "photo_id" if target_kind == "photo" else "album_id"
    detail_id = getattr(detail, id_attr, None) or fallback_id
    detail_title = getattr(detail, "title", "") or ""
    return {
        "success": True,
        "type": target_kind,
        "id": str(detail_id),
        "title": str(detail_title),
        "author": text_list(getattr(detail, "author", [])),
        "tags": text_list(getattr(detail, "tags", [])),
        "actors": text_list(getattr(detail, "actors", [])),
        "works": text_list(getattr(detail, "works", [])),
        "page_count": int_value(getattr(detail, "page_count", 0)),
        "pub_date": str(getattr(detail, "pub_date", "") or ""),
        "update_date": str(getattr(detail, "update_date", "") or ""),
        "episode_count": len(episodes),
        "episodes": episodes,
    }

try:
    if option_path:
        option = jmcomic.create_option_by_file(option_path)
    else:
        option = jmcomic.JmModuleConfig.option_class().default()

    if target_dir:
        option.dir_rule.base_dir = target_dir

    jm_id = jmcomic.JmcomicText.parse_to_jm_id(raw_id)
    extra = None
    if format_type == "zip":
        zip_save_dir = target_dir or option.dir_rule.base_dir
        extra = jmcomic.Feature.export_zip(delete_original_file=True, zip_dir=zip_save_dir)

    if target_type == "photo":
        res = jmcomic.download_photo(jm_id, option=option, extra=extra)
        detail, dler = res
        exported_zips = dler.manifest_dict[detail].get_export_filepath_list('zip') if extra else []
        archive_path = exported_zips[0] if exported_zips else ""
        file_size = os.path.getsize(archive_path) if archive_path and os.path.exists(archive_path) else 0
        data = detail_payload(detail, "photo", jm_id)
    else:
        res = jmcomic.download_album(jm_id, option=option, extra=extra)
        detail, dler = res
        exported_zips = dler.manifest_dict[detail].get_export_filepath_list('zip') if extra else []
        archive_path = exported_zips[0] if exported_zips else ""
        file_size = os.path.getsize(archive_path) if archive_path and os.path.exists(archive_path) else 0
        data = detail_payload(detail, "album", jm_id)
    data.update({
        "archive_path": archive_path,
        "archive_filename": os.path.basename(archive_path) if archive_path else "",
        "file_size": file_size,
        "save_dir": target_dir or option.dir_rule.base_dir,
    })
    print("` + resultMarker + `" + json.dumps(data, ensure_ascii=False))
except Exception as e:
    err_data = {
        "success": False,
        "error": str(e)
    }
    print("` + resultMarker + `" + json.dumps(err_data, ensure_ascii=False))
    sys.exit(1)
`
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "python", "-c", pyScript, rawID, targetType, format, dir, optionFile)
	cmd.Env = pythonEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if result, ok := extractResult(stdout.Bytes()); ok {
		if err != nil {
			return result, fmt.Errorf("jmcomic download failed: %v", result["error"])
		}
		return result, nil
	}

	if err != nil {
		return nil, fmt.Errorf("python download execution error: %v, stderr: %s", err, stderr.String())
	}

	return map[string]any{
		"success": true,
		"jm_id":   rawID,
		"output":  stdout.String(),
	}, nil
}

// Ensure interface implemented
var _ chaos.Feature = jmComicFeature{}

// Unused dummy to silence strconv import if needed
var _ = strconv.Itoa
