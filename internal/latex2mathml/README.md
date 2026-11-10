# latex2mathml

> **eden-press fork.** This copy is the package
> `go.aocyber.ai/eden-press/internal/latex2mathml`, patched for eden-press
> (see NOTICE). Import it only from inside eden-press; importing the upstream
> path below gives the unpatched converter (eden-press#31).

This libary convert latex formula to mathml string and depend only 
on [etree](https://github.com/neruyzo/etree).

```go
package main

import (
	"fmt"
	"git.sr.ht/~mekyt/latex2mathml"
)

func main() {
	formula := `x = {-b \pm \sqrt{b^2-4ac} \over 2a}`
	xmlns := "http://www.w3.org/1998/Math/MathML"
	display := "inline"
	indent := 0

	fmt.Println(latex2mathml.Convert(
		formula,
		xmlns,
		display,
		indent,
	))
}
```

## Disclamer

This is a translation of the library written in Python 
[latex2mathml](https://github.com/roniemartinez/latex2mathml). 
A big thanks to it's authors.
