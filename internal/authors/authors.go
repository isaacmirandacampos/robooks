// Package authors encontra registros de autor que são a mesma pessoa escrita de
// formas diferentes.
//
// O problema aparece quando a biblioteca cresce por importação: cada epub traz o autor
// como o editor daquele arquivo escreveu, e o servidor cria um registro novo para cada
// grafia. Numa biblioteca de onze mil livros isso rendeu 9397 autores, muitos deles
// repetidos — "FÁBIO ULHOA COELHO" e "Fábio Ulhoa Coelho", "Assis, Machado de" e
// "Machado de Assis", "Tom Perrota" e "Tom Perrotta".
//
// A interface de merge do servidor resolve o merge, mas não a busca: para unificar é
// preciso já saber que o par existe. É essa busca que este pacote faz.
//
// A entrada é deliberadamente pobre — id, número de livros e nome — porque é o que
// qualquer catálogo consegue exportar. O pacote não conhece banco nem servidor.
package authors

import (
	"sort"
	"strings"
	"unicode"

	"github.com/isaacmirandacampos/robooks/internal/meta"
)

// Record é um registro de autor no catálogo.
type Record struct {
	ID    string
	Books int
	Name  string
}

// Rule identifica por que dois registros foram considerados a mesma pessoa. Serve para
// o relatório: as regras têm confiabilidades diferentes e merecem revisão diferente.
type Rule string

const (
	// RuleGrafia — os nomes só diferem em acento, caixa ou pontuação.
	RuleGrafia Rule = "grafia"
	// RuleInvertido — mesmas palavras em outra ordem, tipicamente "Sobrenome, Nome".
	RuleInvertido Rule = "invertido"
	// RuleIniciais — um dos nomes abrevia ou omite nomes do meio.
	RuleIniciais Rule = "iniciais"
	// RuleTypo — uma ou duas letras de diferença. É a única regra que erra sozinha.
	RuleTypo Rule = "typo"
)

// Confiavel diz se a regra pode ser aplicada sem revisão humana.
func (r Rule) Confiavel() bool { return r != RuleTypo }

// Group é um conjunto de registros que representam a mesma pessoa.
type Group struct {
	Canonical Record   // o registro que deve sobreviver ao merge
	Others    []Record // os que devem ser absorvidos
	Rules     []Rule   // por que foram agrupados
	Books     int      // soma de livros no grupo
}

// Confiavel diz se todas as regras que formaram o grupo dispensam revisão.
func (g Group) Confiavel() bool {
	for _, r := range g.Rules {
		if !r.Confiavel() {
			return false
		}
	}
	return true
}

// Options ajusta o rigor da detecção.
type Options struct {
	// Typos liga a busca por erro de digitação. Desligada, só rodam as regras que não
	// erram: grafia, inversão e iniciais.
	Typos bool
	// MinLenTypo é o tamanho mínimo da chave para admitir uma letra de diferença.
	// Nomes curtos casam por acaso: "ana lima" e "ana lume" não são a mesma pessoa.
	MinLenTypo int
	// Keep força um id a ser o sobrevivente do seu grupo.
	//
	// A escolha automática não sabe ortografia: entre "Bertrand Russel" com quatro livros
	// e "Bertrand Russell" com dois, ela fica com o primeiro, que é justamente a grafia
	// errada. Nenhuma regra derivada dos dados resolve isso — só quem conhece o autor.
	Keep map[string]bool
	// Skip lista ids que não devem entrar em grupo nenhum.
	//
	// Existe porque a regra de typo não tem como distinguir um erro de digitação de duas
	// pessoas com sobrenomes parecidos: "Marco Borges" e "Márcio Borges" estão a uma letra
	// de distância e são gente diferente. Vetar o id é mais honesto que afrouxar a regra
	// até o caso sumir — e o veto fica registrado no comando, não na cabeça de ninguém.
	Skip map[string]bool
}

// DefaultOptions são os padrões usados pelo comando.
func DefaultOptions() Options { return Options{Typos: true, MinLenTypo: 12} }

// entry é um Record com as três chaves derivadas que as regras comparam.
type entry struct {
	rec    Record
	canon  string   // tokens ordenados: pega grafia e inversão de uma vez
	semIni string   // tokens ordenados, sem as iniciais soltas
	ordem  []string // tokens na ordem original, para separar grafia de inversão
}

// Analyze agrupa os registros que parecem a mesma pessoa.
//
// Registros sem nome ou com nome genérico ("Unknown") ficam de fora: não são duplicata
// de ninguém, são ausência de informação.
func Analyze(recs []Record, opt Options) []Group {
	var es []entry
	for _, r := range recs {
		if opt.Skip[r.ID] {
			continue
		}
		toks := Tokens(r.Name)
		if len(toks) == 0 || generico(toks) {
			continue
		}
		ord := append([]string(nil), toks...)
		sort.Strings(toks)
		es = append(es, entry{
			rec:    r,
			canon:  strings.Join(toks, " "),
			semIni: strings.Join(semIniciais(toks), " "),
			ordem:  ord,
		})
	}

	uf := newUnionFind(len(es))
	rules := map[[2]int]Rule{}
	// A regra fica registrada por par mesmo quando os dois já estavam no mesmo grupo:
	// o relatório precisa dizer todos os motivos que uniram o grupo, não só o primeiro.
	link := func(i, j int, r Rule) {
		uf.union(i, j)
		rules[[2]int{min(i, j), max(i, j)}] = r
	}

	// Grafia e inversão: chave canônica idêntica. Um mapa resolve, sem comparar pares.
	porCanon := map[string][]int{}
	for i, e := range es {
		porCanon[e.canon] = append(porCanon[e.canon], i)
	}
	for _, idxs := range porCanon {
		for k := 1; k < len(idxs); k++ {
			i, j := idxs[0], idxs[k]
			r := RuleInvertido
			if strings.Join(es[i].ordem, " ") == strings.Join(es[j].ordem, " ") {
				r = RuleGrafia
			}
			link(i, j, r)
		}
	}

	// Iniciais: "George R. R. Martin" e "George Martin" viram a mesma chave quando as
	// iniciais saem. Exige dois nomes inteiros dos dois lados — sem isso "J. Smith" e
	// "Smith" casariam, e aí sobrenome vira identidade.
	porSemIni := map[string][]int{}
	for i, e := range es {
		if e.semIni == "" || len(strings.Fields(e.semIni)) < 2 {
			continue
		}
		porSemIni[e.semIni] = append(porSemIni[e.semIni], i)
	}
	for _, idxs := range porSemIni {
		for k := 1; k < len(idxs); k++ {
			i, j := idxs[0], idxs[k]
			if es[i].canon == es[j].canon {
				continue // já pego por grafia/inversão
			}
			link(i, j, RuleIniciais)
		}
	}

	if opt.Typos {
		detectarTypos(es2canon(es), uf, link, opt.MinLenTypo)
	}

	// Monta os grupos a partir das componentes.
	comp := map[int][]int{}
	for i := range es {
		root := uf.find(i)
		comp[root] = append(comp[root], i)
	}

	var out []Group
	for _, idxs := range comp {
		if len(idxs) < 2 {
			continue
		}
		var recsG []Record
		total := 0
		for _, i := range idxs {
			recsG = append(recsG, es[i].rec)
			total += es[i].rec.Books
		}
		canon := escolherCanonico(recsG, opt.Keep)

		var others []Record
		for _, r := range recsG {
			if r.ID != canon.ID {
				others = append(others, r)
			}
		}
		sort.Slice(others, func(a, b int) bool {
			if others[a].Books != others[b].Books {
				return others[a].Books > others[b].Books
			}
			return others[a].Name < others[b].Name
		})

		out = append(out, Group{
			Canonical: canon,
			Others:    others,
			Rules:     regrasDo(idxs, rules),
			Books:     total,
		})
	}

	// Maior impacto primeiro: o usuário resolve os grupos que movem mais livros antes de
	// gastar atenção nos que movem um.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Books != out[j].Books {
			return out[i].Books > out[j].Books
		}
		return out[i].Canonical.Name < out[j].Canonical.Name
	})
	return out
}

func es2canon(es []entry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.canon
	}
	return out
}

// detectarTypos compara chaves de comprimento parecido.
//
// A comparação é O(n²) dentro de cada faixa de comprimento, não sobre o catálogo
// inteiro: uma letra de diferença muda o tamanho em no máximo um, então nomes de
// tamanhos distantes não podem ser typo um do outro. Com nove mil autores isso reduz
// quarenta e quatro milhões de comparações para algumas centenas de milhares.
func detectarTypos(canon []string, uf *unionFind, link func(int, int, Rule), minLen int) {
	porLen := map[int][]int{}
	for i, c := range canon {
		porLen[len(c)] = append(porLen[len(c)], i)
	}
	for L, idxs := range porLen {
		if L < minLen {
			continue
		}
		// Mesma faixa e a faixa seguinte: cobre inserção/remoção de uma letra.
		vizinhos := append(append([]int(nil), idxs...), porLen[L+1]...)
		for a := 0; a < len(idxs); a++ {
			i := idxs[a]
			for _, j := range vizinhos {
				if j <= i {
					continue
				}
				if canon[i] == canon[j] {
					continue
				}
				if uf.find(i) == uf.find(j) {
					continue
				}
				// Duas letras só em nomes longos, onde o acaso é improvável.
				max := 1
				if L >= 20 {
					max = 2
				}
				if levenshtein(canon[i], canon[j], max) <= max {
					link(i, j, RuleTypo)
				}
			}
		}
	}
}

func regrasDo(idxs []int, rules map[[2]int]Rule) []Rule {
	vistas := map[Rule]bool{}
	for a := 0; a < len(idxs); a++ {
		for b := a + 1; b < len(idxs); b++ {
			if r, ok := rules[[2]int{min(idxs[a], idxs[b]), max(idxs[a], idxs[b])}]; ok {
				vistas[r] = true
			}
		}
	}
	var out []Rule
	for _, r := range []Rule{RuleGrafia, RuleInvertido, RuleIniciais, RuleTypo} {
		if vistas[r] {
			out = append(out, r)
		}
	}
	return out
}

// escolherCanonico decide qual grafia sobrevive ao merge.
//
// Contagem de livros não decide sozinha: "FÁBIO ULHOA COELHO" tem sete livros e ainda
// assim é a grafia errada. A qualidade da grafia vem primeiro, o volume desempata.
func escolherCanonico(recs []Record, keep map[string]bool) Record {
	// Um -keep no grupo encerra a discussão. Dois seriam contradição do usuário; vence o
	// primeiro em ordem de id, para a saída não depender da ordem de varredura.
	var forcado *Record
	for i, r := range recs {
		if keep[r.ID] && (forcado == nil || r.ID < forcado.ID) {
			forcado = &recs[i]
		}
	}
	if forcado != nil {
		return *forcado
	}

	melhor := recs[0]
	for _, r := range recs[1:] {
		if melhorQue(r, melhor) {
			melhor = r
		}
	}
	return melhor
}

func melhorQue(a, b Record) bool {
	if pa, pb := penalidade(a.Name), penalidade(b.Name); pa != pb {
		return pa < pb
	}
	if a.Books != b.Books {
		return a.Books > b.Books
	}
	// Entre grafias igualmente boas, a mais completa: "George R. R. Martin" carrega mais
	// informação que "George Martin".
	if na, nb := len(Tokens(a.Name)), len(Tokens(b.Name)); na != nb {
		return na > nb
	}
	return a.Name < b.Name
}

// penalidade pontua defeitos de grafia; menor é melhor.
func penalidade(name string) int {
	p := 0
	if strings.ContainsAny(name, ",;") {
		p += 4 // "Assis, Machado de" — ordem de catálogo, não de leitura
	}
	letras, up, low := 0, 0, 0
	for _, r := range name {
		if unicode.IsLetter(r) {
			letras++
			if unicode.IsUpper(r) {
				up++
			} else {
				low++
			}
		}
	}
	switch {
	case letras > 1 && up == letras:
		p += 3 // TUDO MAIÚSCULO
	case letras > 1 && low == letras:
		p += 2 // tudo minúsculo
	}
	if strings.Contains(name, "  ") {
		p++ // "Jenny    Smith"
	}
	if name != strings.TrimSpace(name) {
		p++
	}
	return p
}

// Tokens quebra o nome nas palavras que importam para a comparação.
//
// A pontuação vira separador antes da normalização porque "J.R.R. Tolkien" e
// "J. R. R. Tolkien" precisam render os mesmos tokens; removendo o ponto sem colocar
// espaço no lugar, o primeiro viraria um token só, "jrr".
func Tokens(name string) []string {
	var b strings.Builder
	for _, r := range name {
		if strings.ContainsRune(".,;&-_/\\|", r) {
			b.WriteRune(' ')
			continue
		}
		b.WriteRune(r)
	}
	raw := strings.Fields(meta.NormKey(b.String()))

	var out []string
	for _, t := range raw {
		// "JRR" e "J. R. R." são a mesma coisa escrita de dois jeitos. Um bloco curto sem
		// vogal é quase sempre iniciais coladas — e quando não é (o sobrenome "Ng"), o
		// pior resultado é o nome deixar de casar com nada, não casar errado.
		if len(t) >= 2 && len(t) <= 3 && semVogal(t) {
			for _, r := range t {
				out = append(out, string(r))
			}
			continue
		}
		out = append(out, t)
	}
	return out
}

func semVogal(t string) bool { return !strings.ContainsAny(t, "aeiouy") }

func semIniciais(toks []string) []string {
	var out []string
	for _, t := range toks {
		if len([]rune(t)) > 1 {
			out = append(out, t)
		}
	}
	return out
}

// generico descarta os registros que marcam ausência de autor. Unificá-los agruparia
// livros sem relação nenhuma.
var genericos = map[string]bool{
	"unknown": true, "desconhecido": true, "anonimo": true, "anonymous": true,
	"autor desconhecido": true, "unknown author": true, "varios": true,
	"varios autores": true, "vv aa": true, "n a": true, "sem autor": true,
}

func generico(toks []string) bool { return genericos[strings.Join(toks, " ")] }

// levenshtein com corte: para de calcular assim que a distância mínima possível passa de
// max, o que torna a varredura de pares viável.
func levenshtein(a, b string, max int) int {
	la, lb := len(a), len(b)
	if la-lb > max || lb-la > max {
		return max + 1
	}
	prev := make([]int, lb+1)
	cur := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		cur[0] = i
		menor := cur[0]
		for j := 1; j <= lb; j++ {
			custo := 1
			if a[i-1] == b[j-1] {
				custo = 0
			}
			cur[j] = minInt(prev[j]+1, cur[j-1]+1, prev[j-1]+custo)
			if cur[j] < menor {
				menor = cur[j]
			}
		}
		if menor > max {
			return max + 1
		}
		prev, cur = cur, prev
	}
	return prev[lb]
}

func minInt(xs ...int) int {
	m := xs[0]
	for _, x := range xs[1:] {
		if x < m {
			m = x
		}
	}
	return m
}

type unionFind struct{ parent, rank []int }

func newUnionFind(n int) *unionFind {
	u := &unionFind{parent: make([]int, n), rank: make([]int, n)}
	for i := range u.parent {
		u.parent[i] = i
	}
	return u
}

func (u *unionFind) find(x int) int {
	for u.parent[x] != x {
		u.parent[x] = u.parent[u.parent[x]]
		x = u.parent[x]
	}
	return x
}

func (u *unionFind) union(a, b int) bool {
	ra, rb := u.find(a), u.find(b)
	if ra == rb {
		return false
	}
	if u.rank[ra] < u.rank[rb] {
		ra, rb = rb, ra
	}
	u.parent[rb] = ra
	if u.rank[ra] == u.rank[rb] {
		u.rank[ra]++
	}
	return true
}
