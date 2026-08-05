package ingest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/isaacdmcampos/kinava/internal/epub"
	"github.com/isaacdmcampos/kinava/internal/index"
)

// Modos de tratamento de duplicata.
const (
	// DupBest mantém a melhor das duas cópias e descarta a outra. É o padrão: quando o
	// arquivo baixado é superior ao que já está na biblioteca, trocar é o resultado
	// desejado; quando é inferior, descartá-lo também é.
	DupBest = "best"
	// DupSkip deixa a biblioteca intocada e não move o arquivo de entrada.
	DupSkip = "skip"
	// DupQuarantine tira o arquivo baixado do caminho, sem tocar na biblioteca.
	DupQuarantine = "quarantine"
	// DupReplace troca sempre, mesmo que o novo seja pior. Existe para quando você sabe
	// algo que a heurística não sabe.
	DupReplace = "replace"
)

// candidate descreve uma das cópias em disputa.
type candidate struct {
	path   string
	words  int
	size   int64
	series string
	index  float64
}

// betterCopy decide se `a` deve substituir `b`.
//
// A ordem é série, depois texto, depois bytes — e não é arbitrária:
//
//   - Série primeiro porque é o que faz o Kavita agrupar o volume. Uma cópia com
//     calibre:series preenchido vale mais que uma maior sem série ("Mago Negro 02 - A
//     Aprendiz" contra "A Aprendiz" solto).
//   - Texto antes de bytes porque edições do mesmo livro divergem em conteúdo real:
//     "Nove Semanas e Meia de Amor" tem 44508 palavras contra 34531 da outra cópia.
//     Decidir por bytes (que sobem com imagens) descartaria a versão mais completa.
//   - Bytes por último, como desempate: mesmo texto e mesma série, o arquivo maior
//     costuma trazer imagens em melhor resolução.
func betterCopy(a, b candidate) bool {
	sa, sb := strings.TrimSpace(a.series) != "", strings.TrimSpace(b.series) != ""
	if sa != sb {
		return sa
	}
	if sa && sb {
		ia, ib := a.index > 0, b.index > 0
		if ia != ib {
			return ia
		}
	}
	if a.words > 0 && b.words > 0 {
		hi := a.words
		if b.words > hi {
			hi = b.words
		}
		// Diferença de texto só decide quando é relevante; abaixo de 2% é ruído de
		// formatação entre conversões do calibre.
		if absInt(a.words-b.words)*100/hi >= 2 {
			return a.words > b.words
		}
	}
	return a.size > b.size
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// describeChoice explica em uma linha por que uma cópia venceu, para o dry-run não
// pedir fé no resultado.
func describeChoice(newer, existing candidate) string {
	sn, se := strings.TrimSpace(newer.series) != "", strings.TrimSpace(existing.series) != ""
	switch {
	case sn != se:
		if sn {
			return "o novo tem série no metadado"
		}
		return "o da biblioteca tem série no metadado"
	case newer.words > 0 && existing.words > 0 && absInt(newer.words-existing.words)*100/maxInt(newer.words, existing.words) >= 2:
		if newer.words > existing.words {
			return fmt.Sprintf("o novo tem mais texto (%d vs %d palavras)", newer.words, existing.words)
		}
		return fmt.Sprintf("o da biblioteca tem mais texto (%d vs %d palavras)", existing.words, newer.words)
	case newer.size != existing.size:
		if newer.size > existing.size {
			return fmt.Sprintf("mesmo texto, novo é maior (%s vs %s)", human(newer.size), human(existing.size))
		}
		return fmt.Sprintf("mesmo texto, o da biblioteca é maior (%s vs %s)", human(existing.size), human(newer.size))
	}
	return "empate; mantém o da biblioteca"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func human(n int64) string {
	switch {
	case n > 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n > 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

// candidateFor monta o candidato a partir de um arquivo em disco.
func candidateFor(path string) candidate {
	c := candidate{path: path}
	if st, err := os.Stat(path); err == nil {
		c.size = st.Size()
	}
	m, _ := epub.ReadMeta(path)
	c.series, c.index = m.Series, m.SeriesIndex
	_, c.words = epub.Signature(path)
	return c
}

// candidateFromIndex evita reabrir o arquivo da biblioteca: o índice já sabe tudo o que
// a decisão precisa.
func candidateFromIndex(lib string, e *index.Entry) candidate {
	return candidate{
		path:   filepath.Join(lib, e.Path),
		words:  e.Words,
		size:   e.Size,
		series: e.Series,
	}
}
