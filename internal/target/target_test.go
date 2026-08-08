package target

import (
	"testing"

	"github.com/isaacmirandacampos/robooks/internal/epub"
	"github.com/isaacmirandacampos/robooks/internal/meta"
)

func TestKavitaPlacement(t *testing.T) {
	cases := []struct {
		name    string
		file    string
		m       epub.Meta
		wantDir string
	}{
		{
			// Volume de série: todos os volumes na pasta da série, que é o que faz o
			// Kavita agrupar sem depender do nome da pasta.
			name: "serie do OPF agrupa", file: "Maze Runner 02 - Prova de Fogo - James Dashner",
			m:       epub.Meta{Title: "Prova de fogo", Author: "James Dashner", Series: "Maze Runner", SeriesIndex: 2},
			wantDir: "Maze Runner",
		},
		{
			// Nome de download sujo não deve virar pasta quando há título de editor.
			name: "titulo do metadado vence o nome do arquivo", file: "1984 baixado da internet",
			m:       epub.Meta{Title: "1984", Author: "George Orwell"},
			wantDir: "1984",
		},
		{
			// Acentos ficam no metadado; a pasta é ASCII para não quebrar Samba.
			name: "pasta sem acento", file: "A Ameaca - Ken Follett",
			m:       epub.Meta{Title: "A Ameaça", Author: "Ken Follett"},
			wantDir: "A Ameaca",
		},
	}
	for _, c := range cases {
		b := meta.Parse(c.file)
		got := Kavita{}.Place(b, c.m)
		if got.Dir != c.wantDir {
			t.Errorf("%s: Dir = %q, want %q", c.name, got.Dir, c.wantDir)
		}
	}
}

func TestCalibrePlacement(t *testing.T) {
	b := meta.Parse("A Ameaca - Ken Follett")
	got := Calibre{}.Place(b, epub.Meta{Title: "A Ameaça", Author: "Ken Follett"})
	if got.Dir != "Follett, Ken/A Ameaca" {
		t.Errorf("Dir = %q, want %q", got.Dir, "Follett, Ken/A Ameaca")
	}
}

// Os dois alvos precisam divergir: se produzissem o mesmo layout, a abstração não
// estaria fazendo nada.
func TestTargetsDiferem(t *testing.T) {
	b := meta.Parse("Prova de Fogo - James Dashner")
	m := epub.Meta{Title: "Prova de fogo", Author: "James Dashner", Series: "Maze Runner", SeriesIndex: 2}
	k := Kavita{}.Place(b, m).Dir
	c := Calibre{}.Place(b, m).Dir
	if k == c {
		t.Errorf("kavita e calibre produziram o mesmo diretório: %q", k)
	}
}

// Kavita e Grimmory compartilham o layout de disco porque ambos leem os metadados de
// dentro do epub. O teste trava isso: se um dia divergirem, é uma mudança consciente.
func TestGrimmoryMesmoLayoutQueKavita(t *testing.T) {
	b := meta.Parse("Maze Runner 02 - Prova de Fogo - James Dashner")
	m := epub.Meta{Title: "Prova de fogo", Author: "James Dashner", Series: "Maze Runner", SeriesIndex: 2}
	k, g := Kavita{}.Place(b, m), Grimmory{}.Place(b, m)
	if k.Dir != g.Dir || k.Filename != g.Filename {
		t.Errorf("layouts divergiram: kavita=%s/%s grimmory=%s/%s", k.Dir, k.Filename, g.Dir, g.Filename)
	}
}

func TestGrimmoryRegistrado(t *testing.T) {
	if _, err := Get("grimmory"); err != nil {
		t.Errorf("alvo grimmory não registrado: %v", err)
	}
}
