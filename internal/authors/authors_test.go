package authors

import (
	"sort"
	"strings"
	"testing"
)

func rec(id string, books int, name string) Record {
	return Record{ID: id, Books: books, Name: name}
}

// grupoDe devolve o grupo que contém o id, ou nil.
func grupoDe(gs []Group, id string) *Group {
	for i := range gs {
		if gs[i].Canonical.ID == id {
			return &gs[i]
		}
		for _, o := range gs[i].Others {
			if o.ID == id {
				return &gs[i]
			}
		}
	}
	return nil
}

func ids(g Group) []string {
	out := []string{g.Canonical.ID}
	for _, o := range g.Others {
		out = append(out, o.ID)
	}
	sort.Strings(out)
	return out
}

func TestTokens(t *testing.T) {
	casos := []struct {
		in   string
		want string
	}{
		{"J.R.R. Tolkien", "j r r tolkien"},
		{"J. R. R. Tolkien", "j r r tolkien"},
		{"JRR Tolkien", "j r r tolkien"},
		{"FÁBIO ULHOA COELHO", "fabio ulhoa coelho"},
		{"Fábio Ulhoa Coelho", "fabio ulhoa coelho"},
		{"Assis, Machado de", "assis machado de"},
		{"Jenny    Smith", "jenny smith"},
	}
	for _, c := range casos {
		got := strings.Join(Tokens(c.in), " ")
		if got != c.want {
			t.Errorf("Tokens(%q) = %q, quer %q", c.in, got, c.want)
		}
	}
}

func TestGrafiaEInversao(t *testing.T) {
	gs := Analyze([]Record{
		rec("1", 7, "FÁBIO ULHOA COELHO"),
		rec("2", 2, "Fábio Ulhoa Coelho"),
		rec("3", 5, "Machado de Assis"),
		rec("4", 1, "Assis, Machado de"),
	}, DefaultOptions())

	if len(gs) != 2 {
		t.Fatalf("quer 2 grupos, veio %d: %+v", len(gs), gs)
	}

	// A grafia com caixa mista vence, mesmo com menos livros: TUDO MAIÚSCULO é defeito.
	g := grupoDe(gs, "1")
	if g.Canonical.Name != "Fábio Ulhoa Coelho" {
		t.Errorf("canônico = %q, quer %q", g.Canonical.Name, "Fábio Ulhoa Coelho")
	}
	if g.Books != 9 {
		t.Errorf("livros = %d, quer 9", g.Books)
	}

	// "Assis, Machado de" é ordem de catálogo; perde para a ordem de leitura.
	g = grupoDe(gs, "4")
	if g.Canonical.Name != "Machado de Assis" {
		t.Errorf("canônico = %q, quer %q", g.Canonical.Name, "Machado de Assis")
	}
	if g.Rules[0] != RuleInvertido {
		t.Errorf("regra = %v, quer %v", g.Rules, RuleInvertido)
	}
}

func TestIniciais(t *testing.T) {
	gs := Analyze([]Record{
		rec("1", 24, "George R. R. Martin"),
		rec("2", 3, "George Martin"),
	}, DefaultOptions())

	if len(gs) != 1 {
		t.Fatalf("quer 1 grupo, veio %d", len(gs))
	}
	// O nome mais completo sobrevive.
	if gs[0].Canonical.Name != "George R. R. Martin" {
		t.Errorf("canônico = %q", gs[0].Canonical.Name)
	}
}

// Sem esta salvaguarda, sobrenome vira identidade: todo "Silva" do acervo colapsaria
// num autor só.
func TestIniciaisNaoColapsaSobrenomeSozinho(t *testing.T) {
	gs := Analyze([]Record{
		rec("1", 3, "J. Smith"),
		rec("2", 5, "Smith"),
		rec("3", 2, "A. Smith"),
	}, Options{Typos: false})

	if len(gs) != 0 {
		t.Fatalf("não deveria agrupar por sobrenome: %+v", gs)
	}
}

func TestTypo(t *testing.T) {
	gs := Analyze([]Record{
		rec("1", 1, "Tom Perrota"),
		rec("2", 4, "Tom Perrotta"),
	}, Options{Typos: true, MinLenTypo: 10})

	if len(gs) != 1 {
		t.Fatalf("quer 1 grupo, veio %d", len(gs))
	}
	if gs[0].Confiavel() {
		t.Error("grupo por typo não pode ser marcado como confiável")
	}
	if gs[0].Canonical.Name != "Tom Perrotta" {
		t.Errorf("canônico = %q, quer o de mais livros", gs[0].Canonical.Name)
	}
}

// Nome curto casa por acaso; a regra de typo não pode alcançá-lo.
func TestTypoIgnoraNomeCurto(t *testing.T) {
	gs := Analyze([]Record{
		rec("1", 1, "Ana Lima"),
		rec("2", 1, "Ana Lume"),
	}, DefaultOptions())

	if len(gs) != 0 {
		t.Fatalf("não deveria agrupar nomes curtos: %+v", gs)
	}
}

// "Unknown" não é uma pessoa. Unificá-lo juntaria livros sem relação nenhuma.
func TestGenericoNaoAgrupa(t *testing.T) {
	gs := Analyze([]Record{
		rec("1", 30, "Unknown"),
		rec("2", 4, "unknown"),
		rec("3", 2, "Autor Desconhecido"),
	}, DefaultOptions())

	if len(gs) != 0 {
		t.Fatalf("genéricos não podem agrupar: %+v", gs)
	}
}

// Transitividade: A~B por grafia e B~C por iniciais devem render um grupo só, não dois
// pares sobrepostos que o usuário teria de mesclar duas vezes.
func TestTransitividade(t *testing.T) {
	gs := Analyze([]Record{
		rec("1", 5, "J. R. R. Tolkien"),
		rec("2", 2, "J.R.R. TOLKIEN"),
		rec("3", 1, "John Ronald Reuel Tolkien"),
		rec("4", 3, "Tolkien, J. R. R."),
	}, Options{Typos: false})

	g := grupoDe(gs, "1")
	if g == nil {
		t.Fatal("id 1 não entrou em grupo nenhum")
	}
	got := ids(*g)
	// O nome por extenso não compartilha tokens com as iniciais, então fica fora — é o
	// limite conhecido da regra, não um bug.
	want := []string{"1", "2", "4"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("grupo = %v, quer %v", got, want)
	}
}

func TestLevenshteinCorte(t *testing.T) {
	if d := levenshtein("abcdef", "abcdef", 1); d != 0 {
		t.Errorf("iguais = %d, quer 0", d)
	}
	if d := levenshtein("perrota tom", "perrotta tom", 1); d != 1 {
		t.Errorf("uma inserção = %d, quer 1", d)
	}
	// Além do corte, o valor exato não importa — só precisa ser maior que max.
	if d := levenshtein("abcdefgh", "zzzzzzzz", 1); d <= 1 {
		t.Errorf("distantes = %d, deveria passar do corte", d)
	}
}

func TestPenalidade(t *testing.T) {
	if penalidade("Fábio Ulhoa Coelho") != 0 {
		t.Error("grafia boa não deveria ter penalidade")
	}
	if penalidade("FÁBIO ULHOA COELHO") <= penalidade("Fábio Ulhoa Coelho") {
		t.Error("tudo maiúsculo deveria ser penalizado")
	}
	if penalidade("Assis, Machado de") <= penalidade("Machado de Assis") {
		t.Error("ordem de catálogo deveria ser penalizada")
	}
}

// O veto precisa remover o registro da análise inteira, não só da saída: se ele
// continuasse participando, poderia servir de ponte entre dois grupos que só se tocam
// através dele.
func TestSkip(t *testing.T) {
	recs := []Record{
		rec("1", 1, "Marco Borges"),
		rec("2", 1, "Márcio Borges"),
	}
	if gs := Analyze(recs, Options{Typos: true, MinLenTypo: 10}); len(gs) != 1 {
		t.Fatalf("sem veto deveria agrupar, veio %d", len(gs))
	}
	if gs := Analyze(recs, Options{Typos: true, MinLenTypo: 10, Skip: map[string]bool{"2": true}}); len(gs) != 0 {
		t.Fatalf("com veto não deveria agrupar: %+v", gs)
	}
}

func TestKeepForcaCanonico(t *testing.T) {
	recs := []Record{
		rec("1055", 4, "Bertrand Russel"),  // mais livros, grafia errada
		rec("3356", 2, "Bertrand Russell"), // menos livros, grafia certa
	}
	gs := Analyze(recs, Options{Typos: true, MinLenTypo: 12})
	if len(gs) != 1 {
		t.Fatalf("quer 1 grupo, veio %d", len(gs))
	}
	// Sem ajuda, a contagem decide — e decide errado.
	if gs[0].Canonical.Name != "Bertrand Russel" {
		t.Errorf("sem keep, canônico = %q", gs[0].Canonical.Name)
	}

	gs = Analyze(recs, Options{Typos: true, MinLenTypo: 12, Keep: map[string]bool{"3356": true}})
	if gs[0].Canonical.Name != "Bertrand Russell" {
		t.Errorf("com keep, canônico = %q, quer %q", gs[0].Canonical.Name, "Bertrand Russell")
	}
	if len(gs[0].Others) != 1 || gs[0].Others[0].ID != "1055" {
		t.Errorf("o outro deveria ser absorvido: %+v", gs[0].Others)
	}
}
