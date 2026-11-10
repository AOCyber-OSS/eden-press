// A module outside eden-press that imports it the way AODex does. Go ignores
// replace directives in dependency modules, so building here shows what a
// real consumer compiles (eden-press#31). The replace below stands in for a
// version requirement on go.aocyber.ai/eden-press: it points at this checkout
// so CI tests the commit under review.
//
// The module path is deliberately outside go.aocyber.ai/eden-press: a path
// inside it could import eden-press's internal packages, which no real
// consumer can. Only eden-press is required here: its dependencies are
// resolved at test time into a scratch copy of this file (see `make
// consumer-test`), so a dependency bump in the root go.mod never leaves a
// second go.mod and go.sum stale, and running the test never edits them.
module aocyber.test/edenpress-consumer

go 1.27.3

require go.aocyber.ai/eden-press v0.0.0

replace go.aocyber.ai/eden-press => ../..
