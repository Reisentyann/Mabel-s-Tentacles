// 文件：chaos-go/jmcomic/jmcomic.go —— 混沌机组件·JMComic：调用 Python API/CLI 查询详情与下载本子
// 修改：2026-09-21（日期由 fresh-header.ps1 刷新）

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
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Reisentyann/Mabel-s-Tentacles/chaos-go"
)

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
            {"id": ep.photo_id, "title": ep.title} for ep in getattr(album, "episode_list", [])
        ]
    }
    print(json.dumps(data, ensure_ascii=False))
except Exception as e:
    err_data = {
        "success": False,
        "error": str(e)
    }
    print(json.dumps(err_data, ensure_ascii=False))
    sys.exit(1)
`
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "python", "-c", pyScript, rawID, optionFile)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	outBytes := bytes.TrimSpace(stdout.Bytes())
	if len(outBytes) > 0 {
		var result map[string]any
		if parseErr := json.Unmarshal(outBytes, &result); parseErr == nil {
			if err != nil {
				return result, fmt.Errorf("jmcomic python script failed: %v", result["error"])
			}
			return result, nil
		}
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
        data = {
            "success": True,
            "type": "photo",
            "id": detail.photo_id,
            "title": detail.title,
            "archive_path": archive_path,
            "archive_filename": os.path.basename(archive_path) if archive_path else "",
            "file_size": file_size,
            "save_dir": target_dir or option.dir_rule.base_dir
        }
    else:
        res = jmcomic.download_album(jm_id, option=option, extra=extra)
        detail, dler = res
        exported_zips = dler.manifest_dict[detail].get_export_filepath_list('zip') if extra else []
        archive_path = exported_zips[0] if exported_zips else ""
        file_size = os.path.getsize(archive_path) if archive_path and os.path.exists(archive_path) else 0
        data = {
            "success": True,
            "type": "album",
            "id": detail.album_id,
            "title": detail.title,
            "archive_path": archive_path,
            "archive_filename": os.path.basename(archive_path) if archive_path else "",
            "file_size": file_size,
            "save_dir": target_dir or option.dir_rule.base_dir
        }
    print(json.dumps(data, ensure_ascii=False))
except Exception as e:
    err_data = {
        "success": False,
        "error": str(e)
    }
    print(json.dumps(err_data, ensure_ascii=False))
    sys.exit(1)
`
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "python", "-c", pyScript, rawID, targetType, format, dir, optionFile)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	outBytes := bytes.TrimSpace(stdout.Bytes())
	if len(outBytes) > 0 {
		var result map[string]any
		if parseErr := json.Unmarshal(outBytes, &result); parseErr == nil {
			if err != nil {
				return result, fmt.Errorf("jmcomic download failed: %v", result["error"])
			}
			return result, nil
		}
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
