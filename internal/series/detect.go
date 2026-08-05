// Package series descobre séries que o arquivo não declara, a partir dos títulos de um
// mesmo autor.
//
// O caso que motivou: Bernard Cornwell tem nove livros chamados "A Batalha de Sharpe",
// "A Espada de Sharpe", "A Fuga de Sharpe" e assim por diante. É a série Sharpe, e
// nenhum dos arquivos traz calibre:series. Agrupar por prefixo comum não resolve —
// a palavra que identifica a série está no fim do título, não no começo.
//
// A detecção é heurística e heurística erra. Uma tentativa anterior, baseada em prefixo,
// criou 299 séries inexistentes porque tratou pares de arquivos duplicados como volumes.
// Por isso aqui as salvaguardas vêm antes da cobertura: é melhor deixar de achar uma
// série do que inventar uma.
package series

import (
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/isaacmirandacampos/robooks/internal/meta"
)

// Book é a entrada mínima para a detecção.
type Book struct {
	Path   string
	Title  string
	Author string
}

// Candidate é uma série proposta.
type Candidate struct {
	Name    string
	Members []Book
	Score   int // em quantos títulos do autor o termo aparece
}

// stopwords são palavras que aparecem em qualquer título e não identificam série.
var stopwords = map[string]bool{
	"a": true, "o": true, "as": true, "os": true, "um": true, "uma": true, "uns": true,
	"umas": true, "de": true, "da": true, "do": true, "das": true, "dos": true,
	"e": true, "em": true, "no": true, "na": true, "nos": true, "nas": true,
	"por": true, "para": true, "com": true, "sem": true, "sob": true, "sobre": true,
	"ao": true, "aos": true, "à": true, "às": true, "que": true, "se": true, "ou": true,
	"the": true, "of": true, "and": true, "in": true, "to": true, "at": true,
	"seu": true, "sua": true, "seus": true, "suas": true, "meu": true, "minha": true,
	"este": true, "esta": true, "esse": true, "essa": true, "aquele": true,
}

// genericas são palavras de título que se repetem entre livros sem indicar série.
// Sem esta lista, "Contos" viraria série para todo autor de coletâneas.
var genericas = map[string]bool{
	"contos": true, "conto": true, "livro": true, "livros": true, "volume": true,
	"vol": true, "parte": true, "obras": true, "obra": true, "coleção": true,
	"colecao": true, "antologia": true, "completa": true, "completo": true,
	"história": true, "historia": true, "histórias": true, "historias": true,
	"novo": true, "nova": true, "grande": true, "grandes": true, "melhor": true,
	"melhores": true, "vida": true, "morte": true, "amor": true, "guerra": true,
	"tempo": true, "mundo": true, "homem": true, "mulher": true, "noite": true,
	"dia": true, "casa": true, "rei": true, "senhor": true, "filho": true, "filha": true,
	"segredo": true, "segredos": true, "mistério": true, "misterio": true,
	"crime": true, "assassinato": true, "caso": true, "última": true, "ultimo": true,
	"último": true, "primeira": true, "primeiro": true, "edição": true, "edicao": true,
	"selecionados": true, "escolhidos": true, "reunidos": true, "essencial": true,
	// Vistos na primeira execução real: agrupavam manuais e coletâneas que não são
	// série. "Clássicos" juntava seis Conan Doyle de coleções diferentes.
	"clássicos": true, "classicos": true, "clássico": true, "classico": true,
	"civil": true, "penal": true, "direito": true, "manual": true, "curso": true,
	"esquematizado": true, "comentado": true, "brasileiro": true, "brasileira": true,
	"português": true, "portugues": true, "literatura": true, "biblioteca": true,
	"trilogia": true, "saga": true, "série": true, "serie": true, "box": true,
}

// editoras aparecem no título de coleções ("Clássicos Zahar") e agrupariam livros que
// só compartilham a casa publicadora. Editora não é série.
var editoras = map[string]bool{
	"zahar": true, "record": true, "rocco": true, "intrinseca": true, "intrínseca": true,
	"companhia": true, "letras": true, "objetiva": true, "planeta": true, "globo": true,
	"sextante": true, "arqueiro": true, "aleph": true, "darkside": true, "suma": true,
	"leya": true, "ática": true, "atica": true, "scipione": true, "moderna": true,
	"saraiva": true, "atlas": true, "forense": true, "revan": true, "boitempo": true,
	"martins": true, "fontes": true, "penguin": true, "harper": true, "collins": true,
	"bertrand": true, "nova": true, "fronteira": true, "ediouro": true, "melhoramentos": true,
	"paulinas": true, "vozes": true, "unesp": true, "edusp": true, "autêntica": true,
	"autentica": true, "todavia": true, "fósforo": true, "fosforo": true,
}

var reWord = regexp.MustCompile(`[\p{L}\p{N}']+`)

// Detect procura séries entre os livros de um mesmo autor.
//
// minMembers controla o rigor: com 3, o termo precisa aparecer em três títulos
// diferentes para ser considerado nome de série.
// Detect procura séries; excluir lista nomes que não devem virar série, para vetar o
// que a heurística acertou na forma mas errou no conteúdo.
func DetectExcluding(books []Book, minMembers int, excluir map[string]bool) []Candidate {
	all := Detect(books, minMembers)
	if len(excluir) == 0 {
		return all
	}
	var out []Candidate
	for _, c := range all {
		if excluir[strings.ToLower(c.Name)] {
			continue
		}
		out = append(out, c)
	}
	return out
}

func Detect(books []Book, minMembers int) []Candidate {
	byAuthor := map[string][]Book{}
	for _, b := range books {
		a := meta.NormKey(b.Author)
		if a == "" {
			continue
		}
		byAuthor[a] = append(byAuthor[a], b)
	}

	var out []Candidate
	for _, group := range byAuthor {
		if len(group) < minMembers {
			continue
		}
		out = append(out, detectInAuthor(group, minMembers)...)
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i].Members) != len(out[j].Members) {
			return len(out[i].Members) > len(out[j].Members)
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func detectInAuthor(group []Book, minMembers int) []Candidate {
	// Conta em quantos títulos distintos cada termo aparece. Contar títulos, e não
	// ocorrências, evita que uma palavra repetida dentro do mesmo título infle o peso.
	inTitles := map[string]map[int]bool{}
	display := map[string]string{}

	for i, b := range group {
		seen := map[string]bool{}
		words := reWord.FindAllString(b.Title, -1)
		for pos, w := range words {
			key := meta.NormKey(w)
			if !termoValido(key, w, pos) {
				continue
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			if inTitles[key] == nil {
				inTitles[key] = map[int]bool{}
			}
			inTitles[key][i] = true
			// Guarda a grafia mais "natural" como nome de exibição: entre "Civil" e
			// "CIVIL" fica a de caixa mista, senão o mesmo termo vira duas séries.
			if cur, ok := display[key]; !ok || melhorGrafia(w, cur) {
				display[key] = w
			}
		}
	}

	var cands []Candidate
	usados := map[int]bool{}

	// Termos mais frequentes primeiro: se um livro casa com dois termos, fica com o
	// que descreve a série maior.
	type kv struct {
		key string
		n   int
	}
	var ranked []kv
	for k, set := range inTitles {
		ranked = append(ranked, kv{k, len(set)})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].n != ranked[j].n {
			return ranked[i].n > ranked[j].n
		}
		return ranked[i].key < ranked[j].key
	})

	for _, r := range ranked {
		if r.n < minMembers {
			continue
		}
		var members []Book
		for idx := range inTitles[r.key] {
			if usados[idx] {
				continue
			}
			members = append(members, group[idx])
		}
		if len(members) < minMembers {
			continue
		}
		// O termo não pode ser o título inteiro de um dos livros: aí ele é o livro, não
		// a série.
		ehTituloInteiro := false
		for _, m := range members {
			if meta.NormKey(m.Title) == r.key {
				ehTituloInteiro = true
				break
			}
		}
		if ehTituloInteiro {
			continue
		}
		for idx := range inTitles[r.key] {
			usados[idx] = true
		}
		sort.Slice(members, func(i, j int) bool { return members[i].Title < members[j].Title })
		cands = append(cands, Candidate{Name: display[r.key], Members: members, Score: r.n})
	}
	return cands
}

// melhorGrafia prefere caixa mista a tudo-maiúsculo, evitando "CIVIL" e "Civil" como
// séries distintas.
func melhorGrafia(novo, atual string) bool {
	shout := func(x string) bool {
		up, letras := 0, 0
		for _, r := range x {
			if unicode.IsLetter(r) {
				letras++
				if unicode.IsUpper(r) {
					up++
				}
			}
		}
		return letras > 1 && up == letras
	}
	return shout(atual) && !shout(novo)
}

// termoValido filtra o que não pode nomear uma série.
func termoValido(key, original string, pos int) bool {
	if len([]rune(key)) < 4 {
		return false // termos curtos demais casam por acaso
	}
	if stopwords[key] || genericas[key] || editoras[key] {
		return false
	}
	// Só dígitos: número de volume, não nome de série.
	if strings.IndexFunc(key, func(r rune) bool { return !unicode.IsDigit(r) }) < 0 {
		return false
	}
	// Nome próprio: precisa começar com maiúscula no título original. É o que separa
	// "Sharpe" de "batalha". A primeira palavra do título é ignorada porque sempre vem
	// capitalizada e não diz nada sobre ser nome próprio.
	if pos == 0 {
		return false
	}
	r := []rune(original)
	return unicode.IsUpper(r[0])
}
