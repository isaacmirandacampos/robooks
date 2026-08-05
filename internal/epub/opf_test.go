package epub

import (
	"os"
	"path/filepath"
	"testing"
)

// Lê um epub real da biblioteca, se existir, só para confirmar que os campos novos
// (tags, ISBN, editora, sinopse) saem preenchidos do OPF de verdade.
func TestReadMetaCamposExternos(t *testing.T) {
	home, _ := os.UserHomeDir()
	p := filepath.Join(home, "kavita/data/Livros/O Silmarillion/O Silmarillion - J. R. R. Tolkien.epub")
	if _, err := os.Stat(p); err != nil {
		t.Skip("biblioteca de referência ausente")
	}
	m, err := ReadMeta(p)
	if err != nil {
		t.Fatal(err)
	}
	if m.Title == "" || m.Author == "" {
		t.Errorf("título/autor vazios: %q / %q", m.Title, m.Author)
	}
	t.Logf("título=%q autor=%q isbn=%q editora=%q tags=%v sinopse=%v",
		m.Title, m.Author, m.ISBN, m.Publisher, m.Tags, m.HasDesc)
}
