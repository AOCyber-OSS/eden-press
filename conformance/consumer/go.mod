// A module outside eden-press that imports it the way AODex does. Go ignores
// replace directives in dependency modules, so building here shows what a
// real consumer compiles (eden-press#31). The replace below stands in for a
// version requirement on go.aocyber.ai/eden-press: it points at this checkout
// so CI tests the commit under review.
module go.aocyber.ai/eden-press/conformance/consumer

go 1.27.3

require go.aocyber.ai/eden-press v0.0.0

require (
	codeberg.org/go-latex/latex v0.3.0 // indirect
	git.sr.ht/~sbinet/gg v0.7.0 // indirect
	github.com/alecthomas/chroma/v2 v2.27.0 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/dlclark/regexp2/v2 v2.2.1 // indirect
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 // indirect
	github.com/gorilla/css v1.0.1 // indirect
	github.com/microcosm-cc/bluemonday v1.0.27 // indirect
	github.com/neruyzo/etree v0.0.0-20230816193247-70b7b06b18ad // indirect
	github.com/tdewolff/parse/v2 v2.8.13 // indirect
	github.com/yuin/goldmark v1.8.4 // indirect
	github.com/yuin/goldmark-emoji v1.0.6 // indirect
	github.com/yuin/goldmark-highlighting/v2 v2.0.0-20230729083705-37449abec8cc // indirect
	golang.org/x/image v0.40.0 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/text v0.40.0 // indirect
)

replace go.aocyber.ai/eden-press => ../..
