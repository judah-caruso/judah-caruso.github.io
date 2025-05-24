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
	"strings"
	"time"

	"github.com/judah-caruso/rivit"
)

const (
	inPath  = "riv"
	resPath = "res"
	outPath = "web"

	siteBase         = "https://github.com/judah-caruso/judah-caruso.github.io"
	siteTitle        = "beton brutalism"
	siteStylePath    = "style.css"    // within resPath
	siteTemplatePath = "template.htm" // within resPath
	rssTemplatePath  = "rss.htm"      // within resPath
)

var (
	siteYear        = time.Now().Year()
	flagPort        = flag.Int("port", 8000, "sets the port for the HTTP server")
	flagStartServer = flag.Bool("server", false, "start a simple HTTP server after generation")
)

func main() {
	var (
		err       error
		styles    string
		template  string
		generated int
	)

	log.SetFlags(0)
	flag.Parse()

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
	}

	err = os.Mkdir(outPath, fs.ModeDir)
	if err != nil && !os.IsExist(err) {
		log.Printf("unable to create output directory '%s': %s\n", outPath, err)
		os.Exit(4)
	}

	// Pre-pass pages to make sure everything we expect is filled out
	for _, page := range index.Pages {
		info, err := os.Stat(filepath.Join(inPath, page.LocalName))
		if err == nil {
			page.Updated = info.ModTime()
		}

		src, err := readEntireFile(inPath, page.LocalName)
		if err != nil {
			log.Printf("unable to open page: '%s%c%s'\n", inPath, filepath.Separator, page.LocalName)
			continue
		}

		page.Body = rivit.Parse(src)
		for _, riv := range page.Body {
			if riv.Kind() != rivit.LineNavLink {
				continue
			}

			name := string(riv.(rivit.NavLink))
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
				log.Printf(".. page '%s' has a broken internal link '%s'\n", page.LocalName, name)
			}
		}

		for _, riv := range page.Body {
			if riv.Kind() == rivit.LineNavLink {
				continue
			}

			if riv.Kind() != rivit.LineHeader {
				break
			}

			h := riv.(rivit.Header)
			t := strings.Title(strings.ToLower(string(h)))
			if t != page.DisplayName {
				page.Title = t
				log.Printf(".. page '%s' has title '%s', was '%s'\n", page.LocalName, page.Title, page.DisplayName)
			}

			break
		}

		if page.DisplayName == "Index" {
			page.Title = "Home"
		}
	}

	// Page generation pass
	for _, page := range index.Pages {
		var (
			nav  bytes.Buffer
			body bytes.Buffer
		)

		nav.WriteString("<ul>")

		for _, nl := range page.Nav {
			fmt.Fprintf(&nav,
				"<li><a href=%q>%s</a></li>",
				nl.OutName, nl.DisplayName,
			)
		}

		nav.WriteString("</ul>")

		for _, line := range page.Body {
			if line.Kind() == rivit.LineNavLink {
				continue
			}

			switch v := line.(type) {
			case rivit.Paragraph:
				if len(v) == 1 && v[0].Value == ". . ." {
					fmt.Fprintf(&body, "<hr/>")
				} else {
					fmt.Fprintf(&body, "<p>%s</p>", styledTextToHtml(index, page, v))
				}

			case rivit.List:
				fmt.Fprint(&body, listToHtml(index, page, v))

			case rivit.Header:
				if body.Len() == 0 {
					fmt.Fprintf(&body, "<h1>%s</h1>", v)
				} else {
					fmt.Fprintf(&body, "<h2>%s</h2>", v)
				}

			case rivit.Block:
				body.WriteString("<pre>")

				for i, line := range v.Body {
					l := line[v.Indent:]
					l = strings.ReplaceAll(l, "<", "&lt;")
					l = strings.ReplaceAll(l, ">", "&gt;")

					body.WriteString(l)

					if i < len(v.Body) {
						body.WriteString("\n")
					}
				}

				body.WriteString("</pre>")

			case rivit.Embed:
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
					continue
				}

				file, err := readEntireFile(resPath, v.Path)
				if err != nil {
					log.Printf(".. unable to open embed '%s' within '%s'", v.Path, page.LocalName)
					continue
				}

				enc := base64.StdEncoding.EncodeToString([]byte(file))

				body.WriteString("<figure>")

				switch kind {
				case MediaPng:
					const prefix = "data:image/png;base64"
					fmt.Fprintf(&body, "<img src='%s,%s'/>", prefix, enc)
				case MediaSvg:
					const prefix = "data:image/svg+xml;base64"
					fmt.Fprintf(&body, "<img src='%s,%s'/>", prefix, enc)
				case MediaOgg:
					const prefix = "data:audio/ogg;base64"
					fmt.Fprintf(&body, "<audio loop controls autobuffer src='%s,%s'></audio>", prefix, enc)
				}

				if len(v.Alt) != 0 {
					fmt.Fprintf(&body, "<figcaption>%s</figcaption>", styledTextToHtml(index, page, v.Alt))
				}

				body.WriteString("</figure>")

			default:
				panic(fmt.Sprintf("unimplemented line kind: %T", v))
			}
		}

		y, m, d := page.Updated.Date()

		name := page.DisplayName
		if len(page.Title) > 0 {
			name = page.Title
		}

		r := strings.NewReplacer(
			"$site:title", siteTitle,
			"$site:name", name,
			"$site:style", styles,
			"$site:nav", nav.String(),
			"$site:body", body.String(),
			"$site:link", page.OutName,
			"$site:edit", fmt.Sprintf("%s/edit/main/%s/%s", siteBase, inPath, page.LocalName),
			"$site:updated", fmt.Sprintf("%02d%02d%02d", y-2000, m, d),
			"$site:year", fmt.Sprintf("%d", siteYear),
		)

		page.Final = r.Replace(template)
		err = writeEntireFile(outPath, page.OutName, page.Final)
		if err != nil {
			log.Printf(".. unable to create output file '%s%c%s': %s", outPath, filepath.Separator, page.OutName, err)
			continue
		}

		generated += 1
	}

	for _, p := range index.Pages {
		if p.UniqueName != "index" && p.Refs == 0 {
			log.Printf(".. page '%s' is orphaned", p.LocalName)
		}
	}

	// Generate RSS feed
	{
		template, err = readEntireFile(resPath, rssTemplatePath)
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

	{
		var body strings.Builder
		for _, p := range index.Pages {
			body.WriteString("<item>\n")
			fmt.Fprintf(&body, "\t<title>%s</title>\n", p.DisplayName)
			fmt.Fprintf(&body, "\t<link>https://judahcaruso.com/%s</link>\n", p.OutName)
			fmt.Fprintf(&body, "\t<pubDate>%s</pubDate>\n", p.Updated.Format(time.RFC1123))
			fmt.Fprintf(&body, "\t<description><![CDATA[%s]]</description>\n", p.Final)
			body.WriteString("</item>\n")
		}

		r := strings.NewReplacer(
			"$site:title", siteTitle,
			"$site:name", siteTitle,
			"$site:updated", time.Now().Format(time.RFC1123),
			"$site:posts", body.String(),
		)

		err = writeEntireFile(outPath, "rss.xml", r.Replace(template))
		if err != nil {
			log.Printf(".. unable to create output file '%s%crss.xml': %s", outPath, filepath.Separator, err)
			os.Exit(3)
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

func listToHtml(index *Index, page *Page, lst rivit.List) string {
	var b bytes.Buffer

	b.WriteString("<ul>")

	for _, l := range lst {
		fmt.Fprintf(&b, "<li>%s", styledTextToHtml(index, page, l.Value))
		if len(l.Sublist) != 0 {
			fmt.Fprint(&b, listToHtml(index, page, l.Sublist))
		}
		fmt.Fprint(&b, "</li>")
	}

	b.WriteString("</ul>")

	return b.String()
}

func styledTextToHtml(index *Index, page *Page, text []rivit.StyledText) string {
	var b bytes.Buffer

	for _, t := range text {
		switch t.Style {
		case rivit.StyleNone:
			b.WriteString(t.Value)

		case rivit.StyleItalic:
			fmt.Fprintf(&b, "<em>%s</em>", t.Value)

		case rivit.StyleBold:
			fmt.Fprintf(&b, "<strong>%s</strong>", t.Value)

		case rivit.StyleMono:
			fmt.Fprintf(&b, "<code>%s</code>", t.Value)

		case rivit.StyleInternalLink:
			if p, ok := index.Pages[t.Link]; ok {
				display := t.Value
				if len(display) == 0 {
					display = p.DisplayName
					if len(p.Title) > 0 {
						display = p.Title
					}
				}

				p.Refs += 1
				fmt.Fprintf(&b, "<a href=%q>%s</a>", p.OutName, display)
			} else {
				log.Printf(".. '%s' has a broken internal link '%s'\n", page.LocalName, t.Link)

				display := t.Value
				if len(display) == 0 {
					display = t.Link
				}

				url := fmt.Sprintf("%s/new/main/%s?filename=%s.riv", siteBase, inPath, t.Link)
				fmt.Fprintf(&b, "<a class='broken' href=%q target='_blank'>%s</a>", url, display)
			}

		case rivit.StyleExternalLink:
			display := t.Value
			if len(display) == 0 {
				display = t.Link
			}

			fmt.Fprintf(&b, "<a href=%q target='_blank'>%s</a>", t.Link, display)

		default:
			panic(fmt.Sprintf("unimplemented style: %s (%v)", t.Value, t.Style))
		}
	}

	return b.String()
}

type (
	Index struct {
		Pages map[string]*Page
	}
	Page struct {
		Refs    uint // how many pages reference this one
		Body    rivit.Rivit
		Nav     []*Page
		Updated time.Time
		Final   string

		UniqueName  string // path without extension
		Title       string // First title line or empty if none
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
