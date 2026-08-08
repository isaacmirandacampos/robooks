package edit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func escreve(t *testing.T, conteudo string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "in.tsv")
	if err := os.WriteFile(p, []byte(conteudo), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// A forma sem cabeçalho veio primeiro e precisa continuar valendo.
func TestLerImportTSVSemCabecalho(t *testing.T) {
	p := escreve(t, "/livros/a.epub\tFiction, Romance\n/livros/b.epub\tHorror\n")
	ls, temAutores, err := lerImportTSV(p)
	if err != nil {
		t.Fatal(err)
	}
	if temAutores {
		t.Error("não deveria anunciar autores")
	}
	if len(ls) != 2 || strings.Join(ls[0].tags, "|") != "Fiction|Romance" {
		t.Errorf("linhas = %+v", ls)
	}
	if len(ls[0].authors) != 0 {
		t.Errorf("não deveria ter autores: %v", ls[0].authors)
	}
}

func TestLerImportTSVComCabecalho(t *testing.T) {
	p := escreve(t, "path\tauthors\ttags\n/livros/a.epub\tJ. R. R. Tolkien\tFantasy, Classics\n")
	ls, temAutores, err := lerImportTSV(p)
	if err != nil {
		t.Fatal(err)
	}
	if !temAutores {
		t.Error("deveria anunciar autores")
	}
	if len(ls) != 1 || ls[0].authors[0] != "J. R. R. Tolkien" {
		t.Errorf("linhas = %+v", ls)
	}
}

// Vários autores vêm separados por "&", como o ebook-meta espera.
func TestLerImportTSVMultiploAutor(t *testing.T) {
	p := escreve(t, "path\tauthors\n/livros/a.epub\tMargaret Weis & Tracy Hickman\n")
	ls, _, err := lerImportTSV(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(ls[0].authors) != 2 || ls[0].authors[1] != "Tracy Hickman" {
		t.Errorf("autores = %v", ls[0].authors)
	}
}

// A regressão que motivou a normalização: epub.ReadMeta colapsa espaços ao ler o OPF.
// Sem o mesmo tratamento na entrada, "Albert  Camus" no catálogo pareceria diferente de
// "Albert  Camus" no arquivo, e o comando reescreveria centenas de epubs para gravar
// exatamente o que já estava lá.
func TestLerImportTSVColapsaEspacos(t *testing.T) {
	p := escreve(t, "path\tauthors\ttags\n/livros/a.epub\tAlbert  Camus\tScience   Fiction,  Classics \n")
	ls, _, err := lerImportTSV(p)
	if err != nil {
		t.Fatal(err)
	}
	if ls[0].authors[0] != "Albert Camus" {
		t.Errorf("autor = %q, quer %q", ls[0].authors[0], "Albert Camus")
	}
	if strings.Join(ls[0].tags, "|") != "Classics|Science Fiction" {
		t.Errorf("tags = %v", ls[0].tags)
	}
}

// Campo vazio significa "não mexa neste", e é o que separa "o catálogo não sabe" de
// "o catálogo diz que é vazio" — sem isso, uma coluna em branco apagaria os autores.
func TestLerImportTSVCampoVazioNaoApaga(t *testing.T) {
	p := escreve(t, "path\tauthors\ttags\n/livros/a.epub\t\tFiction\n/livros/b.epub\tStephen King\t\n")
	ls, _, err := lerImportTSV(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(ls) != 2 {
		t.Fatalf("quer 2 linhas, veio %d", len(ls))
	}
	if len(ls[0].authors) != 0 || len(ls[0].tags) != 1 {
		t.Errorf("linha 0 = %+v", ls[0])
	}
	if len(ls[1].tags) != 0 || len(ls[1].authors) != 1 {
		t.Errorf("linha 1 = %+v", ls[1])
	}
}
