package epub

import (
	"archive/zip"
	"errors"
	"github.com/isaacdmcampos/kinava/internal/meta"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// ErrNoOPF indica um epub sem arquivo de metadados.
var ErrNoOPF = errors.New("epub sem arquivo .opf")

// meta é o subconjunto do OPF que interessa: o que o Kavita lê e o que podemos
// melhorar. Lido com regex em vez de encoding/xml porque vários destes OPFs vêm de
// conversão de MOBI e não são XML estritamente bem-formado.
type Meta struct {
	Title       string
	Author      string
	AuthorSort  string
	Series      string
	SeriesIndex float64
	HasSeries   bool
}

var (
	reTitle  = regexp.MustCompile(`(?s)<dc:title[^>]*>(.*?)</dc:title>`)
	reAuthor = regexp.MustCompile(`(?s)<dc:creator([^>]*)>(.*?)</dc:creator>`)
	reFileAs = regexp.MustCompile(`(?:opf:)?file-as\s*=\s*"([^"]*)"`)
	reRole   = regexp.MustCompile(`(?:opf:)?role\s*=\s*"([^"]*)"`)
	reSeries = regexp.MustCompile(`<meta\s+name\s*=\s*"calibre:series"\s+content\s*=\s*"([^"]*)"`)
	reSerIdx = regexp.MustCompile(`<meta\s+name\s*=\s*"calibre:series_index"\s+content\s*=\s*"([^"]*)"`)
	reOpfIn  = regexp.MustCompile(`(?i)\.opf$`)
)

// readMeta abre o epub e extrai os campos do OPF. Só lê o OPF, não o livro todo,
// então custa pouco mesmo em 11 mil arquivos.
func ReadMeta(path string) (Meta, error) {
	var m Meta
	z, err := zip.OpenReader(path)
	if err != nil {
		return m, err
	}
	defer z.Close()

	var opf *zip.File
	for _, f := range z.File {
		if reOpfIn.MatchString(f.Name) {
			opf = f
			break
		}
	}
	if opf == nil {
		return m, ErrNoOPF
	}
	rc, err := opf.Open()
	if err != nil {
		return m, err
	}
	defer rc.Close()
	buf := make([]byte, 96*1024) // o bloco de metadados fica no começo do OPF
	n, _ := rc.Read(buf)
	s := string(buf[:n])

	if x := reTitle.FindStringSubmatch(s); x != nil {
		m.Title = meta.Collapse(html.UnescapeString(strings.TrimSpace(x[1])))
	}
	// Pega o primeiro creator com role de autor, ou o primeiro sem role.
	for _, x := range reAuthor.FindAllStringSubmatch(s, -1) {
		attrs, val := x[1], meta.Collapse(html.UnescapeString(strings.TrimSpace(x[2])))
		if val == "" {
			continue
		}
		role := ""
		if r := reRole.FindStringSubmatch(attrs); r != nil {
			role = strings.ToLower(r[1])
		}
		if m.Author == "" || role == "aut" {
			m.Author = val
			if fa := reFileAs.FindStringSubmatch(attrs); fa != nil {
				m.AuthorSort = meta.Collapse(html.UnescapeString(fa[1]))
			}
			if role == "aut" {
				break
			}
		}
	}
	if x := reSeries.FindStringSubmatch(s); x != nil {
		m.Series = meta.Collapse(html.UnescapeString(x[1]))
		m.HasSeries = m.Series != ""
	}
	if x := reSerIdx.FindStringSubmatch(s); x != nil {
		m.SeriesIndex, _ = strconv.ParseFloat(x[1], 64)
	}
	return m, nil
}
