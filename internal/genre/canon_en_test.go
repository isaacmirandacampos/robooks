package genre

import (
	"strings"
	"testing"
)

func TestCleanENCanoniza(t *testing.T) {
	casos := []struct{ in, want string }{
		{"Ficção", "Fiction"},
		{"Fantasia", "Fantasy"},
		{"Sci-fi", "Science Fiction"},
		{"Mystery Thriller", "Mystery"},
		{"Self Help", "Self-Help"},
		{"Chick-lit", "Chick Lit"},
		{"Dystopian", "Dystopia"},
		{"Biography & Autobiography", "Biography"},
		{"Espiritismo", "Spiritism"},
		{"Mediunidade", "Spiritism"},
	}
	for _, c := range casos {
		got := CleanEN([]string{c.in})
		if len(got) != 1 || got[0] != c.want {
			t.Errorf("CleanEN(%q) = %v, quer [%s]", c.in, got, c.want)
		}
	}
}

// Formato do arquivo, rótulo de loja e recorte geográfico não dizem nada sobre o
// assunto. "Audiobook" sozinho marcava 1352 livros.
func TestCleanENDescarta(t *testing.T) {
	for _, in := range []string{
		"Audiobook", "Ebooks", "General", "Adult", "Book Club", "Brazil", "France",
		"20th Century", "Medieval", "Portuguese", "Foreign Language Study",
		"Los Angeles (calif.)", "Claire (fictitious Character)", "Dune (imaginary Place)",
		"Harry Potter", "Star Trek", "Harlequin Blaze", "1735854266177",
	} {
		if got := CleanEN([]string{in}); len(got) != 0 {
			t.Errorf("CleanEN(%q) = %v, deveria descartar", in, got)
		}
	}
}

// A regressão que motivou a correção do reLang: o prefixo do idioma engolia a
// literatura nacional inteira.
func TestLiteraturaNacionalSobrevive(t *testing.T) {
	casos := []struct{ in, want string }{
		{"Portuguese Literature", "Portuguese Literature"},
		{"Spanish Literature", "Spanish Literature"},
		{"German Literature", "German Literature"},
		{"French Literature", "French Literature"},
		{"Italian Literature", "Italian Literature"},
		{"English Literature", "British Literature"},
		{"Literatura Brasileira", "Brazilian Literature"},
	}
	for _, c := range casos {
		got := CleanEN([]string{c.in})
		if len(got) != 1 || got[0] != c.want {
			t.Errorf("CleanEN(%q) = %v, quer [%s]", c.in, got, c.want)
		}
	}
	// Mas o idioma sozinho continua saindo.
	for _, in := range []string{"Portuguese", "English", "Foreign Languages"} {
		if got := CleanEN([]string{in}); len(got) != 0 {
			t.Errorf("CleanEN(%q) = %v, deveria descartar", in, got)
		}
	}
}

// Hierarquia colada abre em rótulos independentes.
func TestCleanENAbreLista(t *testing.T) {
	got := CleanEN([]string{"Fantasy; Thriller & Suspense; Ghosts"})
	j := strings.Join(got, "|")
	if !strings.Contains(j, "Fantasy") || !strings.Contains(j, "Thriller") {
		t.Errorf("CleanEN da lista = %v", got)
	}
}

// O segundo retorno separa "não está no mapa" de "está no mapa e manda descartar".
// Sem ele, o chamador não consegue tratar a cauda longa por frequência.
func TestCanonENConhecido(t *testing.T) {
	if _, ok := CanonEN("Fiction"); !ok {
		t.Error("Fiction deveria ser conhecido")
	}
	if _, ok := CanonEN("Audiobook"); !ok {
		t.Error("Audiobook deveria ser conhecido (descarte explícito)")
	}
	if v, ok := CanonEN("Serial Murderers"); ok {
		t.Errorf("rótulo fora do mapa deveria ser desconhecido, veio %q", v)
	}
}
