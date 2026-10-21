module go.aocyber.ai/eden-press

go 1.27.3

require (
	codeberg.org/go-latex/latex v0.3.0
	git.sr.ht/~mekyt/latex2mathml v0.0.0-20231214134936-808832af73fc
	github.com/alecthomas/chroma/v2 v2.27.0
	github.com/chromedp/cdproto v0.0.0-20260714215040-dc233986426f
	github.com/chromedp/chromedp v0.16.0
	github.com/fsnotify/fsnotify v1.10.1
	github.com/knadh/koanf/parsers/json v1.0.0
	github.com/knadh/koanf/parsers/toml v0.1.0
	github.com/knadh/koanf/parsers/yaml v1.1.0
	github.com/knadh/koanf/providers/env v1.1.0
	github.com/knadh/koanf/providers/file v1.2.1
	github.com/knadh/koanf/providers/posflag v1.0.1
	github.com/knadh/koanf/v2 v2.3.5
	github.com/microcosm-cc/bluemonday v1.0.27
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c
	github.com/spf13/cobra v1.10.2
	github.com/tdewolff/parse/v2 v2.8.13
	github.com/yuin/goldmark v1.8.4
	github.com/yuin/goldmark-emoji v1.0.6
	github.com/yuin/goldmark-highlighting/v2 v2.0.0-20230729083705-37449abec8cc
	golang.org/x/net v0.57.0
)

require (
	git.sr.ht/~sbinet/gg v0.7.0 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/chromedp/sysutil v1.1.0 // indirect
	github.com/dlclark/regexp2/v2 v2.2.1 // indirect
	github.com/go-json-experiment/json v0.0.0-20260623181947-01eb4420fa68 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/gobwas/ws v1.4.0 // indirect
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 // indirect
	github.com/gorilla/css v1.0.1 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/knadh/koanf/maps v0.1.2 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/neruyzo/etree v0.0.0-20230816193247-70b7b06b18ad // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/image v0.40.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
)

// Objective 8 (math-fidelity hardening): latex2mathml is vendored in-repo as a
// fork at ./internal/latex2mathml so the five converter root-causes (\sqrt[n]
// radicand loss, big-operator limit stacking, binom/pmatrix fence, aligned→mtable,
// mathvariant→codepoint) can be patched (08-02/08-03). The vendored module keeps
// its own module path (git.sr.ht/~mekyt/latex2mathml), so press/math import sites
// are unchanged — this replace directive is the sole fork seam. Its MIT license
// (Copyright (c) 2023 mekyt) is preserved verbatim; see NOTICE.
replace git.sr.ht/~mekyt/latex2mathml => ./internal/latex2mathml
