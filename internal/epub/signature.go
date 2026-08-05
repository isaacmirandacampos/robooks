package epub

import (
	"archive/zip"
	"hash/fnv"
	"io"
	"regexp"
	"sort"

	"github.com/isaacdmcampos/kinava/internal/meta"
)

// Identificação de duplicata pelo conteúdo do livro.
//
// Nem o nome do arquivo nem os metadados resolvem este problema. Na biblioteca o mesmo
// livro aparece como "3 Grau - Clube das Mulheres Contra o Crime" e "3.º Grau", ou com
// um typo ("A Arte de oOuvir o Coração"), ou com subtítulo a mais ("A bibliotecária de
// Auschwitz - Um romance baseado numa história real"). Ao mesmo tempo, títulos quase
// idênticos podem ser livros diferentes: "Dom Quixote parte I" e "parte II" são 98%
// semelhantes no título, e "A Cidade do Sol" existe do Hosseini e do Campanella.
//
// Comparar o texto resolve os dois lados. O hash exato do texto não serve — cada
// conversão do calibre gera HTML um pouco diferente e nenhum par bateu. A medida usada
// aqui é a semelhança de Jaccard sobre shingles de 8 palavras, amostrados pelo hash do
// próprio shingle (não pela posição). Amostrar por conteúdo é o ponto essencial: um dos
// arquivos costuma ter um preâmbulo a mais, e qualquer amostragem posicional
// desalinharia os dois textos — foi o que me deu 0,4% de semelhança num par que na
// verdade é o mesmo livro.
//
// Medido nesta biblioteca: duplicatas ficam acima de 96%, livros distintos abaixo de 2%.

const (
	shingleWords = 8   // tamanho da janela em palavras
	sampleMod    = 128 // mantém ~1/128 dos shingles: assinatura pequena e ainda precisa
	// SimThreshold: medido nesta biblioteca, duplicatas ficam acima de 96% e livros
	// distintos abaixo de 2%, então 85% tem margem larga dos dois lados.
	SimThreshold = 0.85
)

var reHTMLDoc = regexp.MustCompile(`(?i)\.(x?html?)$`)
var reTagStrip = regexp.MustCompile(`<[^>]*>`)

// contentSignature devolve a assinatura amostrada do texto e a contagem de palavras.
func Signature(path string) ([]uint64, int) {
	z, err := zip.OpenReader(path)
	if err != nil {
		return nil, 0
	}
	defer z.Close()

	// Ordena as entradas para o texto sair na mesma ordem em execuções diferentes.
	names := make([]*zip.File, 0, len(z.File))
	for _, f := range z.File {
		if reHTMLDoc.MatchString(f.Name) {
			names = append(names, f)
		}
	}
	sort.Slice(names, func(i, j int) bool { return names[i].Name < names[j].Name })

	var words []string
	for _, f := range names {
		rc, err := f.Open()
		if err != nil {
			continue
		}
		b, err := io.ReadAll(io.LimitReader(rc, 8<<20))
		rc.Close()
		if err != nil {
			continue
		}
		words = append(words, meta.Tokenize(reTagStrip.ReplaceAllString(string(b), " "))...)
	}
	if len(words) < shingleWords+50 {
		return nil, len(words)
	}

	seen := make(map[uint64]struct{}, len(words)/sampleMod+16)
	h := fnv.New64a()
	for i := 0; i+shingleWords <= len(words); i++ {
		h.Reset()
		for j := i; j < i+shingleWords; j++ {
			h.Write([]byte(words[j]))
			h.Write([]byte{' '})
		}
		v := h.Sum64()
		if v%sampleMod == 0 {
			seen[v] = struct{}{}
		}
	}
	sig := make([]uint64, 0, len(seen))
	for v := range seen {
		sig = append(sig, v)
	}
	sort.Slice(sig, func(i, j int) bool { return sig[i] < sig[j] })
	return sig, len(words)
}

// jaccard mede a semelhança entre duas assinaturas ordenadas.
func Jaccard(a, b []uint64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	i, j, inter := 0, 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			inter++
			i++
			j++
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}
