package main

import (
	"testing"
)

func TestHeaders(t *testing.T) {
	src := `
BASIC HEADER
IT'S ANOTHER HEADER
THIS IS A 헤더 WITH UNICODE
0000 IT STILL WORKS
	`

	riv := Parse(src)
	expectEqual(t, 4, len(riv))

	expectEqual(t, LineHeader, riv[0].Kind())
	expectEqual(t, "BASIC HEADER", riv[0].(Header))

	expectEqual(t, LineHeader, riv[1].Kind())
	expectEqual(t, "IT'S ANOTHER HEADER", riv[1].(Header))

	expectEqual(t, LineHeader, riv[2].Kind())
	expectEqual(t, "THIS IS A 헤더 WITH UNICODE", riv[2].(Header))

	expectEqual(t, LineHeader, riv[3].Kind())
	expectEqual(t, "0000 IT STILL WORKS", riv[3].(Header))
}

func TestLinks(t *testing.T) {
	src := `
+nav_link
+   it-works
+      still/works
+   *also{works}*
[  a.b external    ][c.d ..*ext*]
this is an {int *[internal link]***}
	`

	riv := Parse(src)
	expectEqual(t, 6, len(riv))

	expectEqual(t, LineNavLink, riv[0].Kind())
	expectEqual(t, "nav_link", riv[0].(NavLink))

	expectEqual(t, LineNavLink, riv[1].Kind())
	expectEqual(t, "it-works", riv[1].(NavLink))

	expectEqual(t, LineNavLink, riv[2].Kind())
	expectEqual(t, "still/works", riv[2].(NavLink))

	expectEqual(t, LineNavLink, riv[3].Kind())
	expectEqual(t, "*also{works}*", riv[3].(NavLink))

	{
		expectEqual(t, LineParagraph, riv[4].Kind())

		p := riv[4].(Paragraph)
		expectEqual(t, 2, len(p))

		expectEqual(t, StyleExternalLink, p[0].Style)
		expectEqual(t, "a.b", p[0].Link)
		expectEqual(t, "external", p[0].Value)

		expectEqual(t, StyleExternalLink, p[1].Style)
		expectEqual(t, "c.d", p[1].Link)
		expectEqual(t, "..*ext*", p[1].Value)
	}

	{

		expectEqual(t, LineParagraph, riv[5].Kind())

		p := riv[5].(Paragraph)
		expectEqual(t, 2, len(p))

		expectEqual(t, StyleNone, p[0].Style)
		expectEqual(t, "this is an ", p[0].Value)

		expectEqual(t, StyleInternalLink, p[1].Style)
		expectEqual(t, "int", p[1].Link)
		expectEqual(t, "*[internal link]***", p[1].Value)
	}
}

func TestParagraphsAndEscapes(t *testing.T) {
	src := `
No style.
1 * 1 = { 10 }
\*escaped\*\[text\]\{works\}
\\escaped \한글 \\ and \\character\
	`

	riv := Parse(src)
	expectEqual(t, 4, len(riv))

	{
		expectEqual(t, LineParagraph, riv[0].Kind())

		p := riv[0].(Paragraph)
		expectEqual(t, 1, len(p))
		expectEqual(t, StyleNone, p[0].Style)
		expectEqual(t, "No style.", p[0].Value)
	}

	{
		expectEqual(t, LineParagraph, riv[1].Kind())

		p := riv[1].(Paragraph)
		expectEqual(t, 1, len(p))
		expectEqual(t, StyleNone, p[0].Style)
		expectEqual(t, "1 * 1 = { 10 }", p[0].Value)
	}

	{
		expectEqual(t, LineParagraph, riv[2].Kind())

		p := riv[2].(Paragraph)
		expectEqual(t, 1, len(p))
		expectEqual(t, StyleNone, p[0].Style)
		expectEqual(t, "*escaped*[text]{works}", p[0].Value)
	}

	{
		expectEqual(t, LineParagraph, riv[3].Kind())

		p := riv[3].(Paragraph)
		expectEqual(t, 1, len(p))
		expectEqual(t, StyleNone, p[0].Style)
		expectEqual(t, "\\escaped 한글 \\ and \\character\\", p[0].Value)
	}
}

func TestEmbeds(t *testing.T) {
	src := `
@foo.png it's an image
@  ./test/.foo \ **some file**
@ 안녕.tga [foo.bar Hello]
@\.txt
	`

	riv := Parse(src)
	expectEqual(t, 4, len(riv))

	expectEqual(t, LineEmbed, riv[0].Kind())
	expectEqual(t, LineEmbed, riv[1].Kind())
	expectEqual(t, LineEmbed, riv[2].Kind())
	expectEqual(t, LineEmbed, riv[3].Kind())
}

func TestLists(t *testing.T) {
	src := `
.. \ nice
. **1**
.. {1.1}
.. [1.2]
... 1.2.1
.. 1.3
... 1.3.1
. **2**
... 2.3.1
.. 2.2.1
. *3*
	`

	riv := Parse(src)
	expectEqual(t, 1, len(riv))
	expectEqual(t, LineList, riv[0].Kind())

	lst := riv[0].(List)
	expectEqual(t, 4, len(lst))

	{
		l := lst[0]
		expectEqual(t, 0, len(l.Sublist))
		expectEqual(t, 1, len(l.Value))

		i := l.Value[0]
		expectEqual(t, StyleNone, i.Style)
		expectEqual(t, "\\ nice", i.Value)
	}

	{
		l := lst[1]
		expectEqual(t, 3, len(l.Sublist))
	}

	{
		l := lst[2]
		expectEqual(t, 2, len(l.Sublist))
	}

	{
		l := lst[3]
		expectEqual(t, 0, len(l.Sublist))
	}
}

type (
	Ordered interface {
		Integer | Float | ~string
	}
	Signed interface {
		~int | ~int8 | ~int16 | ~int32 | ~int64
	}
	Unsigned interface {
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr
	}
	Integer interface {
		Signed | Unsigned
	}
	Float interface {
		~float32 | ~float64
	}
)

func expectEqual[T Ordered](t *testing.T, expected, actual T) {
	if actual != expected {
		t.Fatalf("expected '%v' but was given '%v'", expected, actual)
	}
}
