package main

import (
	"bytes"
	"encoding/base64"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	inPath  = "riv"
	resPath = "res"
	outPath = "web"

	siteBase         = "https://github.com/judah-caruso/judah-caruso.github.io"
	siteTitle        = "beton brutalism"
	siteStylePath    = "style.css"    // within resPath
	siteTemplatePath = "template.htm" // within resPath

	classP       = "paragraph"
	classH1      = "title"
	classH2      = "header"
	classUl      = "list"
	classLi      = "list-item"
	classPre     = "code"
	classBold    = "bold"
	classItalic  = "italic"
	classIntLink = "internal link"
	classExtLink = "external link"
	classEmbed   = "embed"
	classCaption = "embed-caption"
	classImage   = "image"
	classSvg     = "vector"
	classSound   = "sound"
)

var (
	flagPort        = flag.Int("port", 8080, "sets the port for the HTTP server")
	flagStartServer = flag.Bool("server", false, "start a simple HTTP server after generation")
)

func main() {
	var (
		err      error
		styles   string
		template string
	)

	{
		styles, err = readEntireFile(resPath, siteStylePath)
		if err != nil {
			log.Printf("required stylesheet '%s%c%s' did not exist\n", resPath, filepath.Separator, siteStylePath)
			os.Exit(1)
		}

		styles = strings.ReplaceAll(styles, "\n", "")
	}

	{
		template, err = readEntireFile(resPath, siteTemplatePath)
		if err != nil {
			log.Printf("required template '%s%c%s' did not exist\n", resPath, filepath.Separator, siteStylePath)
			os.Exit(2)
		}

		lines := strings.Split(template, "\n")
		for i := range lines {
			lines[i] = strings.TrimSpace(lines[i])
		}

		template = strings.Join(lines, "\n")
	}

	index := &Index{
		Pages: make(map[string]*Page),
	}

	pageFiles, err := os.ReadDir(inPath)
	if err != nil {
		log.Printf("unable to open required directory '%s': %s\n", inPath, err)
		os.Exit(3)
	}

	for _, f := range pageFiles {
		if f.IsDir() {
			continue
		}

		path := f.Name()
		ext := filepath.Ext(path)
		if ext != ".riv" {
			continue
		}

		unique := path[:len(path)-len(ext)]
		index.Pages[unique] = &Page{
			LocalName:   path,
			UniqueName:  unique,
			DisplayName: strings.Title(strings.ReplaceAll(unique, "-", " ")), // @todo: remove
			OutName:     unique + ".htm",
		}

		log.Printf(".. indexed %s", path)
	}

	err = os.Mkdir(outPath, fs.ModeDir)
	if err != nil && !os.IsExist(err) {
		log.Printf("unable to create output directory '%s': %s\n", outPath, err)
		os.Exit(4)
	}

	generated := 0
	for _, page := range index.Pages {
		y, m, d := time.Now().Date()

		src, err := readEntireFile(inPath, page.LocalName)
		if err != nil {
			log.Printf("unable to open page: '%s%c%s'\n", inPath, filepath.Separator, page.LocalName)
			continue
		}

		page.Body = Parse(src)
		for _, riv := range page.Body {
			if riv.Kind() != LineNavLink {
				continue
			}

			name := string(riv.(NavLink))
			if p, ok := index.Pages[name]; ok {
				exists := false
				for _, nl := range page.Nav {
					if nl.UniqueName == p.UniqueName {
						exists = true
						break
					}
				}

				if !exists {
					p.Refs += 1
					page.Nav = append(page.Nav, p)
				}
			} else {
				log.Printf(".. page '%s' has a broken nav link '%s'\n", page.LocalName, name)
			}
		}

		// sort nav links before output
		sort.SliceStable(page.Nav, func(i, j int) bool {
			l := page.Nav[i]
			r := page.Nav[j]
			return l.UniqueName[0] < r.UniqueName[0]
		})

		var nav bytes.Buffer

		fmt.Fprintf(&nav, "<ul class=%q>", classUl)
		for _, nl := range page.Nav {
			fmt.Fprintf(&nav,
				"<li class=%q><a class=%q href=%q>%s</a></li>",
				classLi, classIntLink,
				nl.OutName, nl.DisplayName,
			)
		}
		fmt.Fprint(&nav, "</ul>")

		var body bytes.Buffer
	loop:
		for i, line := range page.Body {
			if line.Kind() == LineNavLink {
				continue
			}

			switch v := line.(type) {
			case Paragraph:
				fmt.Fprintf(&body, "<p class=%q>%s</p>", classP, styledTextToHtml(index, page, v))
			case List:
				fmt.Fprint(&body, listToHtml(index, page, v))
			case Header:
				if i == 0 {
					fmt.Fprintf(&body, "<h1 class=%q>%s</h1>", classH1, v)
				} else {
					fmt.Fprintf(&body, "<h2 class=%q>%s</h2>", classH2, v)
				}
			case Block:
				fmt.Fprintf(&body, "<pre class=%q>", classPre)
				for i, line := range v.Body {
					l := line[v.Indent:]
					l = strings.ReplaceAll(l, "<", "&lt;")
					l = strings.ReplaceAll(l, ">", "&gt;")

					fmt.Fprint(&body, l)

					if i < len(v.Body) {
						fmt.Fprint(&body, "\n")
					}
				}
				fmt.Fprint(&body, "</pre>")
			case Embed:
				type MediaKind int
				const (
					MediaPng MediaKind = iota
					MediaOgg
					MediaSvg
				)

				kind := MediaKind(0)
				switch ext := filepath.Ext(v.Path); ext {
				case ".png":
					kind = MediaPng
				case ".ogg":
					kind = MediaOgg
				case ".svg":
					kind = MediaSvg
				default:
					log.Printf(".. '%s' references an unsupported media type '%s'\n", page.LocalName, ext)
					continue loop
				}

				file, err := readEntireFile(resPath, v.Path)
				if err != nil {
					log.Printf(".. unable to open embed '%s' within '%s'", v.Path, page.LocalName)
					continue
				}

				enc := base64.StdEncoding.EncodeToString([]byte(file))

				fmt.Fprintf(&body, "<figure class=%q>", classEmbed)
				switch kind {
				case MediaPng:
					const prefix = "data:image/png;base64"
					fmt.Fprintf(&body, "<img class=%q src='%s,%s'/>", classImage, prefix, enc)
				case MediaSvg:
					const prefix = "data:image/svg+xml;base64"
					fmt.Fprintf(&body, "<img class=%q src='%s,%s'/>", classSvg, prefix, enc)
				case MediaOgg:
					const prefix = "data:audio/ogg;base64"
					fmt.Fprintf(&body, "<audio class=%q loop controls autobuffer src='%s,%s'></audio>", classSound, prefix, enc)
				}

				if len(v.Alt) != 0 {
					fmt.Fprintf(&body, "<figcaption class=%q>%s</figcaption>", classCaption, styledTextToHtml(index, page, v.Alt))
				}
				fmt.Fprint(&body, "</figure>")

			default:
				panic(fmt.Sprintf("unimplemented line kind: %T", v))
			}
		}

		tmp := template
		tmp = strings.ReplaceAll(tmp, "$site:title", siteTitle)
		tmp = strings.ReplaceAll(tmp, "$site:name", page.DisplayName)
		tmp = strings.ReplaceAll(tmp, "$site:style", styles)
		tmp = strings.ReplaceAll(tmp, "$site:nav", nav.String())
		tmp = strings.ReplaceAll(tmp, "$site:body", body.String())
		tmp = strings.ReplaceAll(tmp, "$site:edit", fmt.Sprintf("%s/edit/main/%s/%s", siteBase, inPath, page.LocalName))
		tmp = strings.ReplaceAll(tmp, "$site:created", fmt.Sprintf("%02d%02d%02d", y-2000, m, d))

		err = writeEntireFile(outPath, page.OutName, tmp)
		if err != nil {
			log.Printf(".. unable to create output file '%s%c%s': %s", outPath, filepath.Separator, page.OutName, err)
			continue
		}

		generated += 1
	}

	for _, p := range index.Pages {
		if p.UniqueName != "index" && p.Refs == 0 {
			log.Printf(".. orphaned page '%s'", p.LocalName)
		}
	}

	log.Printf(".. generated pages %d", generated)

	if *flagStartServer {
		address := fmt.Sprintf("127.0.0.1:%d", *flagPort)
		log.Printf(".. starting server at http://%s", address)

		httpDir := http.Dir(outPath)
		handler := http.FileServer(httpDir)
		if err := http.ListenAndServe(address, handler); err != nil {
			log.Fatal(err)
		}
	}
}

func listToHtml(index *Index, page *Page, lst List) string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "<ul class=%q>", classUl)

	for _, l := range lst {
		fmt.Fprintf(&b, "<li class=%q>%s", classLi, styledTextToHtml(index, page, l.Value))
		if len(l.Sublist) != 0 {
			fmt.Fprint(&b, listToHtml(index, page, l.Sublist))
		}
		fmt.Fprint(&b, "</li>")
	}

	fmt.Fprint(&b, "</ul>")
	return b.String()
}

func styledTextToHtml(index *Index, page *Page, text []StyledText) string {
	var b bytes.Buffer

	for _, t := range text {
		switch t.Style {
		case StyleNone:
			b.WriteString(t.Value)
		case StyleItalic:
			fmt.Fprintf(&b, "<em class=%q>%s</em>", classItalic, t.Value)
		case StyleBold:
			fmt.Fprintf(&b, "<strong class=%q>%s</strong>", classBold, t.Value)
		case StyleInternalLink:
			if p, ok := index.Pages[t.Link]; ok {
				display := t.Value
				if len(display) == 0 {
					display = p.DisplayName
				}

				fmt.Fprintf(&b, "<a class=%q href=%q>%s</a>", classIntLink, page.OutName, display)
			} else {
				log.Printf(".. '%s' has a broken internal link '%s'\n", page.LocalName, t.Link)

				display := t.Value
				if len(display) == 0 {
					display = t.Link
				}

				url := fmt.Sprintf("%s/new/main/%s?filename=%s.riv", siteBase, inPath, t.Link)
				fmt.Fprintf(&b, "<a class='broken %s' href=%q target='_blank'>%s</a>", classExtLink, url, display)
			}
		case StyleExternalLink:
			display := t.Value
			if len(display) == 0 {
				display = t.Link
			}

			fmt.Fprintf(&b, "<a class=%q href=%q target='_blank'>%s</a>", classExtLink, t.Link, display)
		default:
			panic(fmt.Sprintf("unimplemented style: %s (%v)", t.Value, t.Style))
		}
	}

	return b.String()
}

func init() {
	log.SetFlags(0)
	flag.Parse()
}

type (
	Index struct {
		Pages map[string]*Page
	}
	Page struct {
		Refs uint // how many pages reference this one
		Body Rivit
		Nav  []*Page

		UniqueName  string // path without extension
		DisplayName string // user-facing name
		LocalName   string // .riv filename
		OutName     string // .htm filename
	}
)

func readEntireFile(dir, filename string) (string, error) {
	var path string
	if len(dir) == 0 {
		path = filename
	} else {
		path = filepath.Join(dir, filename)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return string(b), nil
}

func writeEntireFile(dir, filename string, data string) error {
	var path string
	if len(dir) == 0 {
		path = filename
	} else {
		path = filepath.Join(dir, filename)
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}

	_, err = f.WriteString(data)
	if err != nil {
		return err
	}

	return f.Close()
}
