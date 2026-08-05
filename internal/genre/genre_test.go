package genre

import (
	"reflect"
	"testing"
)

func TestClean(t *testing.T) {
	cases := []struct {
		nome string
		in   []string
		want []string
	}{
		{"remove marca de site", []string{"Romance", "Exilado dos livros", "epubr.club"}, []string{"Romance"}},
		{"remove genérico vazio", []string{"General", "Ficção", "ebook"}, []string{"Ficção"}},
		{"remove idioma", []string{"Portuguese", "Foreign Languages", "Terror"}, []string{"Terror"}},
		{"remove código BISAC", []string{"TRV009050", "1.2.2.0.0.1.0", "Aventura"}, []string{"Aventura"}},
		{"remove frase de sinopse", []string{"humor inteligente e princípios de teoria econômica ao longo do livro"}, nil},
		{"abre hierarquia", []string{"Policial / Suspense"}, []string{"Policial", "Suspense"}},
		{"unifica caixa", []string{"romance", "Romance"}, []string{"Romance"}},
		{"traduz e unifica", []string{"Fiction", "Ficção"}, []string{"Ficção"}},
		{"unifica sinônimos", []string{"sci-fi", "Science Fiction", "Ficção Científica"}, []string{"Ficção Científica"}},
		{"mantém gênero legítimo desconhecido", []string{"Espiritismo"}, []string{"Espiritismo"}},
		// "X & Y" vira dois filtros: mais útil que um rótulo composto que ninguém escolhe.
		{"divide composto com &", []string{"Science Fiction & Fantasy"}, []string{"Fantasia", "Ficção Científica"}},
		{"remove nome de pessoa", []string{"Chico Xavier", "Allan Kardec", "Espiritismo"}, []string{"Espiritismo"}},
		{"remove marca de loja", []string{"Oficial", "Comprado", "Epub", "Terror"}, []string{"Terror"}},
		{"unifica singular e plural", []string{"Conto", "Contos"}, []string{"Contos"}},
	}
	for _, c := range cases {
		got := Clean(c.in)
		if len(got) == 0 && len(c.want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: Clean(%v) = %v, want %v", c.nome, c.in, got, c.want)
		}
	}
}

// Um gênero de verdade não pode ser descartado só por ser incomum: a limpeza remove
// ruído, não raridade.
func TestNaoRemoveGeneroRaroLegitimo(t *testing.T) {
	for _, g := range []string{"Antropologia", "Numismática", "Enologia", "Cartografia"} {
		if got := Clean([]string{g}); len(got) != 1 {
			t.Errorf("Clean(%q) = %v, deveria manter", g, got)
		}
	}
}

// Regressões vistas ao inspecionar o resultado na biblioteca real.
func TestRuidoDescobertoNaPratica(t *testing.T) {
	if got := Clean([]string{"o(O_O)o"}); len(got) != 0 {
		t.Errorf("emoticon deveria sair, veio %v", got)
	}
	if got := Clean([]string{"World War II"}); len(got) != 1 || got[0] != "World War II" {
		t.Errorf("numeral romano quebrado: %v", got)
	}
	if got := Clean([]string{"European", "Women", "Romance"}); len(got) != 1 || got[0] != "Romance" {
		t.Errorf("região/demográfico deveriam sair: %v", got)
	}
}

func TestAdmit(t *testing.T) {
	freq := map[string]int{"Romance": 500, "Espiritismo": 40, "Numismática": 1}
	ok, no := Admit(
		[]string{"Romance", "General", "epubr.club", "Numismática", "Culinária", "Espiritismo"},
		freq, 3)

	// Romance e Espiritismo passam por frequência; Culinária passa por ser canônica
	// (primeiro livro do gênero precisa poder entrar); Numismática é rara e não
	// canônica, então fica de fora; General e epubr.club nem chegam à decisão.
	want := map[string]bool{"Romance": true, "Espiritismo": true, "Culinária": true}
	for _, g := range ok {
		if !want[g] {
			t.Errorf("aceitou %q indevidamente", g)
		}
		delete(want, g)
	}
	for g := range want {
		t.Errorf("deveria ter aceitado %q", g)
	}
	if len(no) != 1 || no[0] != "Numismática" {
		t.Errorf("recusados = %v, esperava [Numismática]", no)
	}
}
