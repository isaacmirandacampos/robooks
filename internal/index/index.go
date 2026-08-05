// Package index guarda a impressão digital dos livros já presentes na biblioteca.
//
// Existe por um motivo de custo: identificar duplicata exige a assinatura de conteúdo
// de cada livro, e calculá-la para toda a biblioteca leva mais de um minuto de parede
// com dez workers. Fazer isso a cada ingestão de dois ou três arquivos novos seria
// absurdo. O índice guarda as assinaturas em disco e só recalcula o que mudou —
// detectado por caminho, tamanho e data de modificação.
package index

import (
	"compress/gzip"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// formatVersion sobe quando o cálculo da assinatura muda de forma incompatível; o
// índice antigo é descartado em vez de produzir comparações silenciosamente erradas.
// Subiu para 2 quando o índice passou a guardar Tags: um índice da versão anterior
// não tem o campo, e usá-lo faria o ingest achar que a biblioteca não usa gênero
// nenhum, deixando passar qualquer rótulo.
const formatVersion = 2

// Entry é o que se sabe sobre um livro já indexado.
type Entry struct {
	Path    string // relativo à raiz da biblioteca
	Size    int64
	ModTime int64
	Words   int
	Sig     []uint64 // assinatura de conteúdo amostrada
	Title   string
	Author  string
	Series  string
	Tags    []string // gêneros já em uso, para o ingest não inventar vocabulário novo
}

// Index é o conjunto de livros conhecidos, endereçado pelo caminho relativo.
type Index struct {
	Version int
	Root    string
	Updated time.Time
	Entries map[string]*Entry
}

func New(root string) *Index {
	return &Index{Version: formatVersion, Root: root, Entries: map[string]*Entry{}}
}

// Load lê o índice do disco. Um índice ausente, corrompido ou de versão antiga devolve
// um índice vazio em vez de erro: reconstruir é sempre possível e é o comportamento
// menos surpreendente para quem só quer rodar o comando.
func Load(path, root string) (*Index, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return New(root), nil
		}
		return New(root), err
	}
	defer f.Close()

	zr, err := gzip.NewReader(f)
	if err != nil {
		return New(root), fmt.Errorf("índice ilegível, recriando: %w", err)
	}
	defer zr.Close()

	var idx Index
	if err := gob.NewDecoder(zr).Decode(&idx); err != nil {
		return New(root), fmt.Errorf("índice corrompido, recriando: %w", err)
	}
	if idx.Version != formatVersion || idx.Entries == nil {
		return New(root), nil
	}
	idx.Root = root
	return &idx, nil
}

// Save grava o índice de forma atômica: escreve num temporário e renomeia, para uma
// interrupção no meio nunca deixar um índice truncado no lugar do bom.
func (ix *Index) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	ix.Updated = time.Now()
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(f)
	if err := gob.NewEncoder(zw).Encode(ix); err != nil {
		zw.Close()
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := zw.Close(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// NeedsUpdate diz se o arquivo mudou desde a indexação.
func (ix *Index) NeedsUpdate(rel string, size, modTime int64) bool {
	e, ok := ix.Entries[rel]
	return !ok || e.Size != size || e.ModTime != modTime
}

func (ix *Index) Put(e *Entry) { ix.Entries[e.Path] = e }

// Prune remove do índice os caminhos que não existem mais, para que livros apagados
// deixem de bloquear a entrada de um arquivo igual no futuro.
func (ix *Index) Prune(exists map[string]bool) int {
	n := 0
	for p := range ix.Entries {
		if !exists[p] {
			delete(ix.Entries, p)
			n++
		}
	}
	return n
}

// Match é uma correspondência encontrada na biblioteca.
type Match struct {
	Entry      *Entry
	Similarity float64
}

// FindSimilar devolve os livros da biblioteca cuja assinatura passa do limiar,
// ordenados do mais parecido para o menos.
//
// A varredura é linear sobre o índice. Para dez mil entradas isso custa alguns
// milissegundos por consulta, o que é irrelevante diante do custo de abrir e ler o
// arquivo novo — não vale a complexidade de um índice invertido aqui.
func (ix *Index) FindSimilar(sig []uint64, threshold float64, jaccard func(a, b []uint64) float64) []Match {
	var out []Match
	for _, e := range ix.Entries {
		if len(e.Sig) == 0 {
			continue
		}
		if s := jaccard(sig, e.Sig); s >= threshold {
			out = append(out, Match{Entry: e, Similarity: s})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Similarity > out[j].Similarity })
	return out
}

// DefaultPath devolve o local padrão do índice para uma biblioteca.
func DefaultPath(root string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(root, ".robooks-index.gz")
	}
	// Fora da biblioteca de propósito: o Kavita varre a pasta e um arquivo estranho lá
	// dentro só geraria ruído no scan.
	return filepath.Join(home, ".cache", "robooks", "index.gz")
}

// GenreFreq conta quantos livros usam cada gênero. É o que permite ao ingest decidir se
// um gênero que chega num arquivo novo já faz parte do vocabulário da biblioteca ou é
// mais um rótulo solto.
func (ix *Index) GenreFreq() map[string]int {
	f := map[string]int{}
	for _, e := range ix.Entries {
		for _, t := range e.Tags {
			f[t]++
		}
	}
	return f
}
