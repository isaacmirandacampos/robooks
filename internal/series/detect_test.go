package series

import "testing"

func nomes(cs []Candidate) []string {
	var n []string
	for _, c := range cs {
		n = append(n, c.Name)
	}
	return n
}

// O caso que motivou o package: a palavra que identifica a série está no fim do
// título, então agrupar por prefixo comum não encontra nada.
func TestDetectaSharpe(t *testing.T) {
	livros := []Book{
		{Title: "A Batalha de Sharpe", Author: "Bernard Cornwell"},
		{Title: "A Espada de Sharpe", Author: "Bernard Cornwell"},
		{Title: "A Fuga de Sharpe", Author: "Bernard Cornwell"},
		{Title: "A Águia de Sharpe", Author: "Bernard Cornwell"},
		{Title: "Azincourt", Author: "Bernard Cornwell"},
		{Title: "1356", Author: "Bernard Cornwell"},
	}
	got := Detect(livros, 3)
	if len(got) != 1 || got[0].Name != "Sharpe" {
		t.Fatalf("esperava a série Sharpe, veio %v", nomes(got))
	}
	if len(got[0].Members) != 4 {
		t.Errorf("esperava 4 volumes, veio %d", len(got[0].Members))
	}
	// Os avulsos não podem ser arrastados para dentro da série.
	for _, m := range got[0].Members {
		if m.Title == "Azincourt" || m.Title == "1356" {
			t.Errorf("avulso %q entrou na série", m.Title)
		}
	}
}

// Coletâneas compartilham palavras genéricas e não são série. Este é o erro que a
// detecção por semelhança cometeria.
func TestNaoInventaSerieComPalavraGenerica(t *testing.T) {
	livros := []Book{
		{Title: "20 Contos de Terror", Author: "Stephen King"},
		{Title: "21 Contos de Suspense", Author: "Stephen King"},
		{Title: "50 Contos Reunidos", Author: "Stephen King"},
		{Title: "A Grande Casa Assombrada", Author: "Stephen King"},
		{Title: "A Grande Fuga", Author: "Stephen King"},
		{Title: "A Grande Noite", Author: "Stephen King"},
	}
	if got := Detect(livros, 3); len(got) != 0 {
		t.Errorf("não deveria inventar série, veio %v", nomes(got))
	}
}

// Palavra comum em minúscula não nomeia série, mesmo repetida.
func TestExigeNomeProprio(t *testing.T) {
	livros := []Book{
		{Title: "O homem do castelo", Author: "X Y"},
		{Title: "A mulher do castelo", Author: "X Y"},
		{Title: "O filho do castelo", Author: "X Y"},
	}
	if got := Detect(livros, 3); len(got) != 0 {
		t.Errorf("palavra comum não deveria virar série: %v", nomes(got))
	}
}

// O termo não pode ser o título inteiro de um dos livros: aí ele é o livro.
func TestTermoNaoPodeSerOTituloInteiro(t *testing.T) {
	livros := []Book{
		{Title: "Duna", Author: "Frank Herbert"},
		{Title: "O Messias de Duna", Author: "Frank Herbert"},
		{Title: "Filhos de Duna", Author: "Frank Herbert"},
	}
	if got := Detect(livros, 3); len(got) != 0 {
		t.Errorf("'Duna' é título de livro, não deveria virar série aqui: %v", nomes(got))
	}
}

// Autores diferentes não compartilham série mesmo com a mesma palavra.
func TestNaoCruzaAutores(t *testing.T) {
	livros := []Book{
		{Title: "A Espada de Sharpe", Author: "Bernard Cornwell"},
		{Title: "A Fuga de Sharpe", Author: "Bernard Cornwell"},
		{Title: "O Livro de Sharpe", Author: "Outro Autor"},
	}
	got := Detect(livros, 3)
	if len(got) != 0 {
		t.Errorf("não deveria formar série cruzando autores: %v", nomes(got))
	}
}
