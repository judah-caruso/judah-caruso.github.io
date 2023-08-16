package main

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func Parse(src string) Rivit {
	var (
		parsed = make([]Line, 0)
		lines  = strings.Split(src, "\n")
	)

	li := 0
	for li < len(lines) {
		rawLine := lines[li]
		li += 1

		line := trimRightSpace(string(rawLine))
		if len(line) == 0 {
			continue
		}

		delim := line[0]
		switch delim {
		case '#': // Comment
			continue
		case '+': // NavLink
			line = line[1:]

			link := trimLeftSpace(line)
			if len(link) != 0 {
				parsed = append(parsed, NavLink(link))
			}
		case '@': // Embed
			var (
				path    string
				altText string
			)

			line = trimLeftSpace(line[1:])
			if idx := strings.IndexRune(line, ' '); idx != -1 {
				path = line[:idx]
				altText = trimLeftSpace(line[idx:])
			} else {
				path = line
			}

			if len(path) != 0 {
				var alt []StyledText
				if len(altText) != 0 {
					alt = ParseStyledText(altText)
				}

				parsed = append(parsed, Embed{
					Path: path,
					Alt:  alt,
				})
			}
		case '.': // Lists
			level := countPrefix(line, delim)
			line = trimLeftSpace(line[level:])
			if len(line) == 0 {
				continue
			}

			var (
				end       = li
				rootList  = List{ListItem{Level: level}}
				curParent = &rootList[0]
				curItem   = curParent
			)

			curItem.Value = ParseStyledText(line)

		list_loop:
			for end < len(lines) {
				l := lines[end]
				if len(l) == 0 || l[0] != delim {
					break list_loop
				}

				sl := countPrefix(l, delim)
				if sl == 0 {
					break list_loop
				}

				text := trimLeftSpace(l[sl:])
				if len(text) == 0 {
					end += 1
					continue list_loop
				}

				switch sl {
				case 1:
					// We need to append a new parent to the root list
					rootList = append(rootList, ListItem{Level: sl})
					curParent = &rootList[len(rootList)-1]
					curItem = curParent
				case 2:
					// We need to append a new item to the current parent
					curParent.Sublist = append(curParent.Sublist, ListItem{Level: sl})
					curItem = &curParent.Sublist[len(curParent.Sublist)-1]
				default:
					curItem.Sublist = append(curItem.Sublist, ListItem{Level: sl})
					curItem = &curParent.Sublist[len(curItem.Sublist)-1]
				}

				curItem.Value = ParseStyledText(text)
				end += 1
			}

			li = end
			parsed = append(parsed, rootList)
		case ' ', '\t': // Block
			var (
				start  = li - 1
				end    = start
				indent = countPrefix(line, delim)
			)

		block_loop:
			for end < len(lines) {
				i := countPrefix(lines[end], delim)
				if i < indent {
					break block_loop
				}

				end += 1
			}

			body := lines[start:end]
			if len(body) != 0 {
				parsed = append(parsed, Block{
					Indent: indent,
					Body:   body,
				})
			}

			li = end
		default:
			var (
				tmp      = line
				hadUpper = false
			)

		header_loop:
			for len(tmp) > 0 {
				r, w := utf8.DecodeRuneInString(tmp)
				if unicode.Is(unicode.Latin, r) {
					hadUpper = unicode.IsUpper(r)
					if !hadUpper {
						break header_loop
					}
				}

				tmp = tmp[w:]
			}

			// Headers
			if len(tmp) == 0 && hadUpper {
				parsed = append(parsed, Header(line))
				continue
			}

			// Paragraphs
			p := ParseStyledText(line)
			parsed = append(parsed, Paragraph(p))
		}
	}

	return Rivit(parsed)
}

func ParseStyledText(line string) []StyledText {
	var (
		i      = 0
		parsed = make([]StyledText, 0)
	)

	for i < len(line) {
		switch c := line[i]; c {
		case '*': // Italic, Bold
			var (
				text  string
				start = i
				end   = i
				style = StyleItalic
			)

			end += 1

			if end < len(line) && line[end] == c {
				end += 1
				style = StyleBold
			}

			if style == StyleBold {
				for end < len(line) {
					if line[end] == c && (end+1 < len(line) && line[end+1] == c) {
						end += 2
						break
					}

					end += 1
				}

				text = trimSpace(line[start+2 : end-2])
			} else {
				for end < len(line) {
					if line[end] == c {
						end += 1
						break
					}

					end += 1
				}

				text = trimSpace(line[start+1 : end-1])
			}

			if len(text) != 0 {
				parsed = append(parsed, StyledText{
					Style: style,
					Value: text,
				})
			}

			i = end
		case '{', '[': // InternalLink, ExternalLink
			var (
				start    = i
				end      = i
				endDelim = c + 2 // } or ]
			)

		link_loop:
			for end < len(line) {
				if line[end] == endDelim {
					end += 1
					break link_loop
				}

				end += 1
			}

			var (
				link  string
				value string
			)

			link = trimSpace(line[start+1 : end-1])
			if idx := strings.IndexRune(link, ' '); idx != -1 {
				value = trimLeftSpace(link[idx:])
				link = trimRightSpace(link[:idx])
			}

			if len(link) != 0 {
				style := StyleInternalLink
				if c == '[' {
					style = StyleExternalLink
				}

				parsed = append(parsed, StyledText{
					Style: style,
					Value: value,
					Link:  link,
				})
			}

			i = end
		default: // StyledText, Escapes
			var (
				start = i
				end   = i
			)

		text_loop:
			for end < len(line) {
				switch line[end] {
				case '\\':
					if end+1 < len(line) {
						c, w := utf8.DecodeRuneInString(line[end+1:])
						if w > 1 {
							rn := []rune(line)
							rn = append(rn[:end], rn[end+1:]...)
							line = string(rn)
						} else if !unicode.IsSpace(rune(c)) {
							b := []byte(line)
							b = append(b[:end], b[end+1:]...)
							line = string(b)
						}

						end += w
						continue text_loop
					}
				case '*', '{', '[': // Only swap states if the next isn't whitespace (similar to markdown)
					if end+1 < len(line) && !unicode.IsSpace(rune(line[end+1])) {
						break text_loop
					}
				}

				end += 1
			}

			text := line[start:end]
			if len(text) != 0 {
				parsed = append(parsed, StyledText{
					Style: StyleNone,
					Value: text,
				})
			}

			i = end
		}
	}

	return parsed
}

type Rivit []Line

type LineKind int

const (
	LineNone LineKind = iota
	LineHeader
	LineNavLink
	LineParagraph
	LineBlock
	LineList
	LineEmbed
)

type TextStyle int

const (
	StyleNone TextStyle = iota
	StyleItalic
	StyleBold
	StyleInternalLink
	StyleExternalLink
)

type (
	Line interface {
		Kind() LineKind
	}

	Header    string
	NavLink   string
	Paragraph []StyledText
	List      []ListItem

	Embed struct {
		Path string
		Alt  []StyledText
	}

	Block struct {
		Indent int
		Body   []string
	}

	ListItem struct {
		Level   int
		Value   []StyledText
		Sublist List
	}
	StyledText struct {
		Style TextStyle
		Value string
		Link  string // Only set if Style is StyleInternalLink, StyleExternalLink
	}
)

func (_ Header) Kind() LineKind    { return LineHeader }
func (_ NavLink) Kind() LineKind   { return LineNavLink }
func (_ Paragraph) Kind() LineKind { return LineParagraph }
func (_ Embed) Kind() LineKind     { return LineEmbed }
func (_ Block) Kind() LineKind     { return LineBlock }
func (_ List) Kind() LineKind      { return LineList }

func trimRightSpace(str string) string { return strings.TrimRightFunc(str, unicode.IsSpace) }
func trimLeftSpace(str string) string  { return strings.TrimLeftFunc(str, unicode.IsSpace) }
func trimSpace(str string) string      { return strings.TrimFunc(str, unicode.IsSpace) }

func countPrefix(str string, delim byte) (count int) {
	for i := range str {
		if str[i] != delim {
			break
		}

		count += 1
	}

	return
}
