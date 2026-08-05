// Package enrich busca metadados que o arquivo não traz — gêneros, ISBN, sinopse,
// editora — consultando fontes externas através do calibre.
//
// Por que só no ingest e não em lote: cada consulta leva de 7 a 25 segundos. Para uma
// biblioteca de 11 mil livros isso passa de dez horas, mas para os poucos arquivos de um
// download é aceitável. É também por isso que a concorrência aqui é baixa por padrão —
// as fontes bloqueiam quem dispara consultas em rajada.
//
// A ferramenta usada é o fetch-ebook-metadata do calibre, e não uma API HTTP direta,
// por dois motivos medidos: a API pública do Google Books devolve
// "Quota exceeded" sem chave, e o Open Library não encontra títulos em português
// ("O Silmarillion" retorna zero resultados, enquanto "The Silmarillion" retorna 37).
// O fetch-ebook-metadata agrega várias fontes e devolveu dados corretos em PT-BR.
package enrich

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const (
	pythonBin = "/usr/bin/python3"
	fetchBin  = "/usr/bin/fetch-ebook-metadata"
)

// Result é o que a consulta externa conseguiu descobrir. Campos vazios significam que a
// fonte não sabia, e nesse caso o valor local é preservado.
type Result struct {
	Title     string
	Authors   []string
	Publisher string
	Published string
	ISBN      string
	Tags      []string
	Comments  string
	Found     bool
}

// Options controla a consulta.
type Options struct {
	Timeout      time.Duration
	TranslateTags bool // traduzir gêneros do inglês para português
}

// Available diz se a ferramenta existe nesta máquina.
func Available() bool {
	_, err := os.Stat(fetchBin)
	return err == nil
}

var (
	reField = regexp.MustCompile(`(?m)^([A-Za-z()\s]+?)\s*:\s*(.*)$`)
	reISBN  = regexp.MustCompile(`isbn:([0-9Xx]{10,17})`)
)

// Fetch consulta os metadados de um livro por título e autor.
func Fetch(ctx context.Context, title, author string, o Options) (Result, error) {
	var r Result
	if !Available() {
		return r, fmt.Errorf("%s não encontrado", fetchBin)
	}
	if o.Timeout == 0 {
		o.Timeout = 60 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()

	args := []string{fetchBin, "--title", title}
	if author != "" {
		args = append(args, "--authors", author)
	}
	cmd := exec.CommandContext(cctx, pythonBin, args...)
	// Ambiente mínimo pelo mesmo motivo do resto do projeto: o shebang é
	// "#!/usr/bin/env python3" e um Python de gerenciador de versão não tem o calibre.
	home, _ := os.UserHomeDir()
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + home, "LANG=C.UTF-8"}

	out, err := cmd.Output()
	if err != nil {
		if cctx.Err() != nil {
			return r, fmt.Errorf("timeout após %s", o.Timeout)
		}
		return r, fmt.Errorf("consulta falhou")
	}
	r = parseOutput(string(out))
	if o.TranslateTags {
		r.Tags = TranslateTags(r.Tags)
	}
	return r, nil
}

// parseOutput lê o formato "Campo : valor" que o fetch-ebook-metadata imprime.
func parseOutput(s string) Result {
	var r Result
	for _, m := range reField.FindAllStringSubmatch(s, -1) {
		key := strings.ToLower(strings.TrimSpace(m[1]))
		val := strings.TrimSpace(m[2])
		if val == "" {
			continue
		}
		switch {
		case strings.HasPrefix(key, "title"):
			r.Title, r.Found = val, true
		case strings.HasPrefix(key, "author"):
			for _, a := range strings.Split(val, "&") {
				if a = strings.TrimSpace(a); a != "" {
					r.Authors = append(r.Authors, a)
				}
			}
		case strings.HasPrefix(key, "publisher"):
			r.Publisher = val
		case strings.HasPrefix(key, "published"):
			r.Published = val
		case strings.HasPrefix(key, "tags"):
			for _, t := range strings.Split(val, ",") {
				if t = strings.TrimSpace(t); t != "" {
					r.Tags = append(r.Tags, t)
				}
			}
		case strings.HasPrefix(key, "identifiers"):
			if mm := reISBN.FindStringSubmatch(val); mm != nil {
				r.ISBN = mm[1]
			}
		case strings.HasPrefix(key, "comments"):
			r.Comments = val
		}
	}
	return r
}

// tagPT traduz os gêneros mais frequentes devolvidos pelas fontes, que respondem em
// inglês mesmo para livros em português. A lista cobre o que apareceu de fato numa
// biblioteca real; o que não estiver aqui passa sem tradução, o que é preferível a
// inventar um termo errado.
var tagPT = map[string]string{
	"fiction": "Ficção", "nonfiction": "Não-ficção", "non-fiction": "Não-ficção",
	"fantasy": "Fantasia", "science fiction": "Ficção Científica", "sci-fi": "Ficção Científica",
	"horror": "Terror", "thriller": "Suspense", "mystery": "Mistério", "crime": "Policial",
	"romance": "Romance", "historical": "Histórico", "history": "História",
	"biography": "Biografia", "autobiography": "Autobiografia", "memoir": "Memórias",
	"young adult": "Jovem Adulto", "juvenile fiction": "Infantojuvenil",
	"children": "Infantil", "classics": "Clássicos", "literary": "Literatura",
	"poetry": "Poesia", "drama": "Drama", "adventure": "Aventura", "epic": "Épico",
	"philosophy": "Filosofia", "psychology": "Psicologia", "religion": "Religião",
	"self-help": "Autoajuda", "business": "Negócios", "economics": "Economia",
	"politics": "Política", "science": "Ciência", "technology": "Tecnologia",
	"computers": "Computação", "health": "Saúde", "cooking": "Culinária",
	"travel": "Viagem", "art": "Arte", "music": "Música", "sports": "Esportes",
	"education": "Educação", "law": "Direito", "medical": "Medicina",
	"short stories": "Contos", "essays": "Ensaios", "war": "Guerra",
	"dystopian": "Distopia", "suspense": "Suspense", "erotica": "Erótico",
	"comics": "Quadrinhos", "graphic novels": "Graphic Novel", "humor": "Humor",
	"true crime": "Crime Real", "social science": "Ciências Sociais",
	"family": "Família", "detective": "Detetive", "action": "Ação",
	"paranormal": "Paranormal", "supernatural": "Sobrenatural",
}

// TranslateTags converte gêneros conhecidos para português, remove repetições e mantém
// a ordem de chegada.
func TranslateTags(tags []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range tags {
		key := strings.ToLower(strings.TrimSpace(t))
		v, ok := tagPT[key]
		if !ok {
			// Fontes devolvem hierarquias como "Fiction / Fantasy / Epic"; tenta cada
			// parte antes de desistir da tradução.
			for _, part := range strings.Split(key, "/") {
				if pv, pok := tagPT[strings.TrimSpace(part)]; pok {
					v, ok = pv, true
					break
				}
			}
		}
		if !ok {
			v = strings.TrimSpace(t)
		}
		if v != "" && !seen[strings.ToLower(v)] {
			seen[strings.ToLower(v)] = true
			out = append(out, v)
		}
	}
	return out
}

// MetaArgs monta os argumentos do ebook-meta apenas para o que o arquivo ainda não tem.
//
// O princípio é não sobrescrever dado local: o metadado que veio no arquivo foi posto
// pela editora ou pela conversão e costuma estar certo, enquanto a consulta externa é
// um palpite baseado em título e autor. Preenche lacuna, não corrige.
func (r Result) MetaArgs(hasTags, hasISBN, hasPublisher, hasComments bool) []string {
	var a []string
	if !hasTags && len(r.Tags) > 0 {
		a = append(a, "--tags", strings.Join(r.Tags, ","))
	}
	if !hasISBN && r.ISBN != "" {
		a = append(a, "--isbn", r.ISBN)
	}
	if !hasPublisher && r.Publisher != "" {
		a = append(a, "--publisher", r.Publisher)
	}
	if !hasComments && r.Comments != "" {
		a = append(a, "--comments", r.Comments)
	}
	return a
}
