// Package catalog builds deterministic, non-provider review catalogs for
// configured static production art.
package catalog

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/pack"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
)

const (
	StatusPresent     = "present"
	StatusPlaceholder = "planned-placeholder"
)

type Options struct {
	PackDir   string
	DeployDir string
	UsageRoot string
	OutputDir string
}

type Result struct {
	Entries      int
	IndexPath    string
	MetadataPath string
}

type Metadata struct {
	PackVersion int     `json:"packVersion"`
	Entries     []Entry `json:"entries"`
}

type Entry struct {
	ID            string    `json:"id"`
	ObjectID      string    `json:"objectId"`
	PartID        string    `json:"partId,omitempty"`
	AtomicSet     string    `json:"atomicSet,omitempty"`
	Family        string    `json:"family"`
	Description   string    `json:"description"`
	DeployPath    string    `json:"deployPath"`
	LogicalSize   pack.Size `json:"logicalSize"`
	IntrinsicSize pack.Size `json:"intrinsicSize"`
	Density       string    `json:"density"`
	Status        string    `json:"status"`
	SHA256        string    `json:"sha256,omitempty"`
	MapUsage      []string  `json:"mapUsage"`
	PreviewPath   string    `json:"previewPath,omitempty"`
}

type catalogView struct {
	Groups []catalogGroup
}

type catalogGroup struct {
	Family  string
	Entries []Entry
}

func Build(p *pack.Pack, all []targets.Target, opts Options) (Result, error) {
	if strings.TrimSpace(opts.OutputDir) == "" {
		return Result{}, errors.New("catalog output directory is required")
	}
	usageFiles, err := readUsageFiles(opts.UsageRoot)
	if err != nil {
		return Result{}, err
	}
	entries, sources, err := collectEntries(all, opts.DeployDir, usageFiles)
	if err != nil {
		return Result{}, err
	}
	staging := opts.OutputDir + ".tmp"
	if err := os.RemoveAll(staging); err != nil {
		return Result{}, fmt.Errorf("clean catalog staging: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(staging, "assets"), 0o755); err != nil {
		return Result{}, fmt.Errorf("create catalog staging: %w", err)
	}
	for index := range entries {
		if entries[index].Status != StatusPresent {
			continue
		}
		name := entries[index].ID + ".png"
		data, readErr := os.ReadFile(sources[index])
		if readErr != nil {
			return Result{}, fmt.Errorf("read catalog preview %q: %w", entries[index].ID, readErr)
		}
		if writeErr := os.WriteFile(filepath.Join(staging, "assets", name), data, 0o644); writeErr != nil {
			return Result{}, fmt.Errorf("write catalog preview %q: %w", entries[index].ID, writeErr)
		}
		entries[index].PreviewPath = filepath.ToSlash(filepath.Join("assets", name))
	}
	metadata := Metadata{PackVersion: p.Version, Entries: entries}
	metadataData, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return Result{}, fmt.Errorf("encode catalog metadata: %w", err)
	}
	metadataData = append(metadataData, '\n')
	if err := os.WriteFile(filepath.Join(staging, "catalog.json"), metadataData, 0o644); err != nil {
		return Result{}, fmt.Errorf("write catalog metadata: %w", err)
	}
	var html bytes.Buffer
	if err := catalogTemplate.Execute(&html, catalogView{Groups: groupEntries(entries)}); err != nil {
		return Result{}, fmt.Errorf("render catalog HTML: %w", err)
	}
	if err := os.WriteFile(filepath.Join(staging, "index.html"), html.Bytes(), 0o644); err != nil {
		return Result{}, fmt.Errorf("write catalog HTML: %w", err)
	}
	if err := os.RemoveAll(opts.OutputDir); err != nil {
		return Result{}, fmt.Errorf("replace catalog output: %w", err)
	}
	if err := os.Rename(staging, opts.OutputDir); err != nil {
		return Result{}, fmt.Errorf("publish catalog output: %w", err)
	}
	return Result{
		Entries:      len(entries),
		IndexPath:    filepath.Join(opts.OutputDir, "index.html"),
		MetadataPath: filepath.Join(opts.OutputDir, "catalog.json"),
	}, nil
}

type usageFile struct {
	Path string
	Data []byte
}

func readUsageFiles(root string) ([]usageFile, error) {
	if strings.TrimSpace(root) == "" {
		return nil, nil
	}
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan map usage: %w", err)
	}
	sort.Strings(paths)
	files := make([]usageFile, 0, len(paths))
	for _, path := range paths {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, fmt.Errorf("read map usage %q: %w", path, readErr)
		}
		relative, relativeErr := filepath.Rel(root, path)
		if relativeErr != nil {
			return nil, fmt.Errorf("relativize map usage %q: %w", path, relativeErr)
		}
		files = append(files, usageFile{Path: filepath.ToSlash(relative), Data: data})
	}
	return files, nil
}

func collectEntries(all []targets.Target, deployDir string, usageFiles []usageFile) ([]Entry, []string, error) {
	var entries []Entry
	var sources []string
	for _, target := range all {
		if target.AnimationID != "" {
			continue
		}
		path, err := targets.DeployPath(deployDir, target)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve catalog target %q: %w", target.ID, err)
		}
		entry := Entry{
			ID:          target.ID,
			ObjectID:    target.ObjectID,
			PartID:      target.SetPartID,
			Family:      target.Family,
			Description: target.ObjectDesc,
			DeployPath:  filepath.ToSlash(target.DeployTemplate),
			LogicalSize: target.Size,
			Status:      StatusPlaceholder,
			MapUsage:    matchingUsage(target, usageFiles),
		}
		if target.SetPartID != "" {
			entry.AtomicSet = target.ObjectID
			entry.Description = target.SetPartDesc
		}
		data, readErr := os.ReadFile(path)
		if errors.Is(readErr, os.ErrNotExist) {
			entries = append(entries, entry)
			sources = append(sources, "")
			continue
		}
		if readErr != nil {
			return nil, nil, fmt.Errorf("read production sprite %q: %w", target.ID, readErr)
		}
		config, decodeErr := png.DecodeConfig(bytes.NewReader(data))
		if decodeErr != nil {
			return nil, nil, fmt.Errorf("decode production sprite %q as PNG: %w", target.ID, decodeErr)
		}
		entry.Status = StatusPresent
		entry.IntrinsicSize = pack.Size{Width: config.Width, Height: config.Height}
		entry.Density = density(entry.IntrinsicSize, entry.LogicalSize)
		sum := sha256.Sum256(data)
		entry.SHA256 = hex.EncodeToString(sum[:])
		entries = append(entries, entry)
		sources = append(sources, path)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Family != entries[j].Family {
			return entries[i].Family < entries[j].Family
		}
		return entries[i].ID < entries[j].ID
	})
	// Keep source paths aligned after sorting entries.
	sourceByID := make(map[string]string, len(sources))
	for index, target := range staticTargets(all) {
		if index < len(sources) {
			sourceByID[target.ID] = sources[index]
		}
	}
	sources = sources[:0]
	for _, entry := range entries {
		sources = append(sources, sourceByID[entry.ID])
	}
	return entries, sources, nil
}

func staticTargets(all []targets.Target) []targets.Target {
	out := make([]targets.Target, 0, len(all))
	for _, target := range all {
		if target.AnimationID == "" {
			out = append(out, target)
		}
	}
	return out
}

func matchingUsage(target targets.Target, files []usageFile) []string {
	needles := [][]byte{[]byte(strconv.Quote(target.ObjectID))}
	if target.ID != target.ObjectID {
		needles = append(needles, []byte(strconv.Quote(target.ID)))
	}
	var matches []string
	for _, file := range files {
		for _, needle := range needles {
			if bytes.Contains(file.Data, needle) {
				matches = append(matches, file.Path)
				break
			}
		}
	}
	return matches
}

func density(intrinsic, logical pack.Size) string {
	if logical.Width > 0 && logical.Height > 0 &&
		intrinsic.Width%logical.Width == 0 && intrinsic.Height%logical.Height == 0 {
		x := intrinsic.Width / logical.Width
		y := intrinsic.Height / logical.Height
		if x == y && x > 0 {
			return strconv.Itoa(x) + "x"
		}
	}
	return fmt.Sprintf(
		"%dx%d over %dx%d",
		intrinsic.Width,
		intrinsic.Height,
		logical.Width,
		logical.Height,
	)
}

func groupEntries(entries []Entry) []catalogGroup {
	var groups []catalogGroup
	for _, entry := range entries {
		if len(groups) == 0 || groups[len(groups)-1].Family != entry.Family {
			groups = append(groups, catalogGroup{Family: entry.Family})
		}
		groups[len(groups)-1].Entries = append(groups[len(groups)-1].Entries, entry)
	}
	return groups
}

var catalogTemplate = template.Must(template.New("catalog").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Static sprite catalog</title><style>
body{margin:0;background:#101218;color:#e8e3d9;font:14px system-ui,sans-serif}main{padding:24px}h1,h2{color:#f1c96a}.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(360px,1fr));gap:16px}.card{background:#191d27;border:1px solid #353c4d;border-radius:8px;padding:14px;overflow:hidden}.id{font:600 13px ui-monospace,monospace;overflow-wrap:anywhere}.meta{color:#adb6c8;line-height:1.45}.previews{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:8px;margin-top:10px}.preview{min-height:140px;background:#0b0d12;border:1px solid #303747;overflow:auto;padding:6px}.preview b{display:block;color:#c8d0df;font-size:11px;margin-bottom:5px}.preview img{display:block;max-width:none;image-rendering:pixelated}.missing{display:grid;place-items:center;min-height:140px;color:#e29578;border:1px dashed #8d5545}.usage{overflow-wrap:anywhere}
</style></head><body><main><h1>Static sprite catalog</h1>
{{range .Groups}}<section><h2>{{.Family}}</h2><div class="grid">{{range .Entries}}<article class="card">
<div class="id">{{.ID}}</div><p>{{.Description}}</p><div class="meta">status: {{.Status}}<br>source: {{.IntrinsicSize.Width}}×{{.IntrinsicSize.Height}}; logical: {{.LogicalSize.Width}}×{{.LogicalSize.Height}}; density: {{.Density}}<br>deploy: {{.DeployPath}}<br><span class="usage">maps: {{if .MapUsage}}{{range $i,$v := .MapUsage}}{{if $i}}, {{end}}{{$v}}{{end}}{{else}}unused{{end}}</span></div>
{{if .PreviewPath}}<div class="previews"><div class="preview"><b>Native source</b><img src="{{.PreviewPath}}"></div><div class="preview"><b>Logical game size</b><img src="{{.PreviewPath}}" style="width:{{.LogicalSize.Width}}px;height:{{.LogicalSize.Height}}px"></div><div class="preview"><b>Enlarged inspection</b><img src="{{.PreviewPath}}" style="width:{{.LogicalSize.Width}}px;height:{{.LogicalSize.Height}}px;transform:scale(2);transform-origin:top left"></div></div>{{else}}<div class="missing">planned placeholder — production PNG missing</div>{{end}}
</article>{{end}}</div></section>{{end}}</main></body></html>
`))
