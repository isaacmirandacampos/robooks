// Package target isola o que cada servidor de leitura espera do disco.
//
// A separação existe porque as convenções divergem de verdade. O Kavita exige que
// nenhum arquivo fique na raiz da biblioteca e agrupa séries por calibre:series no OPF,
// ignorando os nomes de pasta para epub. O Calibre organiza por Autor/Título e trata a
// pasta como identidade do livro. Escrever o layout errado não é questão de gosto: com
// os arquivos soltos na raiz, o Kavita cai no modo "series scan" e simplesmente para de
// indexar a biblioteca.
package target

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/isaacmirandacampos/robooks/internal/epub"
	"github.com/isaacmirandacampos/robooks/internal/meta"
)

// Placement é onde um livro deve ficar e com que metadados.
type Placement struct {
	Dir      string   // subdiretório relativo à raiz da biblioteca ("" = raiz)
	Filename string   // nome final do arquivo
	MetaArgs []string // argumentos para o ebook-meta, vazios se nada muda
}

// Target descreve as convenções de um servidor de leitura.
type Target interface {
	Name() string
	// Place decide caminho e metadados finais a partir do que se sabe do livro.
	Place(b meta.Book, m epub.Meta) Placement
	// Describe explica em uma linha a convenção aplicada, para o dry-run.
	Describe() string
}

// Registry lista os alvos disponíveis por nome.
func Registry() map[string]Target {
	return map[string]Target{
		"kavita":   Kavita{},
		"grimmory": Grimmory{},
		"calibre":  Calibre{},
	}
}

// Names devolve os alvos suportados, para mensagens de ajuda.
func Names() []string {
	var n []string
	for k := range Registry() {
		n = append(n, k)
	}
	return n
}

// Get resolve um alvo pelo nome.
func Get(name string) (Target, error) {
	t, ok := Registry()[strings.ToLower(name)]
	if !ok {
		return nil, fmt.Errorf("alvo desconhecido %q (disponíveis: %s)", name, strings.Join(Names(), ", "))
	}
	return t, nil
}

// metaArgsFor monta os argumentos do ebook-meta apenas para os campos que mudam, para
// não reescrever epub à toa.
func metaArgsFor(want, cur epub.Meta) []string {
	var a []string
	if want.Title != "" && want.Title != cur.Title {
		a = append(a, "--title", want.Title)
	}
	if want.Series != "" && want.Series != cur.Series {
		a = append(a, "--series", want.Series)
	}
	if want.SeriesIndex > 0 && want.SeriesIndex != cur.SeriesIndex {
		a = append(a, "--index", fmt.Sprintf("%g", want.SeriesIndex))
	}
	if want.AuthorSort != "" && want.AuthorSort != cur.AuthorSort {
		a = append(a, "--author-sort", want.AuthorSort)
	}
	return a
}

// desiredMeta calcula os metadados ideais a partir do nome interpretado e do que o
// arquivo já traz. Vale para qualquer alvo: título limpo e acentuado, série do nome do
// arquivo, ordenação de autor em "Sobrenome, Nome".
func desiredMeta(b meta.Book, cur epub.Meta) epub.Meta {
	want := cur

	// O título interno costuma ser melhor que o do nome do arquivo porque preserva
	// acentuação, mas em muitos arquivos ele é o próprio nome poluído — então passa
	// pelas mesmas limpezas, sem extração de autor (um " - " no título não separa
	// autor: "666 - O Limiar do Inferno" viraria só "666").
	cand := cur.Title
	if cand == "" {
		cand = b.Title
	}
	cand = meta.ParseTitleOnly(cand).Title
	if cand == "" {
		cand = b.Title
	}
	cand = meta.RestoreColon(cand)
	if meta.IsShouty(cand) {
		cand = meta.TitleCasePT(cand)
	}
	want.Title = meta.Collapse(cand)

	if b.Series != "" && cur.Series == "" {
		want.Series = b.Series
		if b.HasIdx {
			want.SeriesIndex = b.Index
		}
	}

	author := cur.Author
	if author == "" {
		author = b.Author
	}
	if cur.AuthorSort == "" || strings.EqualFold(cur.AuthorSort, "unknown") {
		if s := meta.AuthorFileAs(author); s != "" {
			want.AuthorSort = s
		}
	}
	return want
}

// safeDir limpa um nome para uso como diretório em ext4 e em compartilhamento Windows.
func safeDir(s string) string {
	s = meta.Sanitize(meta.DeaccentStr(s))
	s = strings.Trim(s, " .-_")
	if len(s) > 120 {
		s = strings.Trim(strings.TrimSpace(s[:120]), " .-_")
	}
	return s
}

var _ = filepath.Join
