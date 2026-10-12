package project

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/nuitsjp/ucspec/internal/model"
	"github.com/nuitsjp/ucspec/internal/update"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type Item struct {
	Path   string      `json:"path"`
	Action string      `json:"action"`
	Data   []byte      `json:"-"`
	Mode   fs.FileMode `json:"-"`
}
type Options struct {
	Root       string
	Name       string
	Version    string
	Repository string
	Executable string
	DryRun     bool
}

func Init(assets fs.FS, o Options) (items []Item, err error) {
	root, err := filepath.Abs(o.Root)
	if err != nil {
		return nil, err
	}
	v, err := update.CleanVersion(o.Version)
	if err != nil {
		return nil, err
	}
	if o.Name == "" {
		o.Name = filepath.Base(root)
	}
	if err = update.ValidateRepository(o.Repository); err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	modes := map[string]fs.FileMode{}
	err = fs.WalkDir(assets, "template", func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		b, e := fs.ReadFile(assets, path)
		if e != nil {
			return e
		}
		files[strings.TrimPrefix(path, "template/")] = b
		return nil
	})
	if err != nil {
		return nil, err
	}
	var config model.Project
	if err = json.Unmarshal(files["spec/project.json"], &config); err != nil {
		return nil, err
	}
	config.Name = o.Name
	config.Repository = o.Repository
	b, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, err
	}
	files["spec/project.json"] = append(b, '\n')
	files[".ucspec-version"] = []byte(v + "\n")
	files[".tools/.gitignore"] = []byte("*\n!.gitignore\n")
	files["artifacts/.gitignore"] = []byte("*\n!.gitignore\n")
	for source, target := range map[string]string{
		"authoring.md":              "spec/AUTHORING.md",
		"manual.md":                 "spec/USER-GUIDE.md",
		"schemas/spec.schema.json":  "spec/ucspec.schema.json",
		"ucspec-authoring-skill.md": ".agents/skills/ucspec-authoring/SKILL.md",
	} {
		b, er := fs.ReadFile(assets, source)
		if er != nil {
			return nil, er
		}
		files[target] = b
	}
	files["scripts/ucspec.ps1"] = []byte(invokeScript)
	files["scripts/setup-ucspec.ps1"] = []byte(strings.ReplaceAll(setupScript, "__REPOSITORY__", o.Repository))
	files["spec/AGENTS.md"] = []byte(agentGuide)
	if o.Executable != "" {
		b, er := os.ReadFile(o.Executable)
		if er != nil {
			return nil, er
		}
		name := ".tools/ucspec/" + v + "/ucspec.exe"
		files[name] = b
		modes[name] = 0755
	}
	conflict := false
	for _, rel := range model.Keys(files) {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err = SafePath(root, path); err != nil {
			return nil, err
		}
		mode := modes[rel]
		if mode == 0 {
			mode = 0644
		}
		it := Item{Path: rel, Action: "create", Data: files[rel], Mode: mode}
		old, er := os.ReadFile(path)
		if er == nil {
			if bytes.Equal(old, it.Data) {
				it.Action = "unchanged"
			} else {
				it.Action = "conflict"
				conflict = true
			}
		} else if !os.IsNotExist(er) {
			return nil, er
		}
		items = append(items, it)
	}
	if o.DryRun {
		return items, nil
	}
	if conflict {
		return items, fmt.Errorf("initialization refused: existing managed files differ; no files changed (use --dry-run)")
	}
	created := []string{}
	defer func() {
		if err != nil {
			for i := len(created) - 1; i >= 0; i-- {
				_ = os.Remove(created[i])
			}
		}
	}()
	for _, it := range items {
		if it.Action != "create" {
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(it.Path))
		if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return items, err
		}
		var f *os.File
		f, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, it.Mode)
		if err != nil {
			return items, err
		}
		created = append(created, path)
		_, err = f.Write(it.Data)
		if err == nil {
			err = f.Sync()
		}
		ce := f.Close()
		if err != nil {
			return items, err
		}
		if ce != nil {
			return items, ce
		}
	}
	return items, nil
}

// SafePath rejects path traversal and symlinks in every existing path component.
func SafePath(root, path string) error {
	r, err := filepath.Rel(root, path)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes root")
	}
	cur := root
	parts := append([]string{""}, strings.Split(r, string(filepath.Separator))...)
	for _, p := range parts {
		cur = filepath.Join(cur, p)
		f, e := os.Lstat(cur)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink/reparse point: %s", cur)
		}
	}
	return nil
}

const agentGuide = `# このプロダクトの仕様編集

正本は project.json とそこから参照する構造化データです。

仕様の作成・変更や未網羅のレビューには、プロジェクトルートの
[ucspec-authoring スキル](../.agents/skills/ucspec-authoring/SKILL.md) を使用します。
ユーザーの目的と期待結果からシナリオを記述し、未網羅の業務上の意味と設計要求を残します。
既知の要求に基づく編集を進め、判断に必要な情報が不足する場合に質問します。

- 共通モデルの状態・イベント・ディシジョンを参照し、ユースケースごとに複製しない。
- シナリオの goal と expectedOutcome を記述し、手順はイベント名と通る分岐名で書く。
- 検査を通すために scope、ends、coverage や limits を緩めない。
- レビューの記録で生の網羅率は変わらない。古いレビューの根拠は再評価する。
- check と build を実行し、反例、未網羅、設計要求と未検証事項を報告する。
- ツールの更新は明示操作とし、分析中にネットワーク取得や更新を行わない。

記法は AUTHORING.md、操作方法は USER-GUIDE.md を参照してください。
`
const invokeScript = `# Windows PowerShell 5.1 / PowerShell 7
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$version = (Get-Content -LiteralPath (Join-Path $root '.ucspec-version') -Raw).Trim()
if ($version -notmatch '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$') { throw 'Invalid .ucspec-version' }
$exe = Join-Path $root ".tools/ucspec/$version/ucspec.exe"
if (-not (Test-Path -LiteralPath $exe)) { throw 'Run scripts/setup-ucspec.ps1 first.' }
Push-Location $root
try { & $exe @args; $code = $LASTEXITCODE } finally { Pop-Location }
exit $code
`

// This bootstrapper downloads only the pinned version. It does not run downloaded scripts.
const setupScript = `param([string]$Repository = '__REPOSITORY__')
$ErrorActionPreference = 'Stop'
if ($Repository -notmatch '^[A-Za-z0-9][A-Za-z0-9_.-]*/[A-Za-z0-9][A-Za-z0-9_.-]*$' -or $Repository.Contains('..')) { throw 'Invalid repository' }
$root = Split-Path $PSScriptRoot -Parent
$version = (Get-Content -LiteralPath (Join-Path $root '.ucspec-version') -Raw).Trim()
if ($version -notmatch '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$') { throw 'Invalid pinned version' }
$dest = Join-Path $root ".tools/ucspec/$version/ucspec.exe"
if (Test-Path -LiteralPath $dest) { Write-Host "Already installed: $dest"; exit 0 }
$headers = @{ Accept='application/vnd.github+json'; 'User-Agent'='ucspec-bootstrap' }
if ($env:GH_TOKEN) { $headers.Authorization = "Bearer $env:GH_TOKEN" } elseif ($env:GITHUB_TOKEN) { $headers.Authorization = "Bearer $env:GITHUB_TOKEN" }
$release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repository/releases/tags/v$version" -Headers $headers
if ($release.draft -or $release.prerelease -or $release.tag_name -ne "v$version") { throw 'Expected a stable matching release' }
$name = "ucspec_${version}_windows_amd64.zip"
$zipAsset = @($release.assets | Where-Object name -eq $name)
$sumAsset = @($release.assets | Where-Object name -eq 'checksums.txt')
if ($zipAsset.Count -ne 1 -or $sumAsset.Count -ne 1) { throw 'Release assets missing or duplicated' }
$temp = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $temp | Out-Null
try {
  $headers.Accept = 'application/octet-stream'
  $zip = Join-Path $temp $name
  $sum = Join-Path $temp 'checksums.txt'
  Invoke-WebRequest -UseBasicParsing -Uri "https://api.github.com/repos/$Repository/releases/assets/$($zipAsset[0].id)" -Headers $headers -OutFile $zip
  Invoke-WebRequest -UseBasicParsing -Uri "https://api.github.com/repos/$Repository/releases/assets/$($sumAsset[0].id)" -Headers $headers -OutFile $sum
  $matchesFound = @(Get-Content -LiteralPath $sum | Where-Object { $_ -match ('^[0-9a-fA-F]{64}\s+\*?' + [regex]::Escape($name) + '$') })
  if ($matchesFound.Count -ne 1) { throw 'Missing or duplicate SHA-256 entry' }
  $expected = ($matchesFound[0] -split '\s+')[0]
  if ((Get-FileHash -Algorithm SHA256 -LiteralPath $zip).Hash -ne $expected) { throw 'Checksum mismatch' }
  Add-Type -AssemblyName System.IO.Compression.FileSystem
  $archive = [IO.Compression.ZipFile]::OpenRead($zip)
  try {
    $entries = @($archive.Entries | Where-Object FullName -eq 'ucspec.exe')
    if ($entries.Count -ne 1 -or $entries[0].Length -gt 67108864) { throw 'Invalid executable ZIP entry' }
    $stage = Join-Path $temp 'ucspec.exe'
    [IO.Compression.ZipFileExtensions]::ExtractToFile($entries[0], $stage, $false)
  } finally { $archive.Dispose() }
  $info = & $stage version --json | ConvertFrom-Json
  if ($LASTEXITCODE -ne 0 -or $info.version -ne $version) { throw 'Binary version does not match release' }
  New-Item -ItemType Directory -Force -Path (Split-Path $dest -Parent) | Out-Null
  Move-Item -LiteralPath $stage -Destination $dest
  Write-Host "Installed $dest"
} finally { Remove-Item -LiteralPath $temp -Recurse -Force }
`
