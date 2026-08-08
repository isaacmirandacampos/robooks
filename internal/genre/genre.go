// Package genre limpa e canoniza os gêneros (dc:subject) que o Kavita usa como filtro.
//
// O campo chega poluído de várias origens ao mesmo tempo. Numa biblioteca real de 2763
// livros havia 3180 gêneros distintos — mais gêneros que livros —, com 80% do
// vocabulário aparecendo uma única vez. Um filtro com 3180 opções não filtra nada.
//
// O que polui, medido nessa biblioteca:
//
//	425 entradas  frases de sinopse capturadas como gênero (>45 caracteres)
//	219 entradas  genéricos vazios: "General", "ebook", "Geral"
//	196 entradas  idioma no lugar de gênero: "Portuguese", "Foreign Languages"
//	147 entradas  marca de site: "Exilado dos livros", "epubr.club"
//	 65 casos     mesmo gênero em caixas diferentes: "Romance" e "romance"
//	 25 entradas  hierarquia colada: "Literatura Estrangeira / Romance"
//	 10 entradas  códigos BISAC: "TRV009050", "1.2.2.0.0.1.0"
package genre

import (
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/isaacmirandacampos/robooks/internal/meta"
)

const maxGenreLen = 45

var (
	// Código BISAC ("TRV009050") ou sequência numérica de classificação.
	reCode = regexp.MustCompile(`^(?:[A-Z]{3}\d{6}|[\d.\s-]+)$`)
	// Marca de site, com ou sem domínio explícito.
	reSite = regexp.MustCompile(`(?i)\b[a-z0-9-]+\.(?:com|net|org|club|br|info|xyz|io)\b|` +
		`(?i)exilado|epubr|le ?livros|lelivros|libgen|z-?lib|semeadores|kingofmaine|iosbooks|` +
		`(?i)cr[ée]dito ao site|digitalizado por|revisado por|tradu[çc][ãa]o de`)
	// Idioma no lugar de gênero.
	//
	// O nome do idioma só é ruído quando está sozinho. Uma versão anterior deste padrão
	// não tinha a âncora final, para alcançar "Foreign Languages", e com isso engolia
	// "Portuguese Literature" — 226 livros que perdiam o único rótulo que os reunia.
	// As formas compostas são listadas à parte, para o prefixo não decidir sozinho.
	reLang = regexp.MustCompile(`(?i)^(portuguese|english|spanish|french|german|italian|` +
		`russian|japanese|chinese|latin|greek|idioma|l[íi]ngua)$|` +
		`(?i)^foreign lang\w*|(?i)\blanguage (materials?|study|arts)$`)
	// Uma palavra de verdade tem ao menos três letras seguidas.
	rePalavra = regexp.MustCompile(`\p{L}{3}`)
	// Numeral romano, para o Title Case não transformar "II" em "Ii".
	reRomano = regexp.MustCompile(`^(?i)[ivxlcdm]{1,6}$`)
	// Separadores de hierarquia que devem virar gêneros distintos.
	reSplit = regexp.MustCompile(`\s*[;/|]\s*|\s+>\s+|\s+&\s+|\s+e\s+(?:mist[ée]rio|suspense|aventura|fantasia|romance)\b`)
)

// vazios são rótulos que não dizem nada sobre o livro.
var vazios = map[string]bool{
	"general": true, "geral": true, "ebook": true, "e-book": true, "book": true,
	"books": true, "livro": true, "livros": true, "misc": true, "miscellaneous": true,
	"other": true, "outros": true, "diversos": true, "n/a": true, "none": true,
	"unknown": true, "desconhecido": true, "sem categoria": true, "literatura geral": true,
	"fiction general": true, "nonfiction general": true, "adult": true, "all": true,
	// Descobertos ao inspecionar o vocabulário real: marca de origem, resto de nome de
	// arquivo e rótulos de loja que entraram como gênero.
	"oficial": true, "comprado": true, "epub": true, "mobi": true, "pdf": true,
	"club": true, "group": true, "body": true, "success": true, "digital": true,
	"revisado": true, "traduzido": true, "completo": true, "colecao": true,
	"coleção": true, "serie": true, "série": true, "volume": true, "box": true,
	"nacional": true, "importado": true, "lancamento": true, "lançamento": true,
	"bestseller": true, "best seller": true, "novo": true, "usado": true,
}

// pessoas são nomes que aparecem como gênero em coleções mal etiquetadas — autores,
// médiuns e personagens. Não descrevem o assunto do livro e explodem o vocabulário do
// filtro: numa coleção espírita apareceram mais de vinte deles.
var pessoas = map[string]bool{
	"chico xavier": true, "francisco candido xavier": true, "francisco cândido xavier": true,
	"allan kardec": true, "hippolyte leon denizard rivail": true,
	"hippolyte léon denizard rivail": true, "emmanuel": true, "andre luiz": true,
	"andré luiz": true, "meimei": true, "neio lucio": true, "neio lúcio": true,
	"yvonne a. pereira": true, "publio lentulus": true, "humberto de campos": true,
	"jesus": true, "deus": true, "maria": true,
}

// canon unifica sinônimos e traduções para uma forma só. Sem isso, "Fiction" e "Ficção"
// viram dois filtros para a mesma coisa — e na biblioteca medida eram 146 e 135 livros
// que deveriam estar juntos.
var canon = map[string]string{
	"fiction": "Ficção", "ficcao": "Ficção", "ficção": "Ficção",
	"nonfiction": "Não-ficção", "non-fiction": "Não-ficção", "nao-ficcao": "Não-ficção",
	"fantasy": "Fantasia", "fantasia": "Fantasia",
	"science fiction": "Ficção Científica", "sci-fi": "Ficção Científica",
	"ficcao cientifica": "Ficção Científica", "ficção científica": "Ficção Científica",
	"ficção científica e fantasia": "Ficção Científica", "science fiction & fantasy": "Ficção Científica",
	"horror": "Terror", "terror": "Terror",
	"thriller": "Suspense", "suspense": "Suspense", "thrillers": "Suspense",
	"mystery": "Mistério", "misterio": "Mistério", "mistério": "Mistério",
	"crime": "Policial", "policial": "Policial", "true crime": "Crime Real",
	"detective": "Policial", "crime fiction": "Policial",
	"romance": "Romance", "romances": "Romance", "love stories": "Romance",
	"historical": "Histórico", "historical fiction": "Histórico",
	"history": "História", "historia": "História", "história": "História",
	"biography": "Biografia", "biografia": "Biografia", "biography & autobiography": "Biografia",
	"autobiography": "Autobiografia", "memoir": "Memórias", "memoirs": "Memórias",
	"young adult": "Jovem Adulto", "young adult fiction": "Jovem Adulto", "ya": "Jovem Adulto",
	"juvenile fiction": "Infantojuvenil", "infanto-juvenil": "Infantojuvenil",
	"infantojuvenil": "Infantojuvenil", "children": "Infantil", "children's": "Infantil",
	"juvenile nonfiction": "Infantojuvenil",
	"classics":            "Clássicos", "classicos": "Clássicos", "clássicos": "Clássicos",
	"literary": "Literatura", "literature": "Literatura", "literatura": "Literatura",
	"literature & fiction": "Literatura", "literary criticism": "Crítica Literária",
	"poetry": "Poesia", "poesia": "Poesia", "drama": "Drama", "teatro": "Teatro",
	"adventure": "Aventura", "aventura": "Aventura", "action": "Ação", "acao": "Ação",
	"epic": "Épico", "epico": "Épico",
	"philosophy": "Filosofia", "filosofia": "Filosofia",
	"psychology": "Psicologia", "psicologia": "Psicologia",
	"religion": "Religião", "religiao": "Religião", "religião": "Religião",
	"self-help": "Autoajuda", "autoajuda": "Autoajuda", "self help": "Autoajuda",
	"business": "Negócios", "negocios": "Negócios", "business & economics": "Negócios",
	"economics": "Economia", "economia": "Economia",
	"political science": "Política", "politics": "Política", "politica": "Política",
	"science": "Ciência", "ciencia": "Ciência", "ciência": "Ciência",
	"technology": "Tecnologia", "tecnologia": "Tecnologia",
	"computers": "Computação", "computing": "Computação",
	"health": "Saúde", "saude": "Saúde", "health & fitness": "Saúde",
	"cooking": "Culinária", "culinaria": "Culinária", "cookbooks": "Culinária",
	"travel": "Viagem", "viagem": "Viagem", "art": "Arte", "arte": "Arte",
	"music": "Música", "musica": "Música", "sports": "Esportes", "esportes": "Esportes",
	"education": "Educação", "educacao": "Educação", "law": "Direito", "direito": "Direito",
	"medical": "Medicina", "medicina": "Medicina",
	"short stories": "Contos", "contos": "Contos", "essays": "Ensaios", "ensaios": "Ensaios",
	"crônicas": "Crônicas", "cronicas": "Crônicas",
	"war": "Guerra", "guerra": "Guerra", "military": "Militar",
	"dystopian": "Distopia", "distopia": "Distopia", "utopian": "Utopia",
	"erotica": "Erótico", "erotico": "Erótico", "erótico": "Erótico",
	"comics": "Quadrinhos", "quadrinhos": "Quadrinhos", "graphic novels": "Graphic Novel",
	"manga": "Mangá", "humor": "Humor", "comedy": "Comédia",
	"social science": "Ciências Sociais", "sociology": "Sociologia", "sociologia": "Sociologia",
	"anthropology": "Antropologia", "family": "Família", "familia": "Família",
	"paranormal": "Paranormal", "supernatural": "Sobrenatural",
	"literatura brasileira":  "Literatura Brasileira",
	"literatura estrangeira": "Literatura Estrangeira",
	"romance histórico":      "Romance Histórico", "romance historico": "Romance Histórico",
	"contos e crônicas": "Contos", "espiritismo": "Espiritismo",
	"administração": "Administração", "administracao": "Administração",
	"gastronomia": "Gastronomia", "fotografia": "Fotografia", "cinema": "Cinema",
	// Variantes encontradas no acervo real que apareciam como filtros separados.
	"conto": "Contos", "auto-ajuda": "Autoajuda", "clássico": "Clássicos",
	"classico": "Clássicos", "poemas": "Poesia", "poema": "Poesia",
	"romantico": "Romance", "romântico": "Romance", "romance sobrenatural": "Sobrenatural",
	"ficção histórica": "Romance Histórico", "ficcao historica": "Romance Histórico",
	"juvenil": "Infantojuvenil", "chick lit": "Chick-lit", "chick-lit": "Chick-lit",
	"espiritualismo": "Espiritismo", "espírita": "Espiritismo", "espirita": "Espiritismo",
	"livro espírita": "Espiritismo", "doutrina espírita": "Espiritismo",
	"codificação espírita": "Espiritismo", "codificação": "Espiritismo",
	"espiritual": "Espiritualidade", "mind & spirit": "Espiritualidade",
	"detetive": "Policial", "assassinato": "Policial",
	"mystery & detective": "Mistério", "thriller & mystery": "Suspense",
	"short stories (single author": "Contos", "other languages": "",
	"personal memoirs": "Memórias", "biographies & memoirs": "Biografia",
	"personal growth": "Autoajuda", "personal development & self-help": "Autoajuda",
	"motivational & inspirational": "Autoajuda",
	"politics & social sciences":   "Política", "religion & spirituality": "Religião",
	"latin america": "", "south america": "", "portugal": "", "brasil": "",
	"contemporary": "", "ebook company": "", "by anjo_high_tech": "",
	// Região e recorte demográfico não descrevem o assunto e poluem o filtro.
	"european": "", "american": "", "africa": "", "asia": "", "europe": "",
	"united states": "", "england": "", "france": "", "germany": "", "women": "",
	"men": "", "general interest": "", "world": "", "international": "",
}

// Clean recebe os gêneros de um livro e devolve a versão utilizável: ruído removido,
// hierarquias abertas, sinônimos unificados e duplicatas eliminadas.
func Clean(tags []string) []string {
	seen := map[string]bool{}
	var out []string

	for _, raw := range tags {
		// Hierarquias viram gêneros independentes: "Policial / Suspense" é mais útil
		// como dois filtros do que como um rótulo que ninguém vai escolher.
		for _, part := range reSplit.Split(raw, -1) {
			t := strings.TrimSpace(part)
			t = strings.Trim(t, ".,;:-–—\"'()[]")
			t = meta.Collapse(t)
			if !usable(t) {
				continue
			}
			c := canonical(t)
			if c == "" {
				continue // mapeado explicitamente para descarte
			}
			k := strings.ToLower(c)
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, c)
		}
	}
	sort.Strings(out)
	return out
}

// usable decide se o texto pode ser um gênero.
func usable(t string) bool {
	if len([]rune(t)) < 3 || len([]rune(t)) > maxGenreLen {
		return false // curto demais para significar algo, ou é frase de sinopse
	}
	low := strings.ToLower(t)
	if vazios[low] || pessoas[low] || reCode.MatchString(t) || reSite.MatchString(t) || reLang.MatchString(t) {
		return false
	}
	// Precisa conter uma palavra de verdade: pelo menos três letras seguidas. Sem
	// isso, emoticons e enfeites passam — "o(O_O)o" tem letras e comprimento válido,
	// mas não é gênero.
	if !rePalavra.MatchString(t) {
		return false
	}
	// Símbolos demais denunciam enfeite, não rótulo.
	simbolos := 0
	for _, r := range t {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsSpace(r) && r != '-' && r != '\'' {
			simbolos++
		}
	}
	if simbolos*3 > len([]rune(t)) {
		return false
	}
	// Frase, não rótulo: gênero não costuma passar de quatro palavras.
	if len(strings.Fields(t)) > 4 {
		return false
	}
	return true
}

// canonical unifica sinônimos e padroniza a caixa.
func canonical(t string) string {
	low := strings.ToLower(strings.TrimSpace(t))
	if c, ok := canon[low]; ok {
		return c // string vazia significa "descartar"; o chamador filtra
	}
	// Sem entrada no mapa, ao menos padroniza a caixa para "Romance" e "romance" não
	// virarem dois filtros.
	return titleCase(t)
}

var minusculas = map[string]bool{
	"de": true, "da": true, "do": true, "das": true, "dos": true, "e": true,
	"em": true, "a": true, "o": true, "as": true, "os": true, "para": true,
	"com": true, "sem": true, "no": true, "na": true, "and": true, "of": true, "the": true,
}

func titleCase(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		if i > 0 && minusculas[w] {
			continue
		}
		// "World War II" não pode virar "World War Ii".
		if reRomano.MatchString(w) && w != "i" && w != "c" && w != "d" && w != "m" && w != "l" {
			words[i] = strings.ToUpper(w)
			continue
		}
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

// Stats resume o efeito da limpeza sobre um conjunto de gêneros.
type Stats struct {
	Before, After                 int
	DistinctBefore, DistinctAfter int
}

// Analyze aplica a limpeza a um vocabulário inteiro e devolve o antes e depois, para
// que o efeito possa ser conferido antes de escrever em milhares de arquivos.
func Analyze(all map[string]int) (Stats, map[string]int) {
	st := Stats{DistinctBefore: len(all)}
	after := map[string]int{}
	for t, n := range all {
		st.Before += n
		for _, c := range Clean([]string{t}) {
			after[c] += n
			st.After += n
		}
	}
	st.DistinctAfter = len(after)
	return st, after
}

// Vocabulary é o conjunto de gêneros que sobrevive ao corte de frequência.
//
// Limpar o texto de cada rótulo não basta para o filtro ficar usável: mesmo depois da
// limpeza, o acervo medido tinha 2150 gêneros distintos, três em cada quatro deles com
// um único livro. Uma lista suspensa com 2150 opções não filtra nada. O corte por
// frequência mínima é o que transforma isso num conjunto navegável.
type Vocabulary struct {
	Freq map[string]int
	Min  int
}

// BuildVocabulary conta a frequência de cada gênero já limpo em toda a biblioteca.
// Exige uma passada completa antes de escrever, porque a decisão de manter um gênero
// depende de quantos livros o usam — não dá para decidir olhando um livro só.
func BuildVocabulary(perBook [][]string, min int) *Vocabulary {
	f := map[string]int{}
	for _, gs := range perBook {
		for _, g := range gs {
			f[g]++
		}
	}
	return &Vocabulary{Freq: f, Min: min}
}

// Keep diz se o gênero é frequente o bastante para valer um item no filtro.
func (v *Vocabulary) Keep(g string) bool {
	if v == nil || v.Min <= 1 {
		return true
	}
	return v.Freq[g] >= v.Min
}

// Apply limpa e depois corta pelo vocabulário.
func (v *Vocabulary) Apply(tags []string) []string {
	var out []string
	for _, g := range Clean(tags) {
		if v.Keep(g) {
			out = append(out, g)
		}
	}
	return out
}

// Size devolve quantos gêneros sobrevivem ao corte.
func (v *Vocabulary) Size() int {
	if v == nil {
		return 0
	}
	n := 0
	for g := range v.Freq {
		if v.Keep(g) {
			n++
		}
	}
	return n
}

// Top lista os gêneros mantidos, do mais comum para o menos.
func (v *Vocabulary) Top(n int) []string {
	type kv struct {
		k string
		v int
	}
	var all []kv
	for g, c := range v.Freq {
		if v.Keep(g) {
			all = append(all, kv{g, c})
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].v != all[j].v {
			return all[i].v > all[j].v
		}
		return all[i].k < all[j].k
	})
	var out []string
	for i, x := range all {
		if n > 0 && i >= n {
			break
		}
		out = append(out, x.k)
	}
	return out
}

// Admit decide quais gêneros de um livro que está entrando podem ficar.
//
// A regra tem duas portas porque um filtro só se mantém útil se for difícil poluí-lo,
// sem por isso congelar o vocabulário:
//
//   - passa o que a biblioteca já usa o bastante (freq >= min). Um livro novo marcado
//     como "Romance" reforça um filtro que já existe.
//   - passa o que é canônico conhecido, mesmo inédito na biblioteca. O primeiro livro
//     de "Culinária" precisa poder inaugurar o gênero.
//
// O resto fica de fora. Sem isso, cada arquivo baixado reintroduz "General",
// "epubr.club" e frases de sinopse, e o trabalho de limpeza se desfaz sozinho.
func Admit(tags []string, freq map[string]int, min int) (aceitos, recusados []string) {
	for _, g := range Clean(tags) {
		switch {
		case min <= 1, freq[g] >= min, IsCanonical(g):
			aceitos = append(aceitos, g)
		default:
			recusados = append(recusados, g)
		}
	}
	return aceitos, recusados
}

// IsCanonical diz se o gênero pertence ao vocabulário conhecido — o conjunto de valores
// para os quais o mapa de sinônimos aponta.
func IsCanonical(g string) bool {
	if canonSet == nil {
		canonSet = map[string]bool{}
		for _, v := range canon {
			if v != "" {
				canonSet[strings.ToLower(v)] = true
			}
		}
	}
	return canonSet[strings.ToLower(g)]
}

var canonSet map[string]bool
